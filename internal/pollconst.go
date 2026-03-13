package internal

// event poll size
const (
	PollInitSize = 1024
	PollMaxSize  = 8192
)

// Event poller 返回事件
type Event uint32

// Event poller 返回事件值
const (
	EventNone Event = 0x0
	EventStop Event = 0xff
	EventIdle Event = 0xfe

	EventRead  Event = 0x1
	EventWrite Event = 0x2
	EventErr   Event = 0x80
)

const (
	CmdStop byte = 0x0
	CmdUser byte = 0x1
)
