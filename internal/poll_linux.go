package internal

import (
	"sync/atomic"
	"time"

	"github.com/bernardhu/connaxis/wrapper"
	"golang.org/x/sys/unix"
)

// Poll ...
type Poll struct {
	id  int // id
	fd  int // epoll fd
	wfd int // wake fd

	wait     int
	waitDone chan struct{}
	stopping int32
}

// OpenPoll ...
func OpenPoll(id int) *Poll {
	var err error
	l := new(Poll)
	l.id = id
	l.wait = -1

	l.fd, err = unix.EpollCreate1(0)
	if err != nil {
		wrapper.Debugf("open poll:%d fail, err:%v", id, err)
		return nil
	}

	r0, _, errno := unix.Syscall(unix.SYS_EVENTFD2, 0, 0, 0)
	if errno != 0 {
		unix.Close(l.fd)
		wrapper.Debugf("open eventfd poll:%d fail, err:%v", id, err)
		return nil
	}
	l.wfd = int(r0)

	if l.AddRead(l.wfd) != nil {
		_ = unix.Close(l.fd)
		_ = unix.Close(l.wfd)

		wrapper.Debugf("open poll:%d add read fail", id)
		return nil
	}

	l.waitDone = make(chan struct{})
	return l
}

// Close ...
func (p *Poll) Close() error {
	atomic.StoreInt32(&p.stopping, 1)
	if err := p.Trigger(CmdUser); err != nil {
		wrapper.Debugf("close poll: %d fail, err:%v", p.id, err)
		return err
	}

	<-p.waitDone
	return nil
}

// Trigger ...
func (p *Poll) Trigger(cmd byte) error {
	var wakeBytes = []byte{1, 0, 0, 0, 0, 0, 0, 0}
	wakeBytes[0] = cmd
	_, err := unix.Write(p.wfd, wakeBytes)
	return err
}

// SetPollWait ...
func (p *Poll) SetPollWait(msec int) {
	p.wait = msec
	if p.wait < 0 {
		p.wait = -1
	}
}

// Poll ...
func (p *Poll) Poll(handler func(fd int, event Event)) {
	defer func() {
		unix.Close(p.wfd)
		unix.Close(p.fd)
		close(p.waitDone)
	}()

	evs := make([]unix.EpollEvent, PollInitSize)
	buf := make([]byte, 8)
	full := 0

	for {
		//wrapper.Debugf("poll: %d start to wait len:%d wait:%d", p.id, len(evs), p.wait)
		n, err := unix.EpollWait(p.fd, evs, p.wait)
		if err != nil && err != unix.EINTR && err != unix.EAGAIN {
			wrapper.Errorf("epollwait err ", err)
			continue
		}

		//wrapper.Debugf("poll: %d recv: %d event", p.id, n)
		for i := 0; i < n; i++ {
			if fd := int(evs[i].Fd); fd != p.wfd {
				var rEvents Event
				if ((evs[i].Events & unix.POLLHUP) != 0) && ((evs[i].Events & unix.POLLIN) == 0) {
					rEvents |= EventErr
				}
				if (evs[i].Events&unix.EPOLLERR != 0) || (evs[i].Events&unix.EPOLLOUT != 0) {
					rEvents |= EventWrite
				}
				if evs[i].Events&(unix.EPOLLIN|unix.EPOLLPRI|unix.EPOLLRDHUP) != 0 {
					rEvents |= EventRead
				}

				//wrapper.Debugf("poll: %d fd: %d get event: %d event", p.id, fd, rEvents)
				handler(fd, rEvents)
			} else {
				rn, re := unix.Read(p.wfd, buf)
				if re != nil || rn != 8 {
					wrapper.Errorf("poll wfd read err:%v, size:%d", re, rn)
				}

				if atomic.LoadInt32(&p.stopping) == 1 {
					handler(0, EventStop)
					return
				}
				handler(0, EventNone)
			}
		}

		handler(0, EventIdle)

		if n == len(evs) {
			full = full + 1
			if full == 3 {
				if len(evs) < PollMaxSize {
					evs = make([]unix.EpollEvent, 2*len(evs))
					wrapper.Debugf("poll: %d evs resize to: %d", p.id, len(evs))
				}
				full = 0
			}
		} else {
			full = 0
		}

		if n <= 0 {
			time.Sleep(time.Millisecond)
		}
	}
}

func (p *Poll) add(fd int, events uint32) error {
	var ev unix.EpollEvent
	ev.Fd = int32(fd)
	ev.Events = events

	err := unix.EpollCtl(p.fd, unix.EPOLL_CTL_ADD, fd, &ev)
	if err == unix.EEXIST {
		err = unix.EpollCtl(p.fd, unix.EPOLL_CTL_MOD, fd, &ev)
	}

	return err
}

// AddReadWrite ...
func (p *Poll) AddReadWrite(fd int) error {
	//wrapper.Debugf("poll: %d fd: %d set rw", p.id, fd)
	return p.add(fd, unix.EPOLLIN|unix.EPOLLPRI|unix.EPOLLOUT)
}

// AddRead ...
func (p *Poll) AddRead(fd int) error {
	//wrapper.Debugf("poll: %d fd: %d set r", p.id, fd)
	return p.add(fd, unix.EPOLLIN|unix.EPOLLPRI)
}

func (p *Poll) mod(fd int, events uint32) error {
	var ev unix.EpollEvent
	ev.Fd = int32(fd)
	ev.Events = events

	err := unix.EpollCtl(p.fd, unix.EPOLL_CTL_MOD, fd, &ev)
	//wrapper.Debugf("poll: %d fd: %d first mod err:%v", p.id, fd, err)
	if err == unix.ENOENT {
		err = unix.EpollCtl(p.fd, unix.EPOLL_CTL_ADD, fd, &ev)
		//wrapper.Debugf("poll: %d fd: %d then add err:%v", p.id, fd, err)
	}

	return err
}

// ModRead ...
func (p *Poll) ModRead(fd int) error {
	//wrapper.Debugf("poll: %d fd: %d mod r", p.id, fd)
	return p.mod(fd, unix.EPOLLIN|unix.EPOLLPRI)
}

// ModReadWrite ...
func (p *Poll) ModReadWrite(fd int) error {
	//wrapper.Debugf("poll: %d fd: %d mod rw", p.id, fd)
	return p.mod(fd, unix.EPOLLIN|unix.EPOLLPRI|unix.EPOLLOUT)
}

// Del ...
func (p *Poll) Del(fd int) error {
	//wrapper.Debugf("poll: %d fd: %d del", p.id, fd)
	return unix.EpollCtl(p.fd, unix.EPOLL_CTL_DEL, fd, nil)
}
