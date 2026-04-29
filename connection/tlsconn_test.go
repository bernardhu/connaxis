package connection

import (
	"errors"
	"net"
	"testing"
	"time"
)

func TestTLSBufferConnCloseKeepsDirectUntilHandshakeOwnerClearsIt(t *testing.T) {
	direct := &testDirectConn{}
	buf := newTLSBufferConn()
	buf.direct = direct

	if err := buf.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if buf.direct == nil {
		t.Fatal("Close() cleared direct; handshake owner should clear it after HandshakeContext returns")
	}

	if _, err := buf.Read(make([]byte, 1)); !errors.Is(err, errTestDirectClosed) {
		t.Fatalf("Read() after Close() error = %v, want %v", err, errTestDirectClosed)
	}
}

var errTestDirectClosed = errors.New("test direct conn closed")

type testDirectConn struct {
	closed bool
}

func (c *testDirectConn) Read([]byte) (int, error) {
	if c.closed {
		return 0, errTestDirectClosed
	}
	return 0, nil
}

func (c *testDirectConn) Write(p []byte) (int, error) { return len(p), nil }

func (c *testDirectConn) Close() error {
	c.closed = true
	return nil
}

func (c *testDirectConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (c *testDirectConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (c *testDirectConn) SetDeadline(time.Time) error      { return nil }
func (c *testDirectConn) SetReadDeadline(time.Time) error  { return nil }
func (c *testDirectConn) SetWriteDeadline(time.Time) error { return nil }
