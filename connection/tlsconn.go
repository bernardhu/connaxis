package connection

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	internalktls "github.com/bernardhu/connaxis/internal/ktls"
	"github.com/bernardhu/connaxis/internal/tls"
	"github.com/bernardhu/connaxis/ringbuffer"
	"github.com/bernardhu/connaxis/wrapper"
	"golang.org/x/sys/unix"
)

var TlsHandshakeTimeout = 1 * time.Second

const (
	tlsCipherReadSize  = 32 * 1024
	tlsWriteChunkSize  = 16 * 1024
	tlsMaxWritevIovecs = 2
)

type tlsBufferConn struct {
	// direct is only used during the handshake, when crypto/tls expects blocking I/O.
	// After the handshake, direct is nil and crypto/tls reads from cin and writes to cout.
	directMu sync.Mutex
	direct   net.Conn

	cin  *ringbuffer.RingBuffer
	cout *ringbuffer.RingBuffer

	local  net.Addr
	remote net.Addr
}

func newTLSBufferConn() *tlsBufferConn {
	return &tlsBufferConn{
		cin:  ringbuffer.NewRingBuffer(),
		cout: ringbuffer.NewRingBuffer(),
	}
}

type wouldBlockError struct{}

func (*wouldBlockError) Error() string   { return "connaxis: would block" }
func (*wouldBlockError) Timeout() bool   { return true }
func (*wouldBlockError) Temporary() bool { return true }

var errWouldBlock = &wouldBlockError{}

func (b *tlsBufferConn) setDirect(conn net.Conn) {
	b.directMu.Lock()
	b.direct = conn
	b.directMu.Unlock()
}

func (b *tlsBufferConn) getDirect() net.Conn {
	b.directMu.Lock()
	direct := b.direct
	b.directMu.Unlock()
	return direct
}

func (b *tlsBufferConn) clearDirect() net.Conn {
	b.directMu.Lock()
	direct := b.direct
	b.direct = nil
	b.directMu.Unlock()
	return direct
}

func (b *tlsBufferConn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if direct := b.getDirect(); direct != nil {
		return direct.Read(p)
	}
	if b.cin.Has() == 0 {
		return 0, errWouldBlock
	}
	buf := p
	n := b.cin.Read(&buf, len(buf), true)
	if n <= 0 {
		return 0, errWouldBlock
	}
	return n, nil
}

func (b *tlsBufferConn) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if direct := b.getDirect(); direct != nil {
		return direct.Write(p)
	}
	data := p
	if b.cout.Write(&data, len(data), true) < 0 {
		return 0, unix.ENOMEM
	}
	return len(p), nil
}

func (b *tlsBufferConn) Close() error {
	if direct := b.clearDirect(); direct != nil {
		return direct.Close()
	}
	return nil
}

func (b *tlsBufferConn) LocalAddr() net.Addr {
	if direct := b.getDirect(); direct != nil {
		return direct.LocalAddr()
	}
	return b.local
}

func (b *tlsBufferConn) RemoteAddr() net.Addr {
	if direct := b.getDirect(); direct != nil {
		return direct.RemoteAddr()
	}
	return b.remote
}

func (b *tlsBufferConn) SetDeadline(t time.Time) error {
	if direct := b.getDirect(); direct != nil {
		return direct.SetDeadline(t)
	}
	return nil
}

func (b *tlsBufferConn) SetReadDeadline(t time.Time) error {
	if direct := b.getDirect(); direct != nil {
		return direct.SetReadDeadline(t)
	}
	return nil
}

func (b *tlsBufferConn) SetWriteDeadline(t time.Time) error {
	if direct := b.getDirect(); direct != nil {
		return direct.SetWriteDeadline(t)
	}
	return nil
}

// ATLSConn is a non-blocking async TLS connection integrated with the event loop.
// The TLS handshake is completed before the connection is added to the loop.
type ATLSConn struct {
	Base

	closecb CloseCb
	opencb  OpenCb

	Conn *tls.Conn
	buf  *tlsBufferConn
	ktls bool
	// ktlsRX indicates whether reads go through kernel TLS RX.
	// When false, reads stay on userspace crypto/tls while writes may still use kTLS TX.
	ktlsRX bool

	// In kTLS mode we intentionally drop *tls.Conn after handshake to avoid per-conn
	// userspace TLS overhead, but BoGo and some callers still need negotiated state
	// (SNI/ALPN/exporter/tls-unique). Capture it during handshake and expose via
	// ConnectionState().
	state    tls.ConnectionState
	hasState bool

	preRead    []byte
	preReadOff int

	acceptLoopID  int
	handshakeAt   time.Time
	handshakeMode TLSEngine
	handshakeRC   *internalktls.RecordConn
	handshakeKLW  *internalktls.KeyLogWriter
}

func NewTLSConnServer(ctx context.Context, fd int, cfg *tls.Config) (*ATLSConn, error) {
	if GetTLSEngine() == TLSEngineKTLS {
		return newKTLSConnServer(ctx, fd, cfg)
	}
	return NewATLSConnServer(ctx, fd, cfg)
}

func NewTLSConnClient(ctx context.Context, fd int, cfg *tls.Config) (*ATLSConn, error) {
	if GetTLSEngine() == TLSEngineKTLS {
		return newKTLSConnClient(ctx, fd, cfg)
	}
	return NewATLSConnClient(ctx, fd, cfg)
}

func NewPendingTLSServerConn(fd int, cfg *tls.Config, local, remote net.Addr) (*ATLSConn, error) {
	if cfg == nil {
		return nil, errors.New("nil tls config")
	}

	c := &ATLSConn{
		buf:           newTLSBufferConn(),
		handshakeAt:   time.Now(),
		handshakeMode: GetTLSEngine(),
	}
	c.SetFd(fd)
	c.SetRecvbuf(ringbuffer.NewRingBuffer())
	c.SetLocal(local)
	c.SetRemote(remote)
	direct := newFDConn(fd, local, remote)

	switch c.handshakeMode {
	case TLSEngineKTLS:
		klw := internalktls.NewKeyLogWriter(nil)
		ktlsCfg := internalktls.PrepareKTLSConfig(cfg, klw)
		if len(ktlsCfg.CipherSuites) == 0 {
			return nil, errors.New("ktls cipher suite list is empty")
		}
		c.handshakeKLW = klw
		c.handshakeRC = internalktls.NewRecordConn(direct)
		c.buf.setDirect(c.handshakeRC)
		c.Conn = tls.Server(c.buf, ktlsCfg)
	default:
		c.buf.setDirect(direct)
		c.Conn = tls.Server(c.buf, cfg)
	}

	return c, nil
}

func handshakeConnFromFD(fd int) (net.Conn, error) {
	dup, err := unix.Dup(fd)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(dup), "connaxis-tls-handshake")
	if f == nil {
		_ = unix.Close(dup)
		return nil, errors.New("failed to create os.File for handshake fd")
	}
	conn, err := net.FileConn(f)
	_ = f.Close()
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func NewATLSConnServer(ctx context.Context, fd int, cfg *tls.Config) (*ATLSConn, error) {
	if cfg == nil {
		return nil, errors.New("nil tls config")
	}

	buf := newTLSBufferConn()
	hc, err := handshakeConnFromFD(fd)
	if err != nil {
		return nil, err
	}
	if strictConn, err := maybeWrapStrictTLS13ServerConn(hc, cfg); err != nil {
		_ = hc.Close()
		return nil, err
	} else {
		hc = strictConn
	}
	buf.setDirect(hc)
	buf.local = hc.LocalAddr()
	buf.remote = hc.RemoteAddr()

	tc := tls.Server(buf, cfg)

	handshakeCtx := ctx
	if handshakeCtx == nil {
		var cancel context.CancelFunc
		handshakeCtx, cancel = context.WithTimeout(context.Background(), TlsHandshakeTimeout)
		defer cancel()
	}
	if err := tc.HandshakeContext(handshakeCtx); err != nil {
		_ = tc.Close()
		_ = hc.Close()
		return nil, err
	}
	recordCipherSuite(tc.ConnectionState())

	_ = hc.Close()
	buf.clearDirect()

	c := &ATLSConn{
		Conn: tc,
		buf:  buf,
	}
	c.SetFd(fd)
	c.SetRecvbuf(ringbuffer.NewRingBuffer())
	return c, nil
}

func NewATLSConnClient(ctx context.Context, fd int, cfg *tls.Config) (*ATLSConn, error) {
	if cfg == nil {
		return nil, errors.New("nil tls config")
	}

	buf := newTLSBufferConn()
	hc, err := handshakeConnFromFD(fd)
	if err != nil {
		return nil, err
	}
	buf.setDirect(hc)
	buf.local = hc.LocalAddr()
	buf.remote = hc.RemoteAddr()

	tc := tls.Client(buf, cfg)

	handshakeCtx := ctx
	if handshakeCtx == nil {
		var cancel context.CancelFunc
		handshakeCtx, cancel = context.WithTimeout(context.Background(), TlsHandshakeTimeout)
		defer cancel()
	}
	if err := tc.HandshakeContext(handshakeCtx); err != nil {
		_ = tc.Close()
		_ = hc.Close()
		return nil, err
	}
	recordCipherSuite(tc.ConnectionState())

	_ = hc.Close()
	buf.clearDirect()

	c := &ATLSConn{
		Conn: tc,
		buf:  buf,
	}
	c.SetFd(fd)
	c.SetRecvbuf(ringbuffer.NewRingBuffer())
	return c, nil
}

func (c *ATLSConn) ConnectionState() tls.ConnectionState {
	if c == nil {
		return tls.ConnectionState{}
	}
	if c.Conn != nil {
		return c.Conn.ConnectionState()
	}
	if c.hasState {
		return c.state
	}
	return tls.ConnectionState{}
}

func (c *ATLSConn) SetAcceptLoopID(loopID int) {
	c.acceptLoopID = loopID
}

func (c *ATLSConn) AcceptLoopID() int {
	return c.acceptLoopID
}

func (c *ATLSConn) HandshakeAt() time.Time {
	return c.handshakeAt
}

func (c *ATLSConn) CompleteServerHandshake() error {
	if c == nil {
		return errors.New("nil tls conn")
	}
	if c.Conn == nil || c.buf == nil || c.buf.getDirect() == nil {
		return errors.New("tls conn not initialized")
	}
	if err := unix.SetNonblock(c.fd, false); err != nil {
		return err
	}
	defer func() {
		_ = unix.SetNonblock(c.fd, true)
	}()

	handshakeCtx := context.Background()
	if TlsHandshakeTimeout > 0 {
		var cancel context.CancelFunc
		handshakeCtx, cancel = context.WithTimeout(context.Background(), TlsHandshakeTimeout)
		defer cancel()
	}

	if err := c.Conn.HandshakeContext(handshakeCtx); err != nil {
		return err
	}
	recordCipherSuite(c.Conn.ConnectionState())

	if c.handshakeMode == TLSEngineKTLS {
		if err := c.finalizeServerKTLS(); err != nil {
			return err
		}
	}

	c.buf.clearDirect()
	c.handshakeRC = nil
	c.handshakeKLW = nil
	return nil
}

func (c *ATLSConn) RemoteAddr() net.Addr {
	return c.remote
}

func (c *ATLSConn) LocalAddr() net.Addr {
	return c.local
}

func (c *ATLSConn) GetLocalAddr() string {
	if c.local == nil {
		return ""
	}
	if c.localAddr == "" {
		c.localAddr = c.local.String()
	}
	return c.localAddr
}

func (c *ATLSConn) GetRemoteAddr() string {
	if c.remote == nil {
		return ""
	}
	if c.remoteAddr == "" {
		c.remoteAddr = c.remote.String()
	}
	return c.remoteAddr
}

func (c *ATLSConn) SetRemote(addr net.Addr) {
	c.remote = addr
	c.remoteAddr = ""
	if c.buf != nil {
		c.buf.remote = addr
	}
}

func (c *ATLSConn) SetCloseCB(cb CloseCb) {
	c.closecb = cb
}

func (c *ATLSConn) SetOpenCB(cb OpenCb) {
	c.opencb = cb
}

func (c *ATLSConn) SetLocal(addr net.Addr) {
	c.local = addr
	c.localAddr = ""
	if c.buf != nil {
		c.buf.local = addr
	}
}

func (c *ATLSConn) readCiphertext() (int, error) {
	buf := c.buf.cin.PeekWrite(tlsCipherReadSize)
	if len(buf) == 0 {
		return 0, unix.ENOMEM
	}
	n, err := unix.Read(c.fd, buf)
	if n > 0 {
		_ = c.buf.cin.Forward(n, ringbuffer.OpWrite)
		atomic.AddInt32(&c.recv, int32(n))
	}
	if err != nil {
		if err == unix.EAGAIN {
			if n < 0 {
				n = 0
			}
			return n, unix.EAGAIN
		}
		return n, err
	}
	if n == 0 {
		return 0, errors.New("eof")
	}
	return n, nil
}

func (c *ATLSConn) Read(b []byte) (int, error) {
	if c.fd == 0 {
		return 0, unix.EBADF
	}
	if len(b) == 0 {
		return 0, nil
	}

	// Serve drained plaintext first (captured in kTLS mode and also on kTLS->std fallback).
	if c.preReadOff < len(c.preRead) {
		n := copy(b, c.preRead[c.preReadOff:])
		c.preReadOff += n
		if c.preReadOff >= len(c.preRead) {
			c.preRead = nil
			c.preReadOff = 0
		}
		if n > 0 {
			atomic.AddInt32(&c.recv, int32(n))
			return n, nil
		}
	}

	if c.ktls && c.ktlsRX {
		n, err := unix.Read(c.fd, b)
		if n < 0 {
			n = 0
		}
		if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
			n = 0
			err = unix.EAGAIN
		}
		// Some kernels may surface a clean shutdown or an encrypted close_notify
		// as EIO on the kTLS data path. Treat it as EOF for stream semantics.
		if err == unix.EIO {
			return 0, io.EOF
		}
		if n == 0 && err == nil {
			return 0, io.EOF
		}
		if n > 0 {
			atomic.AddInt32(&c.recv, int32(n))
		} else if err != nil && err != unix.EAGAIN {
			wrapper.Debugf("ktls read fd=%d n=%d err=%v", c.fd, n, err)
		}
		return n, err
	}

	for {
		n, err := c.Conn.Read(b)
		if n > 0 {
			return n, nil
		}
		if err == nil {
			// Non-blocking BIO path may transiently return (0, nil) when there is
			// currently no decrypted app data. Treat it as would-block so caller
			// does not misclassify it as a read error.
			return 0, unix.EAGAIN
		}
		if err != errWouldBlock {
			return 0, err
		}

		_, rerr := c.readCiphertext()
		if rerr != nil {
			if rerr == unix.EAGAIN {
				return 0, unix.EAGAIN
			}
			return 0, rerr
		}
	}
}

func (c *ATLSConn) Write(b []byte) (int, error) {
	if c.fd == 0 {
		return 0, unix.EBADF
	}
	if len(b) == 0 {
		return 0, nil
	}

	if c.ktls {
		n, err := unix.Write(c.fd, b)
		if n < 0 {
			n = 0
		}
		if err == unix.EAGAIN {
			n = 0
		}
		if n > 0 {
			atomic.AddInt32(&c.send, int32(n))
		}
		return n, err
	}

	// For aTLS, Write encrypts plaintext into c.buf.cout.
	// LoopConn will flush ciphertext via FlushN in the same event cycle.
	return c.Conn.Write(b)
}

func (c *ATLSConn) Close() error {
	c.clearWrite()
	if c.Conn != nil {
		_ = c.Conn.Close()
	}
	if c.buf != nil {
		c.buf.cin.Reset()
		c.buf.cout.Reset()
	}
	if c.closecb != nil {
		c.closecb.OnClose(c.ctx)
	}
	err := unix.Close(c.fd)
	c.fd = 0
	return err
}

func (c *ATLSConn) Open() {
	if c.opencb != nil {
		c.opencb.OnOpen(c.ctx)
	}
}

func (c *ATLSConn) SetDeadline(t time.Time) error {
	return nil
}

func (c *ATLSConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *ATLSConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func (c *ATLSConn) GetType() int {
	return TYPE_CONN_TLS
}

func (c *ATLSConn) IsKTLS() bool {
	return c != nil && c.ktls
}

func (c *ATLSConn) IsKTLSRX() bool {
	return c != nil && c.ktls && c.ktlsRX
}

func (c *ATLSConn) BufferedPlaintextLen() int {
	if c == nil {
		return 0
	}
	if c.preReadOff >= len(c.preRead) {
		return 0
	}
	return len(c.preRead) - c.preReadOff
}

func (c *ATLSConn) PendingWrite() int {
	pending := c.Base.PendingWrite()
	if c.buf != nil {
		pending += c.buf.cout.Has()
	}
	return pending
}

func (c *ATLSConn) FlushN(maxBytes int) (int, error) {
	if c.fd == 0 {
		return 0, unix.EBADF
	}
	if c.ktls {
		return flushWriteQueue(c.fd, &c.wq, maxBytes, &c.send)
	}
	if c.buf == nil {
		return 0, errors.New("tls buffer conn not initialized")
	}

	flushed := 0
	limit := maxBytes > 0
	remaining := maxBytes

	for {
		if limit && remaining <= 0 {
			return flushed, nil
		}

		if c.buf.cout.Has() > 0 {
			head, tail := c.buf.cout.Peek2()
			if limit {
				if remaining <= 0 {
					return flushed, nil
				}
				if len(head) > remaining {
					head = head[:remaining]
					tail = nil
				} else if len(head)+len(tail) > remaining {
					tail = tail[:remaining-len(head)]
				}
			}

			var iovecs [tlsMaxWritevIovecs]unix.Iovec
			iovcnt := 0
			if len(head) > 0 {
				iovecs[iovcnt].Base = &head[0]
				iovecs[iovcnt].SetLen(len(head))
				iovcnt++
			}
			if len(tail) > 0 && iovcnt < len(iovecs) {
				iovecs[iovcnt].Base = &tail[0]
				iovecs[iovcnt].SetLen(len(tail))
				iovcnt++
			}

			n, err := writev(c.fd, iovecs[:iovcnt])
			if err != nil && err == unix.EAGAIN && n < 0 {
				n = 0
			}
			if n > 0 {
				_ = c.buf.cout.Forward(n, ringbuffer.OpRead)
				atomic.AddInt32(&c.send, int32(n))
				flushed += n
				if limit {
					remaining -= n
				}
				continue
			}

			if err != nil {
				if err == unix.EAGAIN {
					return flushed, unix.EAGAIN
				}
				return flushed, err
			}
			return flushed, nil
		}

		buf := c.peekWrite()
		if len(buf) == 0 {
			return flushed, nil
		}

		chunkLimit := tlsWriteChunkSize
		if limit && remaining < chunkLimit {
			chunkLimit = remaining
		}
		if chunkLimit <= 0 {
			return flushed, nil
		}

		src := buf
		if len(src) > chunkLimit {
			src = src[:chunkLimit]
		}

		n, err := c.Conn.Write(src)
		if n > 0 {
			c.consumeWrite(n)
		}
		if err != nil {
			return flushed, err
		}
	}
}

func (c *ATLSConn) ParsePacket(in *[]byte) (length, expect int) {
	return c.h.ParsePacket(c, in)
}

func (c *ATLSConn) OnData(in *[]byte) (out []byte, close bool) {
	return c.h.OnData(c, in)
}

func (c *ATLSConn) Destroy() {}

func recordCipherSuite(state tls.ConnectionState) {
	name := tls.CipherSuiteName(state.CipherSuite)
	if name == "" {
		name = "unknown"
	}
	wrapper.Increment("tls.cipher." + name)
}
