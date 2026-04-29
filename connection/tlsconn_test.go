package connection

import (
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

func TestTLSBufferConnCloseClearsDirectWithoutWaitingForRead(t *testing.T) {
	direct := &blockingDirectConn{
		reading: make(chan struct{}),
		closed:  make(chan struct{}),
	}
	buf := newTLSBufferConn()
	buf.setDirect(direct)

	readErr := make(chan error, 1)
	go func() {
		_, err := buf.Read(make([]byte, 1))
		readErr <- err
	}()

	select {
	case <-direct.reading:
	case <-time.After(time.Second):
		t.Fatal("Read() did not reach direct conn")
	}

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- buf.Close()
	}()

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close() blocked behind Read()")
	}
	if direct := buf.getDirect(); direct != nil {
		t.Fatalf("direct after Close() = %T, want nil", direct)
	}

	select {
	case err := <-readErr:
		if !errors.Is(err, errBlockingDirectClosed) {
			t.Fatalf("Read() error = %v, want %v", err, errBlockingDirectClosed)
		}
	case <-time.After(time.Second):
		t.Fatal("Read() did not unblock after Close()")
	}
}

var errBlockingDirectClosed = errors.New("blocking direct conn closed")

type blockingDirectConn struct {
	readingOnce sync.Once
	closeOnce   sync.Once
	reading     chan struct{}
	closed      chan struct{}
}

func (c *blockingDirectConn) Read([]byte) (int, error) {
	c.readingOnce.Do(func() {
		close(c.reading)
	})
	<-c.closed
	return 0, errBlockingDirectClosed
}

func (c *blockingDirectConn) Write(p []byte) (int, error) { return len(p), nil }

func (c *blockingDirectConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
	})
	return nil
}

func (c *blockingDirectConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (c *blockingDirectConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (c *blockingDirectConn) SetDeadline(time.Time) error      { return nil }
func (c *blockingDirectConn) SetReadDeadline(time.Time) error  { return nil }
func (c *blockingDirectConn) SetWriteDeadline(time.Time) error { return nil }
