package internal

import (
	"runtime"
	"sync/atomic"
)

// this is a good candidate for a lock-free structure.

type Spinlock struct{ lock uintptr }

func (l *Spinlock) Lock() {
	for !atomic.CompareAndSwapUintptr(&l.lock, 0, 1) {
		runtime.Gosched()
	}
}
func (l *Spinlock) Unlock() {
	atomic.StoreUintptr(&l.lock, 0)
}

type Fakelock struct{}

func (l *Fakelock) Lock() {
}
func (l *Fakelock) Unlock() {
}
