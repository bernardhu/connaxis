//go:build linux

package ktls

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"net"
	"reflect"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/bernardhu/connaxis/internal/tls"
	"github.com/bernardhu/connaxis/wrapper"
	"golang.org/x/sys/unix"
)

const (
	recordTypeChangeCipherSpec = 20
	recordTypeHandshake        = 22

	handshakeTypeClientHello = 1
	handshakeTypeServerHello = 2
)

const (
	ktlsVersion12 = 0x0303
	ktlsVersion13 = 0x0304

	ktlsCipherAESGCM128 = 51
	ktlsCipherAESGCM256 = 52

	ktlsTX = 1
	ktlsRX = 2
)

var ktlsCipherSuites = []uint16{
	tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_AES_128_GCM_SHA256,
	tls.TLS_AES_256_GCM_SHA384,
}

// KTLSPreferTLS12IfNoAESGCM forces TLS1.2 when client doesn't advertise TLS1.3 AES-GCM.
var KTLSPreferTLS12IfNoAESGCM = true

// KTLSForceTLS13AESGCM restricts TLS1.3 cipher suites to AES-GCM.
var KTLSForceTLS13AESGCM = true

// KTLSEnableRX controls whether kTLS RX is enabled.
var KTLSEnableRX = false

// KTLSDisableSessionTickets disables TLS session tickets when kTLS is enabled.
var KTLSDisableSessionTickets = false

// KTLSClientPostHandshakeTimeout is the max time to wait for TLS1.3 tickets.
var KTLSClientPostHandshakeTimeout = 10 * time.Millisecond

// KTLSClientPostHandshakeReadMaxBytes caps buffered app data during ticket drain.
var KTLSClientPostHandshakeReadMaxBytes = 64 * 1024

var tls13AESGCMOnce sync.Once

func maybeForceTLS13AESGCM() {
	if !KTLSForceTLS13AESGCM {
		return
	}
	tls13AESGCMOnce.Do(func() {
		tls.ForceTLS13AESGCM()
	})
}

type recordParser struct {
	buf []byte
	hs  []byte

	cipherOn bool
	seq      uint64

	gotClientRandom bool
	gotServerRandom bool
	clientRandom    [32]byte
	serverRandom    [32]byte
}

func (p *recordParser) feed(data []byte) {
	if len(data) == 0 {
		return
	}
	p.buf = append(p.buf, data...)
	for len(p.buf) >= 5 {
		typ := p.buf[0]
		length := int(p.buf[3])<<8 | int(p.buf[4])
		if len(p.buf) < 5+length {
			return
		}
		payload := p.buf[5 : 5+length]
		p.buf = p.buf[5+length:]
		p.handleRecord(typ, payload)
	}
}

func (p *recordParser) handleRecord(typ byte, payload []byte) {
	if typ == recordTypeChangeCipherSpec {
		p.cipherOn = true
		return
	}
	if p.cipherOn {
		p.seq++
		return
	}
	if typ != recordTypeHandshake {
		return
	}
	p.hs = append(p.hs, payload...)
	for len(p.hs) >= 4 {
		hsType := p.hs[0]
		hsLen := int(p.hs[1])<<16 | int(p.hs[2])<<8 | int(p.hs[3])
		if len(p.hs) < 4+hsLen {
			return
		}
		body := p.hs[4 : 4+hsLen]
		p.handleHandshake(hsType, body)
		p.hs = p.hs[4+hsLen:]
	}
}

func (p *recordParser) handleHandshake(typ byte, body []byte) {
	switch typ {
	case handshakeTypeClientHello:
		if p.gotClientRandom || len(body) < 34 {
			return
		}
		copy(p.clientRandom[:], body[2:34])
		p.gotClientRandom = true
	case handshakeTypeServerHello:
		if p.gotServerRandom || len(body) < 34 {
			return
		}
		copy(p.serverRandom[:], body[2:34])
		p.gotServerRandom = true
	}
}

type recordConn struct {
	net.Conn
	in  recordParser
	out recordParser
}

type RecordConn = recordConn

func newRecordConn(conn net.Conn) *recordConn {
	return &recordConn{Conn: conn}
}

func NewRecordConn(conn net.Conn) *recordConn {
	return newRecordConn(conn)
}

func (c *recordConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.in.feed(p[:n])
	}
	return n, err
}

func (c *recordConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if n > 0 {
		c.out.feed(p[:n])
	}
	return n, err
}

func (c *recordConn) clientRandom() []byte {
	if c.in.gotClientRandom {
		return append([]byte(nil), c.in.clientRandom[:]...)
	}
	if c.out.gotClientRandom {
		return append([]byte(nil), c.out.clientRandom[:]...)
	}
	return nil
}

func (c *recordConn) ClientRandom() []byte {
	return c.clientRandom()
}

func (c *recordConn) serverRandom() []byte {
	if c.in.gotServerRandom {
		return append([]byte(nil), c.in.serverRandom[:]...)
	}
	if c.out.gotServerRandom {
		return append([]byte(nil), c.out.serverRandom[:]...)
	}
	return nil
}

func (c *recordConn) ServerRandom() []byte {
	return c.serverRandom()
}

func (c *recordConn) inSeq() uint64 {
	return c.in.seq
}

func (c *recordConn) InSeq() uint64 {
	return c.inSeq()
}

func (c *recordConn) outSeq() uint64 {
	return c.out.seq
}

func (c *recordConn) OutSeq() uint64 {
	return c.outSeq()
}

type keyLogWriter struct {
	mu                  sync.Mutex
	buf                 string
	masterSecret        []byte
	clientRandom        []byte
	clientTrafficSecret []byte
	serverTrafficSecret []byte
	writer              io.Writer
}

type KeyLogWriter = keyLogWriter

func newKeyLogWriter(dst io.Writer) *keyLogWriter {
	return &keyLogWriter{writer: dst}
}

func NewKeyLogWriter(dst io.Writer) *keyLogWriter {
	return newKeyLogWriter(dst)
}

func (w *keyLogWriter) Write(p []byte) (int, error) {
	if w.writer != nil {
		_, _ = w.writer.Write(p)
	}
	w.mu.Lock()
	w.buf += string(p)
	for {
		idx := strings.IndexByte(w.buf, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimSpace(w.buf[:idx])
		w.buf = w.buf[idx+1:]
		w.parseLine(line)
	}
	w.mu.Unlock()
	return len(p), nil
}

func (w *keyLogWriter) parseLine(line string) {
	if !strings.HasPrefix(line, "CLIENT_RANDOM ") {
		if strings.HasPrefix(line, "CLIENT_TRAFFIC_SECRET_0 ") {
			w.parseTrafficSecret(line, true)
		} else if strings.HasPrefix(line, "SERVER_TRAFFIC_SECRET_0 ") {
			w.parseTrafficSecret(line, false)
		}
		return
	}
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return
	}
	clientRandom, err := hex.DecodeString(parts[1])
	if err != nil {
		return
	}
	masterSecret, err := hex.DecodeString(parts[2])
	if err != nil {
		return
	}
	w.clientRandom = clientRandom
	w.masterSecret = masterSecret
}

func (w *keyLogWriter) parseTrafficSecret(line string, isClient bool) {
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return
	}
	secret, err := hex.DecodeString(parts[2])
	if err != nil {
		return
	}
	if isClient {
		w.clientTrafficSecret = secret
	} else {
		w.serverTrafficSecret = secret
	}
}

func (w *keyLogWriter) secrets() (clientRandom, masterSecret []byte, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.masterSecret) == 0 {
		return nil, nil, false
	}
	return append([]byte(nil), w.clientRandom...), append([]byte(nil), w.masterSecret...), true
}

func (w *keyLogWriter) Secrets() (clientRandom, masterSecret []byte, ok bool) {
	return w.secrets()
}

func (w *keyLogWriter) trafficSecrets() (clientSecret, serverSecret []byte, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.clientTrafficSecret) == 0 || len(w.serverTrafficSecret) == 0 {
		return nil, nil, false
	}
	return append([]byte(nil), w.clientTrafficSecret...), append([]byte(nil), w.serverTrafficSecret...), true
}

func (w *keyLogWriter) TrafficSecrets() (clientSecret, serverSecret []byte, ok bool) {
	return w.trafficSecrets()
}

func addKeyLogWriter(klw *keyLogWriter, dst io.Writer) {
	if dst == nil {
		return
	}
	if klw.writer == nil {
		klw.writer = dst
		return
	}
	klw.writer = io.MultiWriter(klw.writer, dst)
}

func prepareKTLSConfig(cfg *tls.Config, klw *keyLogWriter) *tls.Config {
	c := cfg.Clone()
	// Keep user-configured TLS bounds when present, but constrain the kTLS path
	// to versions currently supported by this implementation (TLS1.2/TLS1.3).
	if c.MinVersion == 0 || c.MinVersion < tls.VersionTLS12 {
		c.MinVersion = tls.VersionTLS12
	}
	if c.MinVersion > tls.VersionTLS13 {
		c.MinVersion = tls.VersionTLS13
	}
	if c.MaxVersion == 0 || c.MaxVersion > tls.VersionTLS13 {
		c.MaxVersion = tls.VersionTLS13
	}
	if c.MaxVersion < tls.VersionTLS12 {
		c.MaxVersion = tls.VersionTLS12
	}
	if c.MaxVersion < c.MinVersion {
		c.MaxVersion = c.MinVersion
	}
	c.CipherSuites = filterKTLSCipherSuites(c.CipherSuites)
	addKeyLogWriter(klw, c.KeyLogWriter)
	c.KeyLogWriter = klw
	maybeForceTLS13AESGCM()
	if KTLSDisableSessionTickets {
		c.SessionTicketsDisabled = true
		c.ClientSessionCache = nil
	}

	applyClientPolicy := func(base *tls.Config, chi *tls.ClientHelloInfo) *tls.Config {
		if !KTLSPreferTLS12IfNoAESGCM {
			return base
		}
		// Respect strict TLS1.3 mode from the caller.
		if base.MinVersion >= tls.VersionTLS13 {
			return base
		}
		if clientHelloHasTLS13AESGCM(chi) {
			return base
		}
		cfg2 := base.Clone()
		cfg2.MaxVersion = tls.VersionTLS12
		return cfg2
	}

	if c.GetConfigForClient != nil {
		orig := c.GetConfigForClient
		c.GetConfigForClient = func(chi *tls.ClientHelloInfo) (*tls.Config, error) {
			cfg2, err := orig(chi)
			if cfg2 == nil || err != nil {
				return cfg2, err
			}
			return applyClientPolicy(prepareKTLSConfig(cfg2, klw), chi), nil
		}
	} else if KTLSPreferTLS12IfNoAESGCM && c.MinVersion < tls.VersionTLS13 {
		// In strict TLS1.3-only mode there is nothing to downgrade, so avoid
		// installing a GetConfigForClient callback entirely. This keeps the
		// server on the simpler stdlib handshake path for very large ClientHello
		// inputs (e.g. tlsfuzzer tolerance cases).
		c.GetConfigForClient = func(chi *tls.ClientHelloInfo) (*tls.Config, error) {
			return applyClientPolicy(c, chi), nil
		}
	}
	return c
}

func PrepareKTLSConfig(cfg *tls.Config, klw *keyLogWriter) *tls.Config {
	return prepareKTLSConfig(cfg, klw)
}

func filterKTLSCipherSuites(in []uint16) []uint16 {
	if len(in) == 0 {
		out := make([]uint16, len(ktlsCipherSuites))
		copy(out, ktlsCipherSuites)
		return out
	}
	out := make([]uint16, 0, len(in))
	for _, s := range in {
		if isKTLSCipher(s) {
			out = append(out, s)
		}
	}
	return out
}

func isKTLSCipher(suite uint16) bool {
	switch suite {
	case tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_AES_128_GCM_SHA256,
		tls.TLS_AES_256_GCM_SHA384:
		return true
	default:
		return false
	}
}

func IsKTLSCipher(suite uint16) bool {
	return isKTLSCipher(suite)
}

func clientHelloHasTLS13AESGCM(chi *tls.ClientHelloInfo) bool {
	if chi == nil {
		return true
	}
	hasTLS13 := false
	for _, v := range chi.SupportedVersions {
		if v == tls.VersionTLS13 {
			hasTLS13 = true
			break
		}
	}
	if !hasTLS13 {
		return true
	}
	for _, suite := range chi.CipherSuites {
		if suite == tls.TLS_AES_128_GCM_SHA256 || suite == tls.TLS_AES_256_GCM_SHA384 {
			return true
		}
	}
	return false
}

type tls12Keys struct {
	clientKey []byte
	serverKey []byte
	clientIV  []byte
	serverIV  []byte
	hash      func() hash.Hash
}

type TLS12Keys = tls12Keys

type tls13Keys struct {
	clientKey []byte
	serverKey []byte
	clientIV  []byte
	serverIV  []byte
	hash      func() hash.Hash
}

type TLS13Keys = tls13Keys

func deriveTLS12Keys(masterSecret, clientRandom, serverRandom []byte, suite uint16) (tls12Keys, error) {
	var keyLen int
	var h func() hash.Hash
	switch suite {
	case tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_RSA_WITH_AES_128_GCM_SHA256:
		keyLen = 16
		h = sha256.New
	case tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_RSA_WITH_AES_256_GCM_SHA384:
		keyLen = 32
		h = sha512.New384
	default:
		return tls12Keys{}, errors.New("unsupported cipher suite for ktls")
	}

	ivLen := 4
	keyBlockLen := 2*keyLen + 2*ivLen
	seed := append(append([]byte(nil), serverRandom...), clientRandom...)
	keyBlock := tls12PRF(masterSecret, "key expansion", seed, keyBlockLen, h)

	offset := 0
	clientKey := keyBlock[offset : offset+keyLen]
	offset += keyLen
	serverKey := keyBlock[offset : offset+keyLen]
	offset += keyLen
	clientIV := keyBlock[offset : offset+ivLen]
	offset += ivLen
	serverIV := keyBlock[offset : offset+ivLen]

	return tls12Keys{
		clientKey: clientKey,
		serverKey: serverKey,
		clientIV:  clientIV,
		serverIV:  serverIV,
		hash:      h,
	}, nil
}

func DeriveTLS12Keys(masterSecret, clientRandom, serverRandom []byte, suite uint16) (tls12Keys, error) {
	return deriveTLS12Keys(masterSecret, clientRandom, serverRandom, suite)
}

func tls12PRF(secret []byte, label string, seed []byte, length int, h func() hash.Hash) []byte {
	labelAndSeed := append([]byte(label), seed...)
	return pHash(secret, labelAndSeed, length, h)
}

func deriveTLS13Keys(clientSecret, serverSecret []byte, suite uint16) (tls13Keys, error) {
	var keyLen int
	var h func() hash.Hash
	switch suite {
	case tls.TLS_AES_128_GCM_SHA256:
		keyLen = 16
		h = sha256.New
	case tls.TLS_AES_256_GCM_SHA384:
		keyLen = 32
		h = sha512.New384
	default:
		return tls13Keys{}, errors.New("unsupported tls1.3 cipher suite for ktls")
	}

	ivLen := 12
	clientKey := hkdfExpandLabel(clientSecret, h, "key", nil, keyLen)
	clientIV := hkdfExpandLabel(clientSecret, h, "iv", nil, ivLen)
	serverKey := hkdfExpandLabel(serverSecret, h, "key", nil, keyLen)
	serverIV := hkdfExpandLabel(serverSecret, h, "iv", nil, ivLen)

	return tls13Keys{
		clientKey: clientKey,
		serverKey: serverKey,
		clientIV:  clientIV,
		serverIV:  serverIV,
		hash:      h,
	}, nil
}

func DeriveTLS13Keys(clientSecret, serverSecret []byte, suite uint16) (tls13Keys, error) {
	return deriveTLS13Keys(clientSecret, serverSecret, suite)
}

func hkdfExpandLabel(secret []byte, h func() hash.Hash, label string, context []byte, length int) []byte {
	fullLabel := append([]byte("tls13 "), []byte(label)...)
	info := make([]byte, 2+1+len(fullLabel)+1+len(context))
	binary.BigEndian.PutUint16(info[0:], uint16(length))
	info[2] = byte(len(fullLabel))
	copy(info[3:], fullLabel)
	idx := 3 + len(fullLabel)
	info[idx] = byte(len(context))
	copy(info[idx+1:], context)

	out, err := hkdf.Expand(h, secret, string(info), length)
	if err != nil {
		return nil
	}
	return out
}

func pHash(secret, seed []byte, length int, h func() hash.Hash) []byte {
	result := make([]byte, length)
	a := seed
	offset := 0
	for offset < length {
		mac := hmac.New(h, secret)
		_, _ = mac.Write(a)
		a = mac.Sum(nil)

		mac = hmac.New(h, secret)
		_, _ = mac.Write(a)
		_, _ = mac.Write(seed)
		sum := mac.Sum(nil)

		n := copy(result[offset:], sum)
		offset += n
	}
	return result
}

type tlsCryptoInfo struct {
	Version    uint16
	CipherType uint16
}

type tls12CryptoInfoAESGCM128 struct {
	Info   tlsCryptoInfo
	IV     [8]byte
	Key    [16]byte
	Salt   [4]byte
	RecSeq [8]byte
}

type tls12CryptoInfoAESGCM256 struct {
	Info   tlsCryptoInfo
	IV     [8]byte
	Key    [32]byte
	Salt   [4]byte
	RecSeq [8]byte
}

func setsockopt(fd, level, opt int, val unsafe.Pointer, vallen uintptr) error {
	_, _, errno := unix.Syscall6(unix.SYS_SETSOCKOPT, uintptr(fd), uintptr(level), uintptr(opt), uintptr(val), vallen, 0)
	if errno != 0 {
		return errno
	}
	return nil
}

func fillRecSeq(dst *[8]byte, seq uint64) {
	binary.BigEndian.PutUint64(dst[:], seq)
}

func fillIVFromSeq(dst *[8]byte, seq uint64) {
	binary.BigEndian.PutUint64(dst[:], seq)
}

func splitIV(iv []byte, salt *[4]byte, explicit *[8]byte) {
	if len(iv) >= 4 {
		copy(salt[:], iv[:4])
	}
	if len(iv) >= 12 {
		copy(explicit[:], iv[4:12])
	}
}

func splitIVAlt(iv []byte, salt *[4]byte, explicit *[8]byte) {
	if len(iv) >= 12 {
		copy(explicit[:], iv[:8])
		copy(salt[:], iv[8:12])
	}
}

func enableKTLS(fd int, isClient bool, suite uint16, keys tls12Keys, inSeq, outSeq uint64) error {
	if err := unix.SetsockoptString(fd, unix.IPPROTO_TCP, unix.TCP_ULP, "tls"); err != nil {
		return err
	}

	txKey, txIV, rxKey, rxIV := keys.serverKey, keys.serverIV, keys.clientKey, keys.clientIV
	if isClient {
		txKey, txIV, rxKey, rxIV = keys.clientKey, keys.clientIV, keys.serverKey, keys.serverIV
	}

	switch suite {
	case tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_RSA_WITH_AES_128_GCM_SHA256:
		var tx tls12CryptoInfoAESGCM128
		tx.Info = tlsCryptoInfo{Version: ktlsVersion12, CipherType: ktlsCipherAESGCM128}
		copy(tx.Key[:], txKey)
		copy(tx.Salt[:], txIV)
		fillRecSeq(&tx.RecSeq, outSeq)
		if err := setsockopt(fd, unix.SOL_TLS, ktlsTX, unsafe.Pointer(&tx), unsafe.Sizeof(tx)); err != nil {
			if err == unix.EINVAL {
				fillIVFromSeq(&tx.IV, outSeq)
				if err := setsockopt(fd, unix.SOL_TLS, ktlsTX, unsafe.Pointer(&tx), unsafe.Sizeof(tx)); err != nil {
					return err
				}
			} else {
				return err
			}
		}

		if KTLSEnableRX {
			var rx tls12CryptoInfoAESGCM128
			rx.Info = tlsCryptoInfo{Version: ktlsVersion12, CipherType: ktlsCipherAESGCM128}
			copy(rx.Key[:], rxKey)
			copy(rx.Salt[:], rxIV)
			fillRecSeq(&rx.RecSeq, inSeq)
			if err := setsockopt(fd, unix.SOL_TLS, ktlsRX, unsafe.Pointer(&rx), unsafe.Sizeof(rx)); err != nil {
				if err == unix.EINVAL {
					fillIVFromSeq(&rx.IV, inSeq)
					if err := setsockopt(fd, unix.SOL_TLS, ktlsRX, unsafe.Pointer(&rx), unsafe.Sizeof(rx)); err != nil {
						return err
					}
				} else {
					return err
				}
			}
		}
	case tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_RSA_WITH_AES_256_GCM_SHA384:
		var tx tls12CryptoInfoAESGCM256
		tx.Info = tlsCryptoInfo{Version: ktlsVersion12, CipherType: ktlsCipherAESGCM256}
		copy(tx.Key[:], txKey)
		copy(tx.Salt[:], txIV)
		fillRecSeq(&tx.RecSeq, outSeq)
		if err := setsockopt(fd, unix.SOL_TLS, ktlsTX, unsafe.Pointer(&tx), unsafe.Sizeof(tx)); err != nil {
			if err == unix.EINVAL {
				fillIVFromSeq(&tx.IV, outSeq)
				if err := setsockopt(fd, unix.SOL_TLS, ktlsTX, unsafe.Pointer(&tx), unsafe.Sizeof(tx)); err != nil {
					return err
				}
			} else {
				return err
			}
		}

		if KTLSEnableRX {
			var rx tls12CryptoInfoAESGCM256
			rx.Info = tlsCryptoInfo{Version: ktlsVersion12, CipherType: ktlsCipherAESGCM256}
			copy(rx.Key[:], rxKey)
			copy(rx.Salt[:], rxIV)
			fillRecSeq(&rx.RecSeq, inSeq)
			if err := setsockopt(fd, unix.SOL_TLS, ktlsRX, unsafe.Pointer(&rx), unsafe.Sizeof(rx)); err != nil {
				if err == unix.EINVAL {
					fillIVFromSeq(&rx.IV, inSeq)
					if err := setsockopt(fd, unix.SOL_TLS, ktlsRX, unsafe.Pointer(&rx), unsafe.Sizeof(rx)); err != nil {
						return err
					}
				} else {
					return err
				}
			}
		}
	default:
		return errors.New("unsupported cipher suite for ktls")
	}

	return nil
}

func EnableKTLS(fd int, isClient bool, suite uint16, keys tls12Keys, inSeq, outSeq uint64) error {
	return enableKTLS(fd, isClient, suite, keys, inSeq, outSeq)
}

func tlsConnRecSeq(tc *tls.Conn) (inSeq, outSeq [8]byte, ok bool) {
	if tc == nil {
		return inSeq, outSeq, false
	}
	v := reflect.ValueOf(tc)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return inSeq, outSeq, false
	}
	e := v.Elem()
	inField := e.FieldByName("in")
	outField := e.FieldByName("out")
	if !inField.IsValid() || !outField.IsValid() {
		return inSeq, outSeq, false
	}
	inSeqField := inField.FieldByName("seq")
	outSeqField := outField.FieldByName("seq")
	if !inSeqField.IsValid() || !outSeqField.IsValid() || !inSeqField.CanAddr() || !outSeqField.CanAddr() {
		return inSeq, outSeq, false
	}
	inSeq = *(*[8]byte)(unsafe.Pointer(inSeqField.UnsafeAddr()))
	outSeq = *(*[8]byte)(unsafe.Pointer(outSeqField.UnsafeAddr()))
	return inSeq, outSeq, true
}

func TLSConnRecSeq(tc *tls.Conn) (inSeq, outSeq [8]byte, ok bool) {
	return tlsConnRecSeq(tc)
}

func enableKTLS13(fd int, isClient bool, suite uint16, keys tls13Keys, inSeq, outSeq [8]byte) error {
	if err := unix.SetsockoptString(fd, unix.IPPROTO_TCP, unix.TCP_ULP, "tls"); err != nil {
		return err
	}

	txKey, txIV, rxKey, rxIV := keys.serverKey, keys.serverIV, keys.clientKey, keys.clientIV
	if isClient {
		txKey, txIV, rxKey, rxIV = keys.clientKey, keys.clientIV, keys.serverKey, keys.serverIV
	}

	switch suite {
	case tls.TLS_AES_128_GCM_SHA256:
		var tx tls12CryptoInfoAESGCM128
		tx.Info = tlsCryptoInfo{Version: ktlsVersion13, CipherType: ktlsCipherAESGCM128}
		copy(tx.Key[:], txKey)
		splitIV(txIV, &tx.Salt, &tx.IV)
		tx.RecSeq = outSeq
		if err := setsockopt(fd, unix.SOL_TLS, ktlsTX, unsafe.Pointer(&tx), unsafe.Sizeof(tx)); err != nil {
			if err == unix.EINVAL {
				splitIVAlt(txIV, &tx.Salt, &tx.IV)
				if err := setsockopt(fd, unix.SOL_TLS, ktlsTX, unsafe.Pointer(&tx), unsafe.Sizeof(tx)); err != nil {
					return err
				}
			} else {
				return err
			}
		}

		if KTLSEnableRX {
			var rx tls12CryptoInfoAESGCM128
			rx.Info = tlsCryptoInfo{Version: ktlsVersion13, CipherType: ktlsCipherAESGCM128}
			copy(rx.Key[:], rxKey)
			splitIV(rxIV, &rx.Salt, &rx.IV)
			rx.RecSeq = inSeq
			if err := setsockopt(fd, unix.SOL_TLS, ktlsRX, unsafe.Pointer(&rx), unsafe.Sizeof(rx)); err != nil {
				if err == unix.EINVAL {
					splitIVAlt(rxIV, &rx.Salt, &rx.IV)
					if err := setsockopt(fd, unix.SOL_TLS, ktlsRX, unsafe.Pointer(&rx), unsafe.Sizeof(rx)); err != nil {
						return err
					}
				} else {
					return err
				}
			}
		}
	case tls.TLS_AES_256_GCM_SHA384:
		var tx tls12CryptoInfoAESGCM256
		tx.Info = tlsCryptoInfo{Version: ktlsVersion13, CipherType: ktlsCipherAESGCM256}
		copy(tx.Key[:], txKey)
		splitIV(txIV, &tx.Salt, &tx.IV)
		tx.RecSeq = outSeq
		if err := setsockopt(fd, unix.SOL_TLS, ktlsTX, unsafe.Pointer(&tx), unsafe.Sizeof(tx)); err != nil {
			if err == unix.EINVAL {
				splitIVAlt(txIV, &tx.Salt, &tx.IV)
				if err := setsockopt(fd, unix.SOL_TLS, ktlsTX, unsafe.Pointer(&tx), unsafe.Sizeof(tx)); err != nil {
					return err
				}
			} else {
				return err
			}
		}

		if KTLSEnableRX {
			var rx tls12CryptoInfoAESGCM256
			rx.Info = tlsCryptoInfo{Version: ktlsVersion13, CipherType: ktlsCipherAESGCM256}
			copy(rx.Key[:], rxKey)
			splitIV(rxIV, &rx.Salt, &rx.IV)
			rx.RecSeq = inSeq
			if err := setsockopt(fd, unix.SOL_TLS, ktlsRX, unsafe.Pointer(&rx), unsafe.Sizeof(rx)); err != nil {
				if err == unix.EINVAL {
					splitIVAlt(rxIV, &rx.Salt, &rx.IV)
					if err := setsockopt(fd, unix.SOL_TLS, ktlsRX, unsafe.Pointer(&rx), unsafe.Sizeof(rx)); err != nil {
						return err
					}
				} else {
					return err
				}
			}
		}
	default:
		return errors.New("unsupported tls1.3 cipher suite for ktls")
	}

	return nil
}

func EnableKTLS13(fd int, isClient bool, suite uint16, keys tls13Keys, inSeq, outSeq [8]byte) error {
	return enableKTLS13(fd, isClient, suite, keys, inSeq, outSeq)
}

func drainClientTickets(tc *tls.Conn, conn net.Conn) []byte {
	if KTLSClientPostHandshakeTimeout <= 0 || KTLSClientPostHandshakeReadMaxBytes <= 0 {
		return nil
	}
	_ = conn.SetReadDeadline(time.Now().Add(KTLSClientPostHandshakeTimeout))
	defer func() {
		_ = conn.SetReadDeadline(time.Time{})
	}()

	buf := make([]byte, KTLSClientPostHandshakeReadMaxBytes)
	n, err := tc.Read(buf)
	if n > 0 {
		out := make([]byte, n)
		copy(out, buf[:n])
		return out
	}
	if err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return nil
		}
		wrapper.Warnf("ktls ticket drain: %v", err)
	}
	return nil
}

func DrainClientTickets(tc *tls.Conn, conn net.Conn) []byte {
	return drainClientTickets(tc, conn)
}
