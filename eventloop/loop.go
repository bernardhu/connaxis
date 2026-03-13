package eventloop

import (
	"errors"
	"net"
	"sync"

	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/internal/tls"
	"github.com/bernardhu/connaxis/pool"
)

var CLOSE_EPOLL_ERR = errors.New("EPOLL_ERROR")
var CLOSE_SEND_ERR = errors.New("SEND_ERROR")
var CLOSE_READ_ERR = errors.New("READ_ERROR")
var CLOSE_PKT_SIZE_LIMIT = errors.New("PKT_SIZE_LIMIT")
var CLOSE_PKT_PARSE_ERR = errors.New("PKT_PARSE_ERROR")
var CLOSE_MEM_ALLOC_FAIL = errors.New("MEM_ALLOC_FAIL")
var CLOSE_BY_USER_LAND = errors.New("BY_USER_LAND")
var CLOSE_BY_ANTI_IDLE = errors.New("BY_ANTI_IDLE")
var CLOSE_BY_ACCEPT = errors.New("BY_ACCEPT")
var CLOSE_BY_DUP_ACCEPT = errors.New("BY_DUP_ACCEPT")
var CLOSE_BY_CLOSE_CMD = errors.New("BY_CLOSE_CMD")
var CLOSE_BY_SLOW_READER = errors.New("BY_SLOW_READER")

type IServer interface {
	GetListenAddrs() []net.Addr
	GetWorkerNum() int
}

type IHandler interface {
	OnReady(s IServer)
	OnClosed(c connection.AppConn, err error)
	OnConnected(c connection.ProtoConn)
}

type IDGenerator interface {
	GetID() uint64
}

type ISelector interface {
	SelectLoop(id int) IEVLoop
	GetLoad() int32
}

type IListener interface {
	Fd() int
	TlsMode() int
	TlsConfig() *tls.Config
	Addr() net.Addr
	ListenAddr() string
	GetEndpoint() IEVEndpoint
	Close()
}

type IEVEndpoint interface {
	net.Addr
	IsReuse() bool
	GetContext() interface{}
}

type IEVLoop interface {
	Init(idx, size, chansize, pktsizelimit, cliSbufLimit int)
	AddListener(l IListener)
	Run()
	SetWg(wg *sync.WaitGroup)
	SetHandler(h IHandler)
	SetSelector(s ISelector)
	AddClient(c connection.EngineConn) error
	AllocID(c connection.EngineConn)
	SetPollWait(int)
	Online() int32
	DialCnt() int32
	SyncTime(int64, int64)
	SetIDGen(IDGenerator)

	Stop()
	Stat(int64, bool)
	Id() int
}

type CmdData struct {
	cmd  int
	fd   int
	id   uint64
	data []byte
	size int
}

func (c *CmdData) reset() {
	c.cmd = 0
	c.fd = 0
	c.id = 0
	if c.data != nil {
		pool.GAlloctor.Put(c.data)
		c.data = nil
	}
	c.size = 0
}
