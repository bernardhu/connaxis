package timer

import (
	"sync"
	"time"

	"github.com/bernardhu/connaxis/internal"
	"github.com/bernardhu/connaxis/wrapper"
	"golang.org/x/sys/unix"
)

type TimeSource struct {
	id    int
	poll  *internal.Poll
	twMap sync.Map
}

func CreateTimeSource() *TimeSource {
	return new(TimeSource)
}

func (t *TimeSource) Open(id, msec int) {
	t.id = id
	t.poll = internal.OpenPoll(t.id)
	t.poll.SetPollWait(msec)
	go t.poll.Poll(t.handler)
}

func (t *TimeSource) Stop() {
	t.twMap.Range(func(k, v interface{}) bool {
		fd := k.(int)
		_ = t.poll.Del(fd)
		t.twMap.Delete(fd)
		return true
	})

	t.poll.Close()
}

// Interval 不为0则表示是周期性定时器。
// Value 和Interval都为0表示停止定时器。
func (t *TimeSource) AddTimerWheel(tw ITimerWheel) (int, error) {
	fd, err := unix.TimerfdCreate(unix.CLOCK_MONOTONIC, unix.TFD_NONBLOCK)
	if err != nil {
		wrapper.Debugf("create timefd fail, err:%v", err)
		return -1, err
	}

	var spec unix.ItimerSpec
	interval := tw.GetInterval()
	spec.Value.Sec = interval / int64(time.Second)
	spec.Value.Nsec = interval % int64(time.Second)
	spec.Interval.Sec = interval / int64(time.Second)
	spec.Interval.Nsec = interval % int64(time.Second)

	wrapper.Debugf("create timer interval sec:%d ns:%d", spec.Interval.Sec, spec.Interval.Nsec)
	err = unix.TimerfdSettime(fd, 0, &spec, nil)
	if err != nil {
		wrapper.Debugf("create timer time fail, err:%v", err)
		return -1, err
	}

	err = t.poll.AddRead(fd)
	if err != nil {
		wrapper.Debugf("add fd fail, err:%v", err)
		return -1, err
	}

	tw.SetFD(fd)
	t.twMap.Store(fd, tw)

	return fd, nil
}

func (t *TimeSource) DelTimerWheel(tw ITimerWheel) {
	fd := tw.GetFD()
	_ = t.poll.Del(fd)
	t.twMap.Delete(fd)
}

func (t *TimeSource) handler(fd int, event internal.Event) {
	var elapse [8]byte
	if fd != 0 {
		_, _ = unix.Read(fd, elapse[:])
		//n, err := unix.Read(fd, elapse[:])
		//wrapper.Debugf("TimeSource: %d fd:%d read %d, err:%v", t.id, fd, n, err)
		var tw ITimerWheel
		val, ok := t.twMap.Load(fd)
		if ok {
			tw = val.(ITimerWheel)
		} else {
			_ = t.poll.Del(fd)
		}

		tw.OnTimeOut()
	} else {
		if event == internal.EventStop {
			t.twMap.Range(func(k, v interface{}) bool {
				fd := k.(int)
				_ = t.poll.Del(fd)
				t.twMap.Delete(fd)
				return true
			})
		}
	}
}
