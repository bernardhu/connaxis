//go:build linux

package connection

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	internalktls "github.com/bernardhu/connaxis/internal/ktls"
	"github.com/bernardhu/connaxis/internal/tls"
	"github.com/bernardhu/connaxis/ringbuffer"
	"github.com/bernardhu/connaxis/wrapper"
	"golang.org/x/sys/unix"
)

const (
	// NOTE: This is intentionally set to 5.15 for our current benchmark environment.
	// If the running kernel doesn't actually support TLS1.3 kTLS, EnableKTLS13 will
	// fail and the connection will transparently fall back to the userspace TLS path.
	ktlsTLS13MinKernelMajor = 5
	ktlsTLS13MinKernelMinor = 15
)

var (
	ktlsTLS13KernelCheckOnce sync.Once
	ktlsTLS13KernelSupported bool
	ktlsTLS13KernelRelease   string
)

func newKTLSConnServer(ctx context.Context, fd int, cfg *tls.Config) (*ATLSConn, error) {
	return newKTLSConn(ctx, fd, cfg, false)
}

func newKTLSConnClient(ctx context.Context, fd int, cfg *tls.Config) (*ATLSConn, error) {
	return newKTLSConn(ctx, fd, cfg, true)
}

func (c *ATLSConn) finalizeServerKTLS() error {
	if c == nil || c.Conn == nil {
		return errors.New("tls conn not initialized")
	}

	state := c.Conn.ConnectionState()
	c.state = state
	c.hasState = true

	if !internalktls.IsKTLSCipher(state.CipherSuite) {
		wrapper.Warnf("ktls disabled for fd %d: unsupported cipher 0x%x", c.fd, state.CipherSuite)
		return nil
	}

	if c.handshakeRC == nil {
		wrapper.Warnf("ktls disabled for fd %d: missing handshake record conn", c.fd)
		return nil
	}

	switch state.Version {
	case tls.VersionTLS12:
		if c.handshakeRC == nil || c.handshakeKLW == nil {
			wrapper.Warnf("ktls disabled for fd %d: missing tls12 handshake state", c.fd)
			return nil
		}
		clientRandom := c.handshakeRC.ClientRandom()
		serverRandom := c.handshakeRC.ServerRandom()
		if len(clientRandom) != 32 || len(serverRandom) != 32 {
			wrapper.Warnf("ktls disabled for fd %d: invalid hello randoms client=%d server=%d", c.fd, len(clientRandom), len(serverRandom))
			return nil
		}
		_, masterSecret, ok := c.handshakeKLW.Secrets()
		if !ok || len(masterSecret) == 0 {
			wrapper.Warnf("ktls disabled for fd %d: missing tls12 master secret", c.fd)
			return nil
		}
		keys, err := internalktls.DeriveTLS12Keys(masterSecret, clientRandom, serverRandom, state.CipherSuite)
		if err != nil {
			wrapper.Warnf("ktls disabled for fd %d: derive tls12 keys: %v", c.fd, err)
			return nil
		}
		if err := internalktls.EnableKTLS(c.fd, false, state.CipherSuite, keys, c.handshakeRC.InSeq(), c.handshakeRC.OutSeq()); err != nil {
			wrapper.Warnf("ktls disabled for fd %d: enable tls12 tx: %v", c.fd, err)
			return nil
		}
		if internalktls.KTLSEnableRX {
			c.preRead = nil
			c.preReadOff = 0
			c.Conn = nil
			c.ktls = true
			c.ktlsRX = true
			return nil
		}
		c.preRead = drainBufferedTLSPlaintext(c.Conn, c.handshakeRC, KTLSClientPostHandshakeReadMaxBytes)
		c.preReadOff = 0
	case tls.VersionTLS13:
		if !kernelSupportsKTLS13() {
			reason := "ktls tls1.3 requires linux kernel >= 5.15"
			if ktlsTLS13KernelRelease != "" {
				reason = reason + ", current=" + ktlsTLS13KernelRelease
			}
			wrapper.Warnf("ktls disabled for fd %d: %s", c.fd, reason)
			return nil
		}
		if c.handshakeKLW == nil {
			wrapper.Warnf("ktls disabled for fd %d: missing tls13 key log state", c.fd)
			return nil
		}
		clientSecret, serverSecret, ok := c.handshakeKLW.TrafficSecrets()
		if !ok {
			wrapper.Warnf("ktls disabled for fd %d: missing tls13 traffic secrets", c.fd)
			return nil
		}
		keys, err := internalktls.DeriveTLS13Keys(clientSecret, serverSecret, state.CipherSuite)
		if err != nil {
			wrapper.Warnf("ktls disabled for fd %d: derive tls13 keys: %v", c.fd, err)
			return nil
		}
		var preRead []byte
		if internalktls.KTLSEnableRX {
			// TLS1.3 clients may pipeline the first app-data record right behind the
			// handshake flight. Drain it from userspace tls.Conn before switching RX
			// to kTLS so we don't drop already-buffered plaintext.
			preRead = drainBufferedTLSPlaintext(c.Conn, c.handshakeRC, KTLSClientPostHandshakeReadMaxBytes)
			wrapper.Debugf("ktls tls13 server pre-read before rx enable fd=%d bytes=%d", c.fd, len(preRead))
		}
		inSeq, outSeq, ok := internalktls.TLSConnRecSeq(c.Conn)
		if !ok {
			wrapper.Warnf("ktls disabled for fd %d: missing tls13 record seq", c.fd)
			return nil
		}
		if err := internalktls.EnableKTLS13(c.fd, false, state.CipherSuite, keys, inSeq, outSeq); err != nil {
			wrapper.Warnf("ktls disabled for fd %d: enable tls13 tx: %v", c.fd, err)
			return nil
		}
		if internalktls.KTLSEnableRX {
			c.preRead = preRead
			c.preReadOff = 0
			c.Conn = nil
			c.ktls = true
			c.ktlsRX = true
			return nil
		}
		c.preRead = drainBufferedTLSPlaintext(c.Conn, c.handshakeRC, KTLSClientPostHandshakeReadMaxBytes)
		c.preReadOff = 0
	default:
		wrapper.Warnf("ktls disabled for fd %d: unsupported version 0x%x", c.fd, state.Version)
		return nil
	}

	c.ktls = true
	c.ktlsRX = false
	return nil
}

func drainBufferedTLSPlaintext(tc *tls.Conn, conn net.Conn, maxBytes int) []byte {
	if maxBytes <= 0 {
		return nil
	}
	wrapper.Debugf("ktls pre-read start maxBytes=%d", maxBytes)
	_ = conn.SetReadDeadline(time.Now())
	defer func() {
		_ = conn.SetReadDeadline(time.Time{})
	}()

	out := make([]byte, 0, minInt(maxBytes, 4096))
	buf := make([]byte, 4096)
	for len(out) < maxBytes {
		need := maxBytes - len(out)
		if need < len(buf) {
			buf = buf[:need]
		}
		n, err := tc.Read(buf)
		if n > 0 {
			out = append(out, buf[:n]...)
			wrapper.Debugf("ktls pre-read chunk n=%d total=%d", n, len(out))
			continue
		}
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				wrapper.Debugf("ktls pre-read stop by timeout total=%d", len(out))
				break
			}
			wrapper.Warnf("ktls pre-read drain: %v", err)
		} else {
			// This can happen on non-blocking paths; just stop draining.
			wrapper.Debugf("ktls pre-read stop by zero-read total=%d", len(out))
		}
		break
	}
	if len(out) == 0 {
		wrapper.Debugf("ktls pre-read done total=0")
		return nil
	}
	wrapper.Debugf("ktls pre-read done total=%d", len(out))
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func newKTLSConn(ctx context.Context, fd int, cfg *tls.Config, isClient bool) (*ATLSConn, error) {
	if cfg == nil {
		return nil, errors.New("nil tls config")
	}

	begin := time.Now()
	wrapper.Debugf("newKTLSConn start fd=%d isClient=%v", fd, isClient)
	klw := internalktls.NewKeyLogWriter(nil)
	ktlsCfg := internalktls.PrepareKTLSConfig(cfg, klw)
	if len(ktlsCfg.CipherSuites) == 0 {
		return nil, errors.New("ktls cipher suite list is empty")
	}

	buf := newTLSBufferConn()
	hc, err := handshakeConnFromFD(fd)
	if err != nil {
		return nil, err
	}
	if !isClient {
		if strictConn, err := maybeWrapStrictTLS13ServerConn(hc, cfg); err != nil {
			_ = hc.Close()
			return nil, err
		} else {
			hc = strictConn
		}
	}

	rc := internalktls.NewRecordConn(hc)
	buf.direct = rc
	buf.local = hc.LocalAddr()
	buf.remote = hc.RemoteAddr()

	var tc *tls.Conn
	if isClient {
		tc = tls.Client(buf, ktlsCfg)
	} else {
		tc = tls.Server(buf, ktlsCfg)
	}

	handshakeCtx := ctx
	if handshakeCtx == nil {
		var cancel context.CancelFunc
		handshakeCtx, cancel = context.WithTimeout(context.Background(), TlsHandshakeTimeout)
		defer cancel()
	}
	if err := tc.HandshakeContext(handshakeCtx); err != nil {
		wrapper.Warnf("newKTLSConn handshake fail fd=%d isClient=%v cost=%s err=%v", fd, isClient, time.Since(begin), err)
		_ = tc.Close()
		_ = hc.Close()
		return nil, err
	}

	state := tc.ConnectionState()
	wrapper.Debugf("newKTLSConn handshake ok fd=%d isClient=%v cost=%s version=0x%x cipher=0x%x", fd, isClient, time.Since(begin), state.Version, state.CipherSuite)
	recordCipherSuite(state)
	if !internalktls.IsKTLSCipher(state.CipherSuite) {
		_ = hc.Close()
		buf.direct = nil
		wrapper.Warnf("newKTLSConn fallback fd=%d isClient=%v reason=unsupported cipher=0x%x", fd, isClient, state.CipherSuite)
		return newStdTLSFallback(fd, buf, tc, nil, errors.New("ktls unsupported cipher"))
	}

	preRead := drainBufferedTLSPlaintext(tc, rc, KTLSClientPostHandshakeReadMaxBytes)
	wrapper.Debugf("newKTLSConn preRead fd=%d isClient=%v bytes=%d", fd, isClient, len(preRead))

	switch state.Version {
	case tls.VersionTLS12:
		clientRandom := rc.ClientRandom()
		serverRandom := rc.ServerRandom()
		if len(clientRandom) != 32 || len(serverRandom) != 32 {
			_ = hc.Close()
			buf.direct = nil
			wrapper.Warnf("newKTLSConn fallback fd=%d isClient=%v reason=hello random invalid client=%d server=%d", fd, isClient, len(clientRandom), len(serverRandom))
			return newStdTLSFallback(fd, buf, tc, preRead, errors.New("failed to capture hello randoms"))
		}

		_, masterSecret, ok := klw.Secrets()
		if !ok || len(masterSecret) == 0 {
			_ = hc.Close()
			buf.direct = nil
			wrapper.Warnf("newKTLSConn fallback fd=%d isClient=%v reason=master secret missing", fd, isClient)
			return newStdTLSFallback(fd, buf, tc, preRead, errors.New("failed to capture master secret"))
		}

		keys, err := internalktls.DeriveTLS12Keys(masterSecret, clientRandom, serverRandom, state.CipherSuite)
		if err != nil {
			_ = hc.Close()
			buf.direct = nil
			wrapper.Warnf("newKTLSConn fallback fd=%d isClient=%v reason=derive tls12 keys err=%v", fd, isClient, err)
			return newStdTLSFallback(fd, buf, tc, preRead, err)
		}

		if err := internalktls.EnableKTLS(fd, isClient, state.CipherSuite, keys, rc.InSeq(), rc.OutSeq()); err != nil {
			_ = hc.Close()
			buf.direct = nil
			wrapper.Warnf("newKTLSConn fallback fd=%d isClient=%v reason=enable ktls12 err=%v", fd, isClient, err)
			return newStdTLSFallback(fd, buf, tc, preRead, err)
		}
		wrapper.Debugf("newKTLSConn ktls12 enabled fd=%d isClient=%v inSeq=%x outSeq=%x preRead=%d", fd, isClient, rc.InSeq(), rc.OutSeq(), len(preRead))
		if !internalktls.KTLSEnableRX {
			_ = hc.Close()
			buf.direct = nil
			c := &ATLSConn{
				Conn:       tc,
				buf:        buf,
				ktls:       true,
				ktlsRX:     false,
				state:      state,
				hasState:   true,
				preRead:    preRead,
				preReadOff: 0,
			}
			c.SetFd(fd)
			c.SetRecvbuf(ringbuffer.NewRingBuffer())
			wrapper.Debugf("newKTLSConn done fd=%d isClient=%v version=tls12 mode=tx-only preRead=%d totalCost=%s", fd, isClient, len(preRead), time.Since(begin))
			return c, nil
		}
	case tls.VersionTLS13:
		if !kernelSupportsKTLS13() {
			_ = hc.Close()
			buf.direct = nil
			reason := "ktls tls1.3 requires linux kernel >= 5.15"
			if ktlsTLS13KernelRelease != "" {
				reason = reason + ", current=" + ktlsTLS13KernelRelease
			}
			wrapper.Warnf("newKTLSConn fallback fd=%d isClient=%v reason=%s", fd, isClient, reason)
			return newStdTLSFallback(fd, buf, tc, preRead, errors.New(reason))
		}

		if isClient && !KTLSDisableSessionTickets && ktlsCfg.ClientSessionCache != nil {
			if ticketData := internalktls.DrainClientTickets(tc, rc); len(ticketData) > 0 {
				wrapper.Debugf("newKTLSConn tls13 ticket drain fd=%d bytes=%d", fd, len(ticketData))
				preRead = append(preRead, ticketData...)
			}
		}

		clientSecret, serverSecret, ok := klw.TrafficSecrets()
		if !ok {
			_ = hc.Close()
			buf.direct = nil
			wrapper.Warnf("newKTLSConn fallback fd=%d isClient=%v reason=traffic secret missing", fd, isClient)
			return newStdTLSFallback(fd, buf, tc, preRead, errors.New("failed to capture traffic secrets"))
		}

		keys, err := internalktls.DeriveTLS13Keys(clientSecret, serverSecret, state.CipherSuite)
		if err != nil {
			_ = hc.Close()
			buf.direct = nil
			wrapper.Warnf("newKTLSConn fallback fd=%d isClient=%v reason=derive tls13 keys err=%v", fd, isClient, err)
			return newStdTLSFallback(fd, buf, tc, preRead, err)
		}

		inSeq, outSeq, ok := internalktls.TLSConnRecSeq(tc)
		if !ok {
			_ = hc.Close()
			buf.direct = nil
			return newStdTLSFallback(fd, buf, tc, preRead, errors.New("failed to read tls record seq"))
		}

		if err := internalktls.EnableKTLS13(fd, isClient, state.CipherSuite, keys, inSeq, outSeq); err != nil {
			_ = hc.Close()
			buf.direct = nil
			wrapper.Warnf("newKTLSConn fallback fd=%d isClient=%v reason=enable ktls13 err=%v", fd, isClient, err)
			return newStdTLSFallback(fd, buf, tc, preRead, err)
		}
		wrapper.Debugf("newKTLSConn ktls13 enabled fd=%d isClient=%v inSeq=%x outSeq=%x preRead=%d", fd, isClient, inSeq, outSeq, len(preRead))
		if !internalktls.KTLSEnableRX {
			_ = hc.Close()
			buf.direct = nil
			c := &ATLSConn{
				Conn:       tc,
				buf:        buf,
				ktls:       true,
				ktlsRX:     false,
				state:      state,
				hasState:   true,
				preRead:    preRead,
				preReadOff: 0,
			}
			c.SetFd(fd)
			c.SetRecvbuf(ringbuffer.NewRingBuffer())
			wrapper.Debugf("newKTLSConn done fd=%d isClient=%v version=tls13 mode=tx-only preRead=%d totalCost=%s", fd, isClient, len(preRead), time.Since(begin))
			return c, nil
		}

		_ = hc.Close()
		buf.direct = nil
		c := &ATLSConn{
			Conn:       nil,
			buf:        nil,
			ktls:       true,
			ktlsRX:     true,
			state:      state,
			hasState:   true,
			preRead:    preRead,
			preReadOff: 0,
		}
		c.SetFd(fd)
		c.SetRecvbuf(ringbuffer.NewRingBuffer())
		wrapper.Debugf("newKTLSConn done fd=%d isClient=%v version=tls13 preRead=%d totalCost=%s", fd, isClient, len(preRead), time.Since(begin))
		return c, nil
	default:
		_ = hc.Close()
		buf.direct = nil
		wrapper.Warnf("newKTLSConn fallback fd=%d isClient=%v reason=unsupported version=0x%x", fd, isClient, state.Version)
		return newStdTLSFallback(fd, buf, tc, preRead, errors.New("unsupported tls version"))
	}

	_ = hc.Close()
	buf.direct = nil
	c := &ATLSConn{
		Conn:       nil,
		buf:        nil,
		ktls:       true,
		ktlsRX:     true,
		state:      state,
		hasState:   true,
		preRead:    preRead,
		preReadOff: 0,
	}
	c.SetFd(fd)
	c.SetRecvbuf(ringbuffer.NewRingBuffer())
	wrapper.Debugf("newKTLSConn done fd=%d isClient=%v version=tls12 preRead=%d totalCost=%s", fd, isClient, len(preRead), time.Since(begin))
	return c, nil
}

func newStdTLSFallback(fd int, buf *tlsBufferConn, tc *tls.Conn, preRead []byte, cause error) (*ATLSConn, error) {
	wrapper.Warnf("ktls disabled for fd %d: %v", fd, cause)
	c := &ATLSConn{
		Conn:       tc,
		buf:        buf,
		preRead:    preRead,
		preReadOff: 0,
	}
	c.SetFd(fd)
	c.SetRecvbuf(ringbuffer.NewRingBuffer())
	return c, nil
}

func kernelSupportsKTLS13() bool {
	ktlsTLS13KernelCheckOnce.Do(func() {
		ktlsTLS13KernelSupported = false

		var u unix.Utsname
		if err := unix.Uname(&u); err != nil {
			return
		}
		rel := charsToString(u.Release[:])
		ktlsTLS13KernelRelease = rel

		maj, min, ok := parseKernelMajorMinor(rel)
		if !ok {
			return
		}
		if maj > ktlsTLS13MinKernelMajor || (maj == ktlsTLS13MinKernelMajor && min >= ktlsTLS13MinKernelMinor) {
			ktlsTLS13KernelSupported = true
		}
	})
	return ktlsTLS13KernelSupported
}

func parseKernelMajorMinor(release string) (int, int, bool) {
	parts := strings.SplitN(release, ".", 3)
	if len(parts) < 2 {
		return 0, 0, false
	}
	maj, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	min, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return maj, min, true
}

func charsToString[T ~byte | ~int8](ca []T) string {
	n := 0
	for n < len(ca) && ca[n] != 0 {
		n++
	}
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		out[i] = byte(ca[i])
	}
	return string(out)
}
