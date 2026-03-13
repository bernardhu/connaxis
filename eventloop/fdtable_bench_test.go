package eventloop

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/bernardhu/connaxis/connection"
)

const (
	benchLoopID   = 7
	benchFDBase   = 4096
	benchConnSize = 1 << 15 // 32768
)

type rwFDSlot struct {
	mu   sync.RWMutex
	conn connection.EngineConn
}

type rwFDTable struct {
	slots []rwFDSlot
}

func newRWFDTable(size int) *rwFDTable {
	return &rwFDTable{slots: make([]rwFDSlot, size)}
}

func (t *rwFDTable) set(fd int, c connection.EngineConn) bool {
	if fd < 0 || fd >= len(t.slots) {
		return false
	}
	slot := &t.slots[fd]
	slot.mu.Lock()
	slot.conn = c
	slot.mu.Unlock()
	return true
}

func (t *rwFDTable) get(fd int) connection.EngineConn {
	if fd < 0 || fd >= len(t.slots) {
		return nil
	}
	slot := &t.slots[fd]
	slot.mu.RLock()
	c := slot.conn
	slot.mu.RUnlock()
	return c
}

func (t *rwFDTable) clear(fd int, c connection.EngineConn) {
	if fd < 0 || fd >= len(t.slots) {
		return
	}
	slot := &t.slots[fd]
	slot.mu.Lock()
	if slot.conn == c {
		slot.conn = nil
	}
	slot.mu.Unlock()
}

func buildBenchConns(b *testing.B) ([]connection.EngineConn, []int) {
	conns := make([]connection.EngineConn, benchConnSize)
	fds := make([]int, benchConnSize)
	for i := 0; i < benchConnSize; i++ {
		fd := benchFDBase + i
		c := &connection.Conn{}
		c.SetFd(fd)
		c.SetLoopID(benchLoopID)
		conns[i] = c
		fds[i] = fd
	}
	return conns, fds
}

func BenchmarkConnLookupMap(b *testing.B) {
	conns, fds := buildBenchConns(b)
	m := make(map[int]connection.EngineConn, len(fds))
	for i, fd := range fds {
		m[fd] = conns[i]
	}

	mask := len(fds) - 1
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fd := fds[i&mask]
		c := m[fd]
		if c == nil || c.LoopID() != benchLoopID {
			b.Fatalf("invalid map lookup: fd=%d", fd)
		}
	}
}

func BenchmarkConnLookupFDTable(b *testing.B) {
	conns, fds := buildBenchConns(b)
	InitFDTable()
	for i, fd := range fds {
		if !fdTableSet(fd, conns[i]) {
			b.Fatalf("fdTableSet failed: fd=%d", fd)
		}
	}

	mask := len(fds) - 1
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fd := fds[i&mask]
		c := fdTableGet(fd)
		if c == nil || c.LoopID() != benchLoopID {
			b.Fatalf("invalid fdtable lookup: fd=%d", fd)
		}
	}
}

func BenchmarkConnLookupRWTable(b *testing.B) {
	conns, fds := buildBenchConns(b)
	t := newRWFDTable(benchFDBase + benchConnSize + 64)
	for i, fd := range fds {
		if !t.set(fd, conns[i]) {
			b.Fatalf("rw table set failed: fd=%d", fd)
		}
	}

	mask := len(fds) - 1
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fd := fds[i&mask]
		c := t.get(fd)
		if c == nil || c.LoopID() != benchLoopID {
			b.Fatalf("invalid rwtable lookup: fd=%d", fd)
		}
	}
}

func BenchmarkSetClearMap(b *testing.B) {
	conns, fds := buildBenchConns(b)
	m := make(map[int]connection.EngineConn, len(fds))

	mask := len(fds) - 1
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := i & mask
		fd := fds[idx]
		c := conns[idx]
		m[fd] = c
		if m[fd] == nil {
			b.Fatalf("map set/get failed: fd=%d", fd)
		}
		delete(m, fd)
	}
}

func BenchmarkSetClearFDTable(b *testing.B) {
	conns, fds := buildBenchConns(b)
	InitFDTable()

	mask := len(fds) - 1
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := i & mask
		fd := fds[idx]
		c := conns[idx]
		if !fdTableSet(fd, c) {
			b.Fatalf("fdTableSet failed: fd=%d", fd)
		}
		if fdTableGet(fd) == nil {
			b.Fatalf("fdtable get failed: fd=%d", fd)
		}
		fdTableClear(fd, c)
	}
}

func BenchmarkSetClearRWTable(b *testing.B) {
	conns, fds := buildBenchConns(b)
	t := newRWFDTable(benchFDBase + benchConnSize + 64)

	mask := len(fds) - 1
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := i & mask
		fd := fds[idx]
		c := conns[idx]
		if !t.set(fd, c) {
			b.Fatalf("rw table set failed: fd=%d", fd)
		}
		if t.get(fd) == nil {
			b.Fatalf("rw table get failed: fd=%d", fd)
		}
		t.clear(fd, c)
	}
}

func BenchmarkParallelConnLookupMapCPU1(b *testing.B) {
	conns, fds := buildBenchConns(b)
	m := make(map[int]connection.EngineConn, len(fds))
	for i, fd := range fds {
		m[fd] = conns[i]
	}
	prev := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(prev)

	mask := len(fds) - 1
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			fd := fds[i&mask]
			i++
			c := m[fd]
			if c == nil || c.LoopID() != benchLoopID {
				b.Fatalf("invalid map-rw lookup: fd=%d", fd)
			}
		}
	})
}

func BenchmarkParallelConnLookupFDTable(b *testing.B) {
	conns, fds := buildBenchConns(b)
	InitFDTable()
	for i, fd := range fds {
		if !fdTableSet(fd, conns[i]) {
			b.Fatalf("fdTableSet failed: fd=%d", fd)
		}
	}

	mask := len(fds) - 1
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			fd := fds[i&mask]
			i++
			c := fdTableGet(fd)
			if c == nil || c.LoopID() != benchLoopID {
				b.Fatalf("invalid fdtable lookup: fd=%d", fd)
			}
		}
	})
}

func BenchmarkParallelConnLookupRWTable(b *testing.B) {
	conns, fds := buildBenchConns(b)
	t := newRWFDTable(benchFDBase + benchConnSize + 64)
	for i, fd := range fds {
		if !t.set(fd, conns[i]) {
			b.Fatalf("rw table set failed: fd=%d", fd)
		}
	}

	mask := len(fds) - 1
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			fd := fds[i&mask]
			i++
			c := t.get(fd)
			if c == nil || c.LoopID() != benchLoopID {
				b.Fatalf("invalid rwtable lookup: fd=%d", fd)
			}
		}
	})
}

func BenchmarkParallelSetClearMapCPU1(b *testing.B) {
	conns, fds := buildBenchConns(b)
	m := make(map[int]connection.EngineConn, len(fds))
	prev := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(prev)
	var gidSeq uint32
	mask := len(fds) - 1
	regionMask := 255

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		gid := int(atomic.AddUint32(&gidSeq, 1) - 1)
		base := (gid << 8) & mask
		i := 0
		for pb.Next() {
			idx := base + (i & regionMask)
			idx &= mask
			i++
			fd := fds[idx]
			c := conns[idx]

			m[fd] = c
			_ = m[fd]
			delete(m, fd)
		}
	})
}

func BenchmarkParallelSetClearFDTable(b *testing.B) {
	conns, fds := buildBenchConns(b)
	InitFDTable()
	var gidSeq uint32
	mask := len(fds) - 1
	regionMask := 255

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		gid := int(atomic.AddUint32(&gidSeq, 1) - 1)
		base := (gid << 8) & mask
		i := 0
		for pb.Next() {
			idx := base + (i & regionMask)
			idx &= mask
			i++
			fd := fds[idx]
			c := conns[idx]
			if !fdTableSet(fd, c) {
				b.Fatalf("fdtable set failed: fd=%d", fd)
			}
			if fdTableGet(fd) == nil {
				b.Fatalf("fdtable get failed: fd=%d", fd)
			}
			fdTableClear(fd, c)
		}
	})
}

func BenchmarkParallelSetClearRWTable(b *testing.B) {
	conns, fds := buildBenchConns(b)
	t := newRWFDTable(benchFDBase + benchConnSize + 64)
	var gidSeq uint32
	mask := len(fds) - 1
	regionMask := 255

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		gid := int(atomic.AddUint32(&gidSeq, 1) - 1)
		base := (gid << 8) & mask
		i := 0
		for pb.Next() {
			idx := base + (i & regionMask)
			idx &= mask
			i++
			fd := fds[idx]
			c := conns[idx]
			if !t.set(fd, c) {
				b.Fatalf("rwtable set failed: fd=%d", fd)
			}
			if t.get(fd) == nil {
				b.Fatalf("rwtable get failed: fd=%d", fd)
			}
			t.clear(fd, c)
		}
	})
}
