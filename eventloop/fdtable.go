package eventloop

import (
	"sync"

	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/wrapper"
	"golang.org/x/sys/unix"
)

const (
	fdTableMinSize      = 1024
	fdTableFallbackSize = 64 * 1024
	fdTableMaxSize      = 4 * 1024 * 1024
)

type fdSlot struct {
	mu   sync.RWMutex
	conn connection.EngineConn
}

var fdTable []fdSlot

func InitFDTable() {
	size, raw := fdTableSize()
	fdTable = make([]fdSlot, size)
	wrapper.Warnf("fd table init size:%d rlimit:%d", size, raw)
}

func fdTableSize() (size int, raw uint64) {
	var lim unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &lim); err != nil {
		return fdTableFallbackSize, 0
	}

	raw = lim.Cur
	if lim.Cur == unix.RLIM_INFINITY || lim.Cur > uint64(fdTableMaxSize) {
		size = fdTableMaxSize
	} else {
		size = int(lim.Cur)
	}
	if size < fdTableMinSize {
		size = fdTableMinSize
	}
	return size, raw
}

func fdTableSet(fd int, c connection.EngineConn) bool {
	if fd < 0 || fd >= len(fdTable) {
		return false
	}
	slot := &fdTable[fd]
	slot.mu.Lock()
	slot.conn = c
	slot.mu.Unlock()
	return true
}

func fdTableGet(fd int) connection.EngineConn {
	if fd < 0 || fd >= len(fdTable) {
		return nil
	}
	slot := &fdTable[fd]
	slot.mu.RLock()
	c := slot.conn
	slot.mu.RUnlock()
	return c
}

func fdTableClear(fd int, c connection.EngineConn) {
	if fd < 0 || fd >= len(fdTable) {
		return
	}

	slot := &fdTable[fd]
	slot.mu.Lock()
	if slot.conn == c {
		slot.conn = nil
	}
	slot.mu.Unlock()
}

func fdTableRange(owner int, fn func(fd int, c connection.EngineConn)) {
	for fd := range fdTable {
		slot := &fdTable[fd]
		slot.mu.RLock()
		c := slot.conn
		slot.mu.RUnlock()
		if c == nil || c.LoopID() != owner {
			continue
		}
		fn(fd, c)
	}
}
