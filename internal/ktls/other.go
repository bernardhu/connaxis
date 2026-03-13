//go:build !linux

package ktls

import (
	"errors"
	"hash"
	"io"
	"net"
	"time"

	"github.com/bernardhu/connaxis/internal/tls"
)

var KTLSPreferTLS12IfNoAESGCM = true
var KTLSForceTLS13AESGCM = true
var KTLSEnableRX = false
var KTLSDisableSessionTickets = false
var KTLSClientPostHandshakeTimeout = 10 * time.Millisecond
var KTLSClientPostHandshakeReadMaxBytes = 64 * 1024

type TLS12Keys struct {
	ClientKey []byte
	ServerKey []byte
	ClientIV  []byte
	ServerIV  []byte
	Hash      func() hash.Hash
}

type TLS13Keys struct {
	ClientKey []byte
	ServerKey []byte
	ClientIV  []byte
	ServerIV  []byte
	Hash      func() hash.Hash
}

type RecordConn struct {
	net.Conn
}

func NewRecordConn(conn net.Conn) *RecordConn {
	return &RecordConn{Conn: conn}
}

func (c *RecordConn) ClientRandom() []byte { return nil }
func (c *RecordConn) ServerRandom() []byte { return nil }
func (c *RecordConn) InSeq() uint64        { return 0 }
func (c *RecordConn) OutSeq() uint64       { return 0 }

type KeyLogWriter struct{}

func NewKeyLogWriter(io.Writer) *KeyLogWriter {
	return &KeyLogWriter{}
}

func (w *KeyLogWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *KeyLogWriter) Secrets() ([]byte, []byte, bool) {
	return nil, nil, false
}
func (w *KeyLogWriter) TrafficSecrets() ([]byte, []byte, bool) {
	return nil, nil, false
}

func PrepareKTLSConfig(cfg *tls.Config, _ *KeyLogWriter) *tls.Config {
	return cfg
}

func IsKTLSCipher(uint16) bool {
	return false
}

func DeriveTLS12Keys([]byte, []byte, []byte, uint16) (TLS12Keys, error) {
	return TLS12Keys{}, errors.New("ktls supported on linux only")
}

func DeriveTLS13Keys([]byte, []byte, uint16) (TLS13Keys, error) {
	return TLS13Keys{}, errors.New("ktls supported on linux only")
}

func EnableKTLS(int, bool, uint16, TLS12Keys, uint64, uint64) error {
	return errors.New("ktls supported on linux only")
}

func TLSConnRecSeq(*tls.Conn) ([8]byte, [8]byte, bool) {
	return [8]byte{}, [8]byte{}, false
}

func EnableKTLS13(int, bool, uint16, TLS13Keys, [8]byte, [8]byte) error {
	return errors.New("ktls supported on linux only")
}

func DrainClientTickets(*tls.Conn, net.Conn) []byte {
	return nil
}
