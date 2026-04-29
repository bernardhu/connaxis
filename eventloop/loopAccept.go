package eventloop

import (
	"sync/atomic"

	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/internal"
	"github.com/bernardhu/connaxis/ringbuffer"
	"github.com/bernardhu/connaxis/tuning"
	"github.com/bernardhu/connaxis/wrapper"
	"golang.org/x/sys/unix"
)

func (l *LoopConn) connByFD(fd int) connection.EngineConn {
	c := fdTableGet(fd)
	if c == nil || c.LoopID() != l.idx {
		return nil
	}
	return c
}

func (l *LoopConn) storeConn(c connection.EngineConn) bool {
	c.SetLoopID(l.idx)
	return fdTableSet(c.Fd(), c)
}

func (l *LoopConn) clearConn(c connection.EngineConn) {
	fdTableClear(c.Fd(), c)
}

func (l *LoopConn) SetMax(max int32) {
	l.max = max
}

func (l *LoopConn) processAccept(fd int) {
	var lis IListener

	for i := range l.listeners {
		if l.listeners[i].Fd() == fd {
			lis = l.listeners[i]
			break
		}
	}

	if lis == nil {
		return
	}
	if lis.TlsMode() == connection.TYPE_CONN {
		l.accept(lis, fd)
	} else {
		l.acceptTLS(lis, fd)
	}
}

func (l *LoopConn) accept(lis IListener, fd int) {
	for i := 0; i < tuning.MaxAcceptPerEvent; i++ {
		nfd, sa, err := unix.Accept(fd)
		if err != nil {
			if err != unix.EAGAIN {
				wrapper.Errorf("loop %d accept err:%v fd:%d max:%d", l.idx, err, nfd, l.max)
			}
			return
		}

		if l.max > 0 && l.sel != nil && l.sel.GetLoad() > l.max {
			_ = unix.Close(nfd)
			wrapper.Errorf("loop %d overload close fd:%d", l.idx, nfd)
			return
		}

		if err := unix.SetNonblock(nfd, true); err != nil {
			_ = unix.Close(nfd)
			wrapper.Errorf("loop %d accept set fd:%d nonblock err:%v", l.idx, nfd, err)
			continue
		}

		if err := setSocketOptions(nfd); err != nil {
			_ = unix.Close(nfd)
			wrapper.Errorf("loop %d accept set fd:%d socket options err:%v", l.idx, nfd, err)
			continue
		}

		atomic.AddInt32(&l.acceptCount, 1)

		c := &connection.Conn{}
		c.SetFd(nfd)
		c.SetRemote(internal.SockaddrToAddr(sa))
		c.SetLocal(lis.Addr())
		c.SetRecvbuf(ringbuffer.NewRingBuffer())
		c.SetAcceptAddr(lis.ListenAddr())
		c.SetListenerEndpoint(lis.GetEndpoint())

		l.AllocID(c)
		c.SetReceiver(l)
		_ = l.attachClient(c)
	}
}

func (l *LoopConn) acceptTLS(lis IListener, fd int) {
	if l.tlsWorker == nil {
		wrapper.Errorf("loop %d tls worker not initialized", l.idx)
		return
	}

	for i := 0; i < tuning.MaxAcceptPerEvent; i++ {
		nfd, sa, err := unix.Accept(fd)
		if err != nil {
			if err != unix.EAGAIN {
				load := int32(0)
				if l.sel != nil {
					load = l.sel.GetLoad()
				}
				wrapper.Errorf("loop %d accept tls err:%v fd:%d max:%d load:%d", l.idx, err, nfd, l.max, load)
			}
			return
		}

		tlsPending := l.tlsWorker.Pending()
		if l.max > 0 && l.sel != nil && (l.sel.GetLoad()+tlsPending > l.max) {
			_ = unix.Close(nfd)
			wrapper.Errorf("loop %d overload close fd:%d", l.idx, nfd)
			return
		}

		if err := unix.SetNonblock(nfd, true); err != nil {
			_ = unix.Close(nfd)
			wrapper.Errorf("loop %d accept tls set fd:%d nonblock err:%v", l.idx, nfd, err)
			continue
		}

		if err := setSocketOptions(nfd); err != nil {
			_ = unix.Close(nfd)
			wrapper.Errorf("loop %d accept tls set fd:%d socket options err:%v", l.idx, nfd, err)
			continue
		}

		atomic.AddInt32(&l.acceptCount, 1)
		atomic.AddInt32(&l.tlsAccept, 1)

		tlsPending = l.tlsWorker.Pending()
		if tuning.TlsHandshakeMaxPending > 0 && tlsPending >= tuning.TlsHandshakeMaxPending {
			_ = unix.Close(nfd)
			wrapper.Errorf("loop %d tls pending overload close fd:%d pending:%d maxPending:%d", l.idx, nfd, tlsPending, tuning.TlsHandshakeMaxPending)
			wrapper.Increment("connaxis.tls.handshake.drop.max_pending")
			continue
		}

		remote := internal.SockaddrToAddr(sa)
		local := lis.Addr()
		c, err := connection.NewPendingTLSServerConn(nfd, lis.TlsConfig(), local, remote)
		if err != nil {
			_ = unix.Close(nfd)
			wrapper.Errorf("loop %d create pending tls conn err:%v fd:%d", l.idx, err, nfd)
			wrapper.Increment("connaxis.tls.handshake.drop.create_pending")
			continue
		}

		c.SetAcceptAddr(lis.ListenAddr())
		c.SetListenerEndpoint(lis.GetEndpoint())
		c.SetAcceptLoopID(l.idx)
		l.AllocID(c)
		if err := l.tlsWorker.Submit(c); err != nil {
			_ = c.Close()
			wrapper.Errorf("loop %d tls handshake pool full close fd:%d pending:%d", l.idx, nfd, l.tlsWorker.Pending())
			wrapper.Increment("connaxis.tls.handshake.drop.queue_full")
		}
	}
}
