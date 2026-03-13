//go:build darwin || netbsd || freebsd || openbsd || dragonfly
// +build darwin netbsd freebsd openbsd dragonfly

package internal

import (
	"github.com/bernardhu/connaxis/wrapper"
	"golang.org/x/sys/unix"
)

// Poll ...
type Poll struct {
	id int // id
	fd int

	waitDone chan struct{}
	wait     *unix.Timespec
}

// OpenPoll ...
func OpenPoll(id int) *Poll {
	var err error
	l := new(Poll)
	l.id = id

	l.fd, err = unix.Kqueue()
	if err != nil {
		return nil
	}

	_, err = unix.Kevent(l.fd, []unix.Kevent_t{{
		Ident:  0,
		Filter: unix.EVFILT_USER,
		Flags:  unix.EV_ADD | unix.EV_CLEAR,
	}}, nil, nil)
	if err != nil {
		_ = unix.Close(l.fd)
		return nil
	}

	l.waitDone = make(chan struct{})
	return l
}

// Close ...
func (p *Poll) Close() error {
	if err := p.Trigger(0x0); err != nil {
		wrapper.Debugf("close poll: %d fail, err:%v", p.id, err)
		return err
	}

	<-p.waitDone
	return nil
}

// Trigger ...
func (p *Poll) Trigger(cmd byte) error {
	_, err := unix.Kevent(p.fd, []unix.Kevent_t{{
		Ident:  0,
		Filter: unix.EVFILT_USER,
		Fflags: unix.NOTE_TRIGGER,
		Data:   int64(cmd),
	}}, nil, nil)
	return err
}

// SetPoolWait ...
func (p *Poll) SetPollWait(msec int) {
	if msec < 0 {
		p.wait = nil
		return
	}
	if p.wait == nil {
		p.wait = &unix.Timespec{}
	}
	p.wait.Sec = int64(msec / 1000)
	p.wait.Nsec = int64((msec % 1000) * 1000)
}

// Wait ...
func (p *Poll) Poll(handler func(fd int, event Event)) {
	defer func() {
		unix.Close(p.fd)
		close(p.waitDone)
	}()

	evs := make([]unix.Kevent_t, PollInitSize)
	full := 0
	for {
		n, err := unix.Kevent(p.fd, nil, evs, p.wait)
		if err != nil && err != unix.EINTR && err != unix.EAGAIN {
			wrapper.Errorf("kqueue err %v", err)
			continue
		}

		//wrapper.Debugf("poll: %d recv: %d event", p.id, n)
		for i := 0; i < n; i++ {
			if fd := int(evs[i].Ident); fd != 0 {
				var rEvents Event
				if (evs[i].Flags&unix.EV_ERROR != 0) || (evs[i].Flags&unix.EV_EOF != 0) {
					rEvents |= EventErr
				}
				if evs[i].Filter == unix.EVFILT_WRITE {
					rEvents |= EventWrite
				}
				if evs[i].Filter == unix.EVFILT_READ {
					rEvents |= EventRead
				}

				//wrapper.Debugf("poll: %d fd:%d get event: %d event", p.id, fd, rEvents)
				handler(fd, rEvents)
			} else {
				data := byte(evs[i].Data & 0xff)
				//wrapper.Debugf("poll: %d wfd get cmd: %d", p.id, data)
				if data == 0x0 { //stop cmd
					handler(0, EventStop)
					return
				}
				handler(0, EventNone)
			}
		}

		if n == len(evs) {
			full = full + 1
			if full == 3 {
				if len(evs) < PollMaxSize {
					evs = make([]unix.Kevent_t, 2*len(evs))
					wrapper.Debugf("poll: %d evs resize to: %d", p.id, len(evs))
				}
				full = 0
			}
		} else {
			full = 0
		}
	}
}

// AddRead ...
func (p *Poll) AddRead(fd int) error {
	//wrapper.Debugf("poll: %d fd:%d set r", p.id, fd)
	_, err := unix.Kevent(p.fd, []unix.Kevent_t{
		{Ident: uint64(fd), Flags: unix.EV_ADD, Filter: unix.EVFILT_READ},
	}, nil, nil)
	return err
}

// AddReadWrite ...
func (p *Poll) AddReadWrite(fd int) error {
	//wrapper.Debugf("poll: %d fd:%d set rw", p.id, fd)
	_, err := unix.Kevent(p.fd, []unix.Kevent_t{
		{Ident: uint64(fd), Flags: unix.EV_ADD, Filter: unix.EVFILT_READ},
		{Ident: uint64(fd), Flags: unix.EV_ADD, Filter: unix.EVFILT_WRITE},
	}, nil, nil)
	return err
}

// ModRead ...
func (p *Poll) ModRead(fd int) error {
	//wrapper.Debugf("poll: %d fd:%d mod r", p.id, fd)
	_, err := unix.Kevent(p.fd, []unix.Kevent_t{
		{Ident: uint64(fd), Flags: unix.EV_DELETE, Filter: unix.EVFILT_WRITE},
	}, nil, nil)
	return err
}

// ModReadWrite ...
func (p *Poll) ModReadWrite(fd int) error {
	//wrapper.Debugf("poll: %d fd:%d mod rw", p.id, fd)
	_, err := unix.Kevent(p.fd, []unix.Kevent_t{
		{Ident: uint64(fd), Flags: unix.EV_ADD, Filter: unix.EVFILT_WRITE},
	}, nil, nil)
	return err
}

// ModDetach ...
func (p *Poll) Del(fd int) error {
	//wrapper.Debugf("poll: %d fd:%d del", p.id, fd)
	_, err := unix.Kevent(p.fd, []unix.Kevent_t{
		{Ident: uint64(fd), Flags: unix.EV_DELETE, Filter: unix.EVFILT_WRITE},
		{Ident: uint64(fd), Flags: unix.EV_DELETE, Filter: unix.EVFILT_READ},
	}, nil, nil)
	return err
}
