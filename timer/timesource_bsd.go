//go:build darwin || netbsd || freebsd || openbsd || dragonfly
// +build darwin netbsd freebsd openbsd dragonfly

package timer

import (
	"sync"
	"sync/atomic"
	"time"
)

type TimeSource struct {
	id       int
	interval int
	seq      uint32
	twMap    sync.Map
	pet      sync.Map
}

func CreateTimeSource() *TimeSource {
	return new(TimeSource)
}

func (t *TimeSource) Open(id, msec int) {
	t.id = id
	if msec < 100 {
		t.interval = 100
	} else {
		t.interval = msec
	}
	go t.poll()
}

func (t *TimeSource) Stop() {
	t.twMap.Range(func(k, v interface{}) bool {
		fd := k.(uint32)
		t.twMap.Delete(fd)
		t.pet.Delete(fd)
		return true
	})
}

// Interval 不为0则表示是周期性定时器。
// Value 和Interval都为0表示停止定时器。
func (t *TimeSource) AddTimerWheel(tw ITimerWheel) (int, error) {
	id := atomic.AddUint32(&t.seq, 1)
	t.pet.Store(id, time.Now().UnixNano()/1000000)

	tw.SetFD(int(id))
	t.twMap.Store(id, tw)

	return int(id), nil
}

func (t *TimeSource) DelTimerWheel(tw ITimerWheel) {
	fd := tw.GetFD()
	id := uint32(fd)
	t.twMap.Delete(id)
	t.pet.Delete(id)
}

func (t *TimeSource) poll() {
	timer := time.NewTicker(time.Millisecond * time.Duration(t.interval))
	defer timer.Stop()

	for {
		now := <-timer.C
		ms := now.UnixNano() / 1000000
		t.pet.Range(func(k, v interface{}) bool {
			fd := k.(uint32)
			last := v.(int64)

			val, ok := t.twMap.Load(fd)
			if ok {
				tw := val.(ITimerWheel)
				if ms-last >= tw.GetInterval() {
					t.pet.Store(fd, ms)
					tw.OnTimeOut()
				}
			} else {
				t.pet.Delete(fd)
			}
			return true
		})

	}
}
