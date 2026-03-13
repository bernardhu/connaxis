package connection

import (
	"net"
	"sync/atomic"
	"time"

	"github.com/bernardhu/connaxis/ringbuffer"
	"github.com/bernardhu/connaxis/timer"
)

// AppConn is the application-facing connection surface.
// Keep this interface narrow for handlers/business code.
type AppConn interface {
	ID() uint64
	Context() interface{}
	SetContext(interface{})

	Close() error
	AddCmd(cmd int, data []byte) error

	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	GetLocalAddr() string
	GetRemoteAddr() string
	IsClient() bool
}

// ProtoConn is the protocol-facing connection surface.
// It extends AppConn with packet-handler routing APIs.
type ProtoConn interface {
	AppConn
	SetPktHandler(h IPktHandler)
	UpdatePktHandler(h IPktHandler)
	GetPktHandler() IPktHandler
}

// EngineConn is the full engine/internal connection surface.
// It should only be consumed by eventloop/engine internals.
type EngineConn interface {
	ProtoConn
	Fd() int
	LoopID() int
	Recvbuf() *ringbuffer.RingBuffer
	GetType() int
	FlushN(maxBytes int) (int, error)
	PendingWrite() int
	SetID(id uint64)
	SetLoopID(loopID int)
	SetReceiver(recv ICmdRecv)
	Destroy()

	SetAcceptAddr(string)

	Open()
	Read(b []byte) (int, error)
	Write(b []byte) (int, error)
	AcceptAddr() string
	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	SetRemote(addr net.Addr)
	SetLocal(addr net.Addr)

	ParsePacket(in *[]byte) (length, expect int)
	OnData(in *[]byte) (out []byte, close bool)

	EnqueueWrite(buf []byte, size int)
	GetSend(reset bool) int32
	GetRecv(reset bool) int32

	SetLastRecv(ts int64)
	GetLastRecv() int64

	SetListenerEndpoint(interface{})
	GetListenerEndpoint() interface{}
}

type ICmdRecv interface {
	AddCmd(cmd, fd int, dial bool, id uint64, data []byte) error
}

type IPktHandler interface {
	// ParsePacket returns:
	//   - length > 0: a complete packet of "length" bytes is available at (*in)[:length]
	//   - length == 0: need more data (partial packet)
	//   - length < 0: fatal parse error (connection will be closed)
	// expect is a hint of the expected total packet size (or -1 if unknown).
	ParsePacket(c ProtoConn, in *[]byte) (length, expect int)
	OnData(c ProtoConn, in *[]byte) (out []byte, close bool)
	Stat(bool)
}

type ITimerWatcher interface {
	AddTimer(t timer.ITimer) bool
	DelTimer(t timer.ITimer)
	RefreshTimer(t timer.ITimer) bool
}

const (
	TYPE_CONN = iota
	TYPE_CONN_OPENSSL
	TYPE_CONN_TLS
)

const (
	CMD_NOP = iota
	CMD_DATA
	CMD_CLOSE
)

// Conn ...
type Base struct {
	ctx interface{}
	fd  int
	id  uint64
	// loopID is the owner event loop index.
	loopID int

	h           IPktHandler
	cmdReceiver ICmdRecv
	recvbuf     *ringbuffer.RingBuffer //recvbuf
	local       net.Addr
	remote      net.Addr
	localAddr   string
	remoteAddr  string
	acceptAddr  string
	Client      bool
	ep          interface{}

	recv int32
	send int32

	lastRecv int64

	wq writeQueue
}

type CloseCb interface {
	OnClose(interface{})
}

type OpenCb interface {
	OnOpen(interface{})
}

func (c *Base) Fd() int {
	return c.fd
}

func (c *Base) LoopID() int {
	return c.loopID
}

func (c *Base) ID() uint64 {
	return c.id
}

func (c *Base) Recvbuf() *ringbuffer.RingBuffer {
	return c.recvbuf
}

func (c *Base) Context() interface{} {
	return c.ctx
}

func (c *Base) SetFd(fd int) {
	c.fd = fd
}

func (c *Base) SetID(id uint64) {
	c.id = id
}

func (c *Base) SetLoopID(loopID int) {
	c.loopID = loopID
}

func (c *Base) SetRecvbuf(buf *ringbuffer.RingBuffer) {
	c.recvbuf = buf
}

func (c *Base) SetContext(itf interface{}) {
	c.ctx = itf
}

func (c *Base) SetReceiver(recv ICmdRecv) {
	c.cmdReceiver = recv
}

func (c *Base) SetPktHandler(h IPktHandler) {
	if c.h == nil {
		c.h = h
	}
}

func (c *Base) UpdatePktHandler(h IPktHandler) {
	c.h = h
}

func (c *Base) GetPktHandler() IPktHandler {
	return c.h
}

func (c *Base) AddCmd(cmd int, data []byte) error {
	return c.cmdReceiver.AddCmd(cmd, c.fd, c.Client, c.id, data)
}

func (c *Base) SetAcceptAddr(addr string) {
	c.acceptAddr = addr
}

func (c *Base) AcceptAddr() string {
	return c.acceptAddr
}

func (c *Base) IsClient() bool {
	return c.Client
}

func (c *Base) GetSend(reset bool) int32 {
	val := atomic.LoadInt32(&c.send)
	if reset {
		atomic.AddInt32(&c.send, -val)
	}
	return val
}

func (c *Base) GetRecv(reset bool) int32 {
	val := atomic.LoadInt32(&c.recv)
	if reset {
		atomic.AddInt32(&c.recv, -val)
	}
	return val
}

func (c *Base) SetLastRecv(ts int64) {
	c.lastRecv = ts
}

func (c *Base) GetLastRecv() int64 {
	return c.lastRecv
}

func (c *Base) SetListenerEndpoint(ep interface{}) {
	c.ep = ep
}

func (c *Base) GetListenerEndpoint() interface{} {
	return c.ep
}

// PendingWrite returns total pending bytes in the write queue.
func (c *Base) PendingWrite() int {
	return c.wq.pending()
}

// EnqueueWrite enqueues a buffer for writev flush.
func (c *Base) EnqueueWrite(buf []byte, size int) {
	c.wq.enqueue(buf, size)
}

func (c *Base) peekWrite() []byte {
	return c.wq.peek()
}

func (c *Base) consumeWrite(n int) {
	c.wq.consume(n)
}

func (c *Base) clearWrite() {
	c.wq.clear()
}
