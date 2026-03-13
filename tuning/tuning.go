package tuning

// EventLoop budgets.
type EventLoop struct {
	MaxAcceptPerEvent        int   // accepts per poll
	MaxReadBytesPerEvent     int   // read bytes per poll
	MaxFlushBytesPerEvent    int   // flush bytes per poll
	MaxCmdPerEvent           int   // cmds per poll
	MaxCmdFlushConnsPerEvent int   // conns flushed per poll
	AcceptSocketNoDelay      bool  // set TCP_NODELAY on accepted sockets
	AcceptSocketSendBufBytes int   // set SO_SNDBUF on accepted sockets (0 = keep system default)
	TlsHandshakeWorkers      int   // handshake workers (0 => GOMAXPROCS)
	TlsHandshakeQueueSize    int   // handshake queue
	TlsHandshakeMaxPending   int32 // handshake cap
}

// Connection write-queue knobs.
type Connection struct {
	WriteQueueCoalesceMaxBytes int // coalesce small writes
	WriteQueueCompactMaxBytes  int // compact tail
	WriteQueueShrinkCap        int // shrink items cap
}

// Profile is a preset.
type Profile struct {
	Name       string
	EventLoop  EventLoop
	Connection Connection
}

// Live globals used by eventloop/connection.
var (
	MaxAcceptPerEvent        = 128
	MaxReadBytesPerEvent     = 256 * 1024
	MaxFlushBytesPerEvent    = 256 * 1024
	MaxCmdPerEvent           = 1024
	MaxCmdFlushConnsPerEvent = 1024
	AcceptSocketNoDelay      = true
	AcceptSocketSendBufBytes = 0

	TlsHandshakeWorkers          = 0
	TlsHandshakeQueueSize        = 1024
	TlsHandshakeMaxPending int32 = 4096

	WriteQueueCoalesceMaxBytes = 128
	WriteQueueCompactMaxBytes  = 4 * 1024
	WriteQueueShrinkCap        = 4 * 1024

	DefProfiles = map[string]Profile{
		"balanced":   ProfileBalanced(),
		"throughput": ProfileThroughput(),
		"latency":    ProfileLatency(),
		"gateway":    ProfileGateway(),
	}
)

// Apply overwrites globals.
func Apply(p Profile) {
	MaxAcceptPerEvent = p.EventLoop.MaxAcceptPerEvent
	MaxReadBytesPerEvent = p.EventLoop.MaxReadBytesPerEvent
	MaxFlushBytesPerEvent = p.EventLoop.MaxFlushBytesPerEvent
	MaxCmdPerEvent = p.EventLoop.MaxCmdPerEvent
	MaxCmdFlushConnsPerEvent = p.EventLoop.MaxCmdFlushConnsPerEvent
	AcceptSocketNoDelay = p.EventLoop.AcceptSocketNoDelay
	AcceptSocketSendBufBytes = p.EventLoop.AcceptSocketSendBufBytes
	TlsHandshakeWorkers = p.EventLoop.TlsHandshakeWorkers
	TlsHandshakeQueueSize = p.EventLoop.TlsHandshakeQueueSize
	TlsHandshakeMaxPending = p.EventLoop.TlsHandshakeMaxPending

	WriteQueueCoalesceMaxBytes = p.Connection.WriteQueueCoalesceMaxBytes
	WriteQueueCompactMaxBytes = p.Connection.WriteQueueCompactMaxBytes
	WriteQueueShrinkCap = p.Connection.WriteQueueShrinkCap
}

// ApplyName applies a built-in profile.
func ApplyName(name string) bool {
	p, ok := DefProfiles[name]
	if !ok {
		return false
	}
	Apply(p)
	return true
}

// ProfileBalanced keeps defaults.
func ProfileBalanced() Profile {
	return Profile{
		Name: "balanced",
		EventLoop: EventLoop{
			MaxAcceptPerEvent:        128,
			MaxReadBytesPerEvent:     256 * 1024,
			MaxFlushBytesPerEvent:    256 * 1024,
			MaxCmdPerEvent:           1024,
			MaxCmdFlushConnsPerEvent: 1024,
			AcceptSocketNoDelay:      true,
			AcceptSocketSendBufBytes: 0,
			TlsHandshakeWorkers:      0,
			TlsHandshakeQueueSize:    1024,
			TlsHandshakeMaxPending:   4096,
		},
		Connection: Connection{
			WriteQueueCoalesceMaxBytes: 128,
			WriteQueueCompactMaxBytes:  4 * 1024,
			WriteQueueShrinkCap:        4 * 1024,
		},
	}
}

// ProfileThroughput favors throughput.
func ProfileThroughput() Profile {
	return Profile{
		Name: "throughput",
		EventLoop: EventLoop{
			MaxAcceptPerEvent:        256,
			MaxReadBytesPerEvent:     512 * 1024,
			MaxFlushBytesPerEvent:    512 * 1024,
			MaxCmdPerEvent:           2048,
			MaxCmdFlushConnsPerEvent: 2048,
			AcceptSocketNoDelay:      true,
			AcceptSocketSendBufBytes: 0,
			TlsHandshakeWorkers:      0,
			TlsHandshakeQueueSize:    2048,
			TlsHandshakeMaxPending:   8192,
		},
		Connection: Connection{
			WriteQueueCoalesceMaxBytes: 512,
			WriteQueueCompactMaxBytes:  16 * 1024,
			WriteQueueShrinkCap:        4 * 1024,
		},
	}
}

// ProfileLatency favors tail latency.
func ProfileLatency() Profile {
	return Profile{
		Name: "latency",
		EventLoop: EventLoop{
			MaxAcceptPerEvent:        64,
			MaxReadBytesPerEvent:     64 * 1024,
			MaxFlushBytesPerEvent:    128 * 1024,
			MaxCmdPerEvent:           512,
			MaxCmdFlushConnsPerEvent: 512,
			AcceptSocketNoDelay:      true,
			AcceptSocketSendBufBytes: 0,
			TlsHandshakeWorkers:      0,
			TlsHandshakeQueueSize:    1024,
			TlsHandshakeMaxPending:   4096,
		},
		Connection: Connection{
			WriteQueueCoalesceMaxBytes: 256,
			WriteQueueCompactMaxBytes:  8 * 1024,
			WriteQueueShrinkCap:        2 * 1024,
		},
	}
}

// ProfileGateway targets throughput with bounded tail latency.
func ProfileGateway() Profile {
	return Profile{
		Name: "gateway",
		EventLoop: EventLoop{
			MaxAcceptPerEvent:        128,
			MaxReadBytesPerEvent:     64 * 1024,
			MaxFlushBytesPerEvent:    128 * 1024,
			MaxCmdPerEvent:           512,
			MaxCmdFlushConnsPerEvent: 512,
			AcceptSocketNoDelay:      true,
			AcceptSocketSendBufBytes: 0,
			TlsHandshakeWorkers:      0,
			TlsHandshakeQueueSize:    1024,
			TlsHandshakeMaxPending:   4096,
		},
		Connection: Connection{
			WriteQueueCoalesceMaxBytes: 256,
			WriteQueueCompactMaxBytes:  8 * 1024,
			WriteQueueShrinkCap:        2 * 1024,
		},
	}
}
