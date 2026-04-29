package eventloop

import (
	"container/list"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/internal"
	"github.com/bernardhu/connaxis/pool"
	"github.com/bernardhu/connaxis/ringbuffer"
	"github.com/bernardhu/connaxis/tuning"
	"github.com/bernardhu/connaxis/wrapper"
	"golang.org/x/sys/unix"
)

type LoopConn struct {
	idx          int            // loop index in the server loops list
	poll         *internal.Poll // epoll or kqueue
	wg           *sync.WaitGroup
	recvbuf      []byte       // shared read packet buffer
	writeFds     map[int]bool // fds currently interested in write events
	closeOnDrain map[int]bool // fds that should close after pending writes are drained
	cmdFlushSet  map[int]bool
	count        int32 // connection count
	dcount       int32 // dial connection count
	seq          uint32
	cmdChan      chan *CmdData
	cmdDialChan  chan *CmdData

	pktSizeLimit int
	cliSbufLimit int
	handler      IHandler
	triggered    int32

	read       int32
	write      int32
	recvcmd    int32
	cmdconsume int32
	cmddrop    int32
	cmdfail    int32

	lastCheck     int64
	checkInterval int64
	idleLimit     int64
	checkList     *list.List
	touchMap      map[uint64]*connChecker

	idg IDGenerator

	nowSec  int64
	nowMsec int64

	acceptChan chan connection.EngineConn

	listeners   []IListener
	sel         ISelector
	tlsWorker   *TLSHandshakeWorker
	max         int32
	acceptCount int32
	tlsAccept   int32

	stopOnce     sync.Once
	shutdownOnce sync.Once
}

type connChecker struct {
	touch int64
	elem  *list.Element
}

func (l *LoopConn) Init(idx, size, chanSize, pktSizeLimit, cliSbufLimit int) {
	l.idx = idx
	l.poll = internal.OpenPoll(idx)
	l.recvbuf = pool.GAlloctor.Get(size)
	l.cmdChan = make(chan *CmdData, chanSize)
	l.cmdDialChan = make(chan *CmdData, chanSize)
	l.acceptChan = make(chan connection.EngineConn, chanSize)
	l.writeFds = make(map[int]bool)
	l.closeOnDrain = make(map[int]bool)
	l.cmdFlushSet = make(map[int]bool)
	l.checkList = list.New()
	l.touchMap = make(map[uint64]*connChecker)
	l.pktSizeLimit = pktSizeLimit
	l.cliSbufLimit = cliSbufLimit

	wrapper.Infof("loop %d buf ask %d size:%d chanSize:%d sizelimit:%d", idx, size, len(l.recvbuf), cap(l.cmdChan), pktSizeLimit)
}

func (l *LoopConn) updateWriteFd(c connection.EngineConn) {
	fd := c.Fd()
	if fd == 0 {
		return
	}
	wantWrite := c.PendingWrite() > 0
	_, hasWrite := l.writeFds[fd]
	if wantWrite == hasWrite {
		return
	}
	if wantWrite {
		_ = l.poll.ModReadWrite(fd)
		l.writeFds[fd] = true
	} else {
		_ = l.poll.ModRead(fd)
		delete(l.writeFds, fd)
	}
}

func (l *LoopConn) drainWrite(c connection.EngineConn) (int, error) {
	if c.PendingWrite() == 0 {
		return 0, nil
	}

	plan := tuning.MaxFlushBytesPerEvent
	total := 0

	for {
		atomic.AddInt32(&l.write, 1)
		n, err := c.FlushN(plan)
		if err != nil {
			return total, err
		}

		if n > 0 {
			total = total + n
		}

		if n <= 0 || (total >= plan && plan > 0) {
			return total, nil
		}
	}
}

func (l *LoopConn) emitOutput(c connection.EngineConn, out []byte) error {
	outLen := len(out)
	if outLen == 0 {
		return nil
	}

	// Keep strict per-connection ordering: when queue is non-empty,
	// new data must be appended behind buffered data.
	if c.PendingWrite() == 0 {
		n, err := c.Write(out)
		if n == outLen {
			return nil
		}
		if err != nil && err != unix.EAGAIN {
			return err
		}
		if n < 0 {
			n = 0
		}
		if n > 0 {
			out = out[n:]
		}
	}

	owner := pool.GAlloctor.Get(len(out))
	if owner == nil {
		return unix.ENOMEM
	}
	copy(owner, out)
	c.EnqueueWrite(owner, len(out))
	return nil
}

func (l *LoopConn) SetPollWait(msec int) {
	l.poll.SetPollWait(msec)
}

func (l *LoopConn) Id() int {
	return l.idx
}

func (l *LoopConn) SyncTime(sec int64, msec int64) {
	atomic.StoreInt64(&l.nowSec, sec)
	atomic.StoreInt64(&l.nowMsec, msec)
}

func (l *LoopConn) SetCheck(interval, limit int64) {
	l.checkInterval = interval
	l.idleLimit = limit
}

func (l *LoopConn) Stat(now int64, print bool) {
	accept := atomic.LoadInt32(&l.acceptCount)
	tlsAccept := atomic.LoadInt32(&l.tlsAccept)
	tlsPending := int32(0)
	tlsMaxWorkers := 0
	tlsWorkers := 0
	tlsIdleWorkers := 0
	if l.tlsWorker != nil {
		tlsPending, tlsMaxWorkers, tlsWorkers, tlsIdleWorkers = l.tlsWorker.Stats()
	}
	write := atomic.LoadInt32(&l.write)
	read := atomic.LoadInt32(&l.read)
	recvcmd := atomic.LoadInt32(&l.recvcmd)
	cmdconsume := atomic.LoadInt32(&l.cmdconsume)
	cmddrop := atomic.LoadInt32(&l.cmddrop)
	cmdfail := atomic.LoadInt32(&l.cmdfail)
	count := atomic.LoadInt32(&l.count)
	len := len(l.cmdChan)
	if print && (accept > 0 || tlsAccept > 0 || tlsPending > 0 || write > 0 || read > 0 || recvcmd > 0 || cmdconsume > 0 || l.cmdfail > 0 || l.cmddrop > 0) {
		wrapper.Infof("index: %d accept:%d tlsAccept:%d tlsPending:%d write: %d read: %d online: %d recvcmd: %d cmdconsume: %d cmdfail: %d cmddrop: %d cmdchanlen:%d", l.idx, accept, tlsAccept, tlsPending, write, read, count, recvcmd, cmdconsume, cmdfail, cmddrop, len)
	}

	wrapper.Count("qps.connaxis.accept.ntls", int64(accept))
	wrapper.Count("qps.connaxis.accept.tls", int64(tlsAccept))
	wrapper.Gauge("cnt.connaxis.accept.tlspending", int64(tlsPending))
	wrapper.Gauge("cnt.connaxis.tls.handshake.pending", int64(tlsPending))
	wrapper.Gauge("cnt.connaxis.tls.handshake.max_workers", int64(tlsMaxWorkers))
	wrapper.Gauge("cnt.connaxis.tls.handshake.workers", int64(tlsWorkers))
	wrapper.Gauge("cnt.connaxis.tls.handshake.idle_workers", int64(tlsIdleWorkers))
	wrapper.Count("qps.connaxis.loop.write", int64(write))
	wrapper.Count("qps.connaxis.loop.read", int64(read))
	wrapper.Count("qps.connaxis.loop.recvcmd", int64(recvcmd))
	wrapper.Count("qps.connaxis.loop.cmdconsume", int64(cmdconsume))
	wrapper.Count("qps.connaxis.loop.cmddrop", int64(cmddrop))
	wrapper.Count("qps.connaxis.loop.cmdfail", int64(cmdfail))
	atomic.AddInt32(&l.acceptCount, -accept)
	atomic.AddInt32(&l.tlsAccept, -tlsAccept)
	atomic.AddInt32(&l.write, -write)
	atomic.AddInt32(&l.read, -read)
	atomic.AddInt32(&l.recvcmd, -recvcmd)
	atomic.AddInt32(&l.cmdconsume, -cmdconsume)
	atomic.AddInt32(&l.cmddrop, -cmddrop)
	atomic.AddInt32(&l.cmdfail, -cmdfail)
}

func (l *LoopConn) Stop() {
	l.stopOnce.Do(func() {
		if l.poll != nil {
			_ = l.poll.Close()
		}
	})
}

func (l *LoopConn) shutdown() {
	l.shutdownOnce.Do(func() {
		for _, entry := range l.listeners {
			if l.poll != nil {
				_ = l.poll.Del(entry.Fd())
			}
			entry.Close()
		}

		fdTableRange(l.idx, func(fd int, c connection.EngineConn) {
			l.closeConn(c, nil)
		})
	})
}

func (l *LoopConn) Online() int32 {
	return atomic.LoadInt32(&l.count)
}

func (l *LoopConn) DialCnt() int32 {
	return atomic.LoadInt32(&l.dcount)
}

func (l *LoopConn) SetWg(wg *sync.WaitGroup) {
	l.wg = wg
}

func (l *LoopConn) SetIDGen(gen IDGenerator) {
	l.idg = gen
}

func (l *LoopConn) SetTLSWorker(worker *TLSHandshakeWorker) {
	l.tlsWorker = worker
}

func (l *LoopConn) GenID(seed int) uint64 {
	seq := atomic.AddUint32(&l.seq, 1)
	return buildSimpleID(seq, seed, l.idx)
}

func (l *LoopConn) AddListener(lis IListener) {
	fd := lis.Fd()
	if fd < 0 {
		return
	}

	for i := range l.listeners {
		if l.listeners[i].Fd() == fd {
			lis.Close()
			return
		}
	}

	l.listeners = append(l.listeners, lis)
	_ = l.poll.AddRead(fd)
}

func (l *LoopConn) AllocID(c connection.EngineConn) {
	if l.idg != nil {
		c.SetID(l.idg.GetID())
	} else {
		c.SetID(l.GenID(c.Fd()))
	}
}

func (l *LoopConn) AddClient(c connection.EngineConn) error {
	c.SetReceiver(l)

	select {
	case l.acceptChan <- c:
		//wrapper.Debugf("loop %d AddClient id:%d fd:%d acceptChan has:%d", l.idx, c.ID(), c.Fd(), len(l.acceptChan))
		break
	default:
		wrapper.Errorf("loop %d AddClient fail, full id:%d fd:%d acceptChan has:%d", l.idx, c.ID(), c.Fd(), len(l.acceptChan))
		return errors.New("chan full")
	}

	if atomic.CompareAndSwapInt32(&l.triggered, 0, 1) { // swapped
		//wrapper.Debugf("loop %d trigger poll", l.idx)
		_ = l.poll.Trigger(internal.CmdUser)
	}
	return nil
}

func (l *LoopConn) SetHandler(h IHandler) {
	l.handler = h
}

func (l *LoopConn) SetSelector(s ISelector) {
	l.sel = s
}

func (l *LoopConn) Run() {
	defer func() {
		wrapper.Infof("index: %d stop", l.idx)
		l.wg.Done()
	}()

	wrapper.Infof("loop %d start", l.idx)
	l.poll.Poll(func(fd int, events internal.Event) {
		if fd == 0 {
			l.processCmd(events)
		} else {
			if c := l.connByFD(fd); c != nil {
				if events&internal.EventErr != 0 {
					l.closeConn(c, CLOSE_EPOLL_ERR)
					return
				}

				// Prioritize writes so overloaded connections can drain pending
				// output before ingesting more input.
				if events&internal.EventWrite != 0 || c.PendingWrite() > 0 {
					l.processSend(c)
					if c.Fd() == 0 {
						return
					}
				}

				if events&internal.EventRead != 0 {
					l.processRead(c)
					if c.Fd() == 0 {
						return
					}
					// Keep one soft write chance after each read batch.
					if c.PendingWrite() > 0 {
						l.processSend(c)
					}
				}
				return
			}

			if events&internal.EventRead != 0 {
				l.processAccept(fd)
			}
		}
	})
}

func (l *LoopConn) postAccept(c connection.EngineConn) error {
	c.Open()
	l.handler.OnConnected(c)
	//wrapper.Debugf("loop: %d add client id:%d fd:%d ", l.idx, c.ID(), c.Fd())
	if err := l.poll.AddRead(c.Fd()); err != nil {
		return err
	}

	// The TLS handshake worker may leave application plaintext buffered inside
	// userspace crypto/tls. For example, a client can pipeline the first request
	// right behind the handshake flight. By the time the connection is handed
	// back to the loop, the kernel receive queue may already be empty, so no
	// read event will fire even though tls.Conn has plaintext ready.
	// Drain once to pull that buffered plaintext into the normal packet path.
	if c.GetType() == connection.TYPE_CONN_TLS {
		if tc, ok := c.(interface{ IsKTLS() bool }); ok && tc.IsKTLS() {
			// kTLS connections already expose any post-handshake plaintext via preRead.
			// Do not force an initial read unless there is buffered app data to drain.
			if pc, ok := c.(interface{ BufferedPlaintextLen() int }); ok && pc.BufferedPlaintextLen() > 0 {
				l.processRead(c)
				if c.Fd() == 0 {
					return fmt.Errorf("ktls conn closed during initial buffered drain")
				}
			}
			return nil
		}
		l.processRead(c)
		if c.Fd() == 0 {
			return fmt.Errorf("tls conn closed during initial drain")
		}
	}
	return nil
}

func (l *LoopConn) attachClient(c connection.EngineConn) bool {
	if prev := l.connByFD(c.Fd()); prev != nil {
		l.closeConn(prev, CLOSE_BY_DUP_ACCEPT)
	}

	if !l.storeConn(c) {
		wrapper.Errorf("loop %d add client fail, fd out of range id:%d fd:%d", l.idx, c.ID(), c.Fd())
		_ = c.Close()
		return false
	}

	now := atomic.AddInt32(&l.count, 1)
	if c.IsClient() {
		atomic.AddInt32(&l.dcount, 1)
	}

	err := l.postAccept(c)
	if err != nil {
		wrapper.Infof("index: %d add client fail, err: %v", l.idx, err)
		l.closeConn(c, CLOSE_BY_ACCEPT)
		return false
	}

	if l.checkInterval > 0 && l.idleLimit > 0 && !c.IsClient() { // not dial client
		l.touchMap[c.ID()] = &connChecker{
			touch: atomic.LoadInt64(&l.nowSec),
			elem:  l.checkList.PushBack(c),
		}
	}

	wrapper.Debugf("loop: %d processCmd add client id:%d fd:%d has: %d", l.idx, c.ID(), c.Fd(), now)
	return true
}

func (l *LoopConn) closeConn(c connection.EngineConn, err error) {
	if c.Fd() == 0 { //already closed
		return
	}

	fd := c.Fd()
	delete(l.closeOnDrain, fd)

	if l.checkInterval > 0 && l.idleLimit > 0 {
		checker := l.touchMap[c.ID()]
		if checker != nil && checker.elem != nil {
			l.checkList.Remove(checker.elem)
		}
		delete(l.touchMap, c.ID())
	}

	//now := atomic.AddInt32(&l.count, -1)
	atomic.AddInt32(&l.count, -1)
	if c.IsClient() {
		atomic.AddInt32(&l.dcount, -1)
	}
	_ = l.poll.Del(fd)
	delete(l.writeFds, fd)
	delete(l.cmdFlushSet, fd)
	l.clearConn(c)
	wrapper.Debugf("loop: %d del client id:%d fd:%d err: %v", l.idx, c.ID(), fd, err)
	c.Close()
	l.handler.OnClosed(c, err)
}

func (l *LoopConn) processSend(c connection.EngineConn) {
	//wrapper.Infof("loop: %d processSend fd:%d pending: %d", l.idx, c.Fd(), c.PendingWrite())
	_, err := l.drainWrite(c)
	if err != nil && err != unix.EAGAIN {
		//wrapper.Debugf("loop: %d fd:%d send fail, err:%v", l.idx, c.Fd(), err)
		l.closeConn(c, CLOSE_SEND_ERR)
		return
	}
	if c.PendingWrite() == 0 && l.closeOnDrain[c.Fd()] {
		l.closeConn(c, CLOSE_BY_USER_LAND)
		return
	}
	l.updateWriteFd(c)
}

// 1 recvbuf 应该只会残存一个数据包的部分数据
// 2 recvbuf 的大小是和数据包大小接近的
// 3 processRead应尽量一次读取更多的数据
// 4 是使用recvbuf 还是 公用recvbuf 取决于谁的剩余空间多，和拷贝的权衡
// 5 todo 应该直接读到eagain
// 6 recvbuf单线程使用
func (l *LoopConn) processRead(c connection.EngineConn) {
	readBytes := 0
	for {
		left := c.Recvbuf().Has()

		recvbuf := l.recvbuf
		if left > 0 {
			if len(l.recvbuf)-left > c.Recvbuf().Capacity()-left { // 公用剩余多
				privateBuf := c.Recvbuf().AlignBytes()
				copy(recvbuf, privateBuf[:left])
			} else { // 使用私有
				if c.Recvbuf().Free() == 0 {
					resizeBuf := pool.GAlloctor.Get(c.Recvbuf().Capacity() * 2)
					if resizeBuf == nil {
						wrapper.Errorf("poll: %d fd:%d too big pkt", l.idx, c.Fd())
						l.closeConn(c, CLOSE_PKT_SIZE_LIMIT)
						return
					}
					bytes := c.Recvbuf().AlignBytes()
					copy(resizeBuf, bytes)
					c.Recvbuf().Update(resizeBuf)
					c.Recvbuf().Forward(len(bytes), ringbuffer.OpWrite)
				}
				recvbuf = c.Recvbuf().AlignBytes()
			}
		}

		eagain := false
		shortRead := false
		budgetExhausted := false

		n, err := c.Read(recvbuf[left:]) //尽可能一次读完
		atomic.AddInt32(&l.read, 1)

		if err != nil {
			if err == unix.EAGAIN {
				eagain = true
			} else {
				l.closeConn(c, CLOSE_READ_ERR)
				return
			}
		}
		if n <= 0 && !eagain {
			l.closeConn(c, CLOSE_READ_ERR)
			return
		}
		readBytes += n
		if tuning.MaxReadBytesPerEvent > 0 && readBytes >= tuning.MaxReadBytesPerEvent {
			budgetExhausted = true
		}
		// On LT pollers, a short read on plain TCP usually means the current socket
		// receive queue has been drained for this round. Stop early and rely on the
		// next read event instead of forcing another syscall to hit EAGAIN.
		if c.GetType() == connection.TYPE_CONN && !eagain && n > 0 && n < len(recvbuf[left:]) {
			shortRead = true
		}

		if l.checkInterval > 0 && l.idleLimit > 0 && !c.IsClient() {
			if checker := l.touchMap[c.ID()]; checker != nil && checker.elem != nil {
				checker.touch = atomic.LoadInt64(&l.nowSec)
				l.checkList.MoveToBack(checker.elem)
			}
		}
		c.SetLastRecv(atomic.LoadInt64(&l.nowSec))
		in := recvbuf[:left+n]

		wantClose := false
		for {
			length, expect := c.ParsePacket(&in) // 完整包才返回正数
			if length < 0 {                      // parse error
				wrapper.Errorf("poll: %d fd:%d invalid pkt length:%d in:%d expect:%d handler:%T", l.idx, c.Fd(), length, len(in), expect, c.GetPktHandler())
				l.closeConn(c, CLOSE_PKT_PARSE_ERR)
				return
			}
			if expect > l.pktSizeLimit {
				wrapper.Errorf("poll: %d fd:%d too big pkt expect:%d limit:%d", l.idx, c.Fd(), expect, l.pktSizeLimit)
				l.closeConn(c, CLOSE_PKT_SIZE_LIMIT)
				return
			}

			if length > 0 {
				if length > len(in) {
					wrapper.Errorf("poll: %d fd:%d invalid pkt length:%d in:%d expect:%d handler:%T", l.idx, c.Fd(), length, len(in), expect, c.GetPktHandler())
					l.closeConn(c, CLOSE_PKT_PARSE_ERR)
					return
				}

				data := in[:length]
				out, action := c.OnData(&data)
				if err := l.emitOutput(c, out); err != nil {
					if err == unix.ENOMEM {
						wrapper.Errorf("poll: %d fd:%d too much to send, want: %d", l.idx, c.Fd(), len(out))
						l.closeConn(c, CLOSE_MEM_ALLOC_FAIL)
					} else {
						l.closeConn(c, CLOSE_SEND_ERR)
					}
					return
				}
				wantClose = action

				in = in[length:]

				if wantClose || len(in) == 0 {
					if len(in) == 0 { // 全部读完，无剩余
						c.Recvbuf().Reset()
					}
					break
				}
				continue
			}

			//partial pkt
			left := len(in)
			if left > 0 { // 还剩部分包
				if expect > c.Recvbuf().Capacity() { // 不够
					resizeBuf := pool.GAlloctor.Get(expect)
					if resizeBuf == nil {
						wrapper.Errorf("poll: %d fd:%d too big pkt", l.idx, c.Fd())
						l.closeConn(c, CLOSE_PKT_SIZE_LIMIT)
						return
					}
					c.Recvbuf().Update(resizeBuf)
					c.Recvbuf().Write(&in, left, true)
				} else { // 够
					c.Recvbuf().Truncate()
					c.Recvbuf().Write(&in, left, true)
				}
			}
			break
		}

		if _, err := l.drainWrite(c); err != nil && err != unix.EAGAIN {
			l.closeConn(c, CLOSE_SEND_ERR)
			return
		}

		if wantClose {
			if c.PendingWrite() > 0 {
				l.closeOnDrain[c.Fd()] = true
				l.updateWriteFd(c)
				return
			}
			l.closeConn(c, CLOSE_BY_USER_LAND)
			return
		}

		if c.GetType() == connection.TYPE_CONN || c.GetType() == connection.TYPE_CONN_TLS {
			pending := c.PendingWrite()
			if c.IsClient() {
				if pending >= ringbuffer.RingBufferGuardByteSize {
					wrapper.Errorf("id:%d fd:%d pending:%d too much pending, drop", c.ID(), c.Fd(), pending)
					l.closeConn(c, CLOSE_SEND_ERR)
					return
				}
			} else if pending > l.cliSbufLimit {
				wrapper.Infof("loop %d processRead id:%d fd:%d pending:%d limit:%d, maybe recv too slow, drop", l.idx, c.ID(), c.Fd(), pending, l.cliSbufLimit)
				l.closeConn(c, CLOSE_BY_SLOW_READER)
				return
			}
		}

		if eagain || shortRead || budgetExhausted {
			l.updateWriteFd(c)
			return
		}
	}
}

func (l *LoopConn) AddCmd(cmd, fd int, dial bool, id uint64, data []byte) error {
	cmddata := cmdpool.Get()
	cmddata.cmd = cmd
	cmddata.fd = fd
	cmddata.id = id

	if len(data) > 0 {
		owner := pool.GAlloctor.Get(len(data))
		if owner == nil {
			cmddata.reset()
			cmdpool.Put(cmddata)
			return errors.New("alloc fail")
		}
		copy(owner, data)
		cmddata.data = owner
		cmddata.size = len(data)
	}

	ch := l.cmdChan
	if dial {
		ch = l.cmdDialChan
	}
	select {
	case ch <- cmddata:
		atomic.AddInt32(&l.recvcmd, 1)
	default:
		atomic.AddInt32(&l.cmdfail, 1)
		cmddata.reset()
		cmdpool.Put(cmddata)
		return errors.New("chan full")
	}

	if atomic.CompareAndSwapInt32(&l.triggered, 0, 1) { // swapped
		_ = l.poll.Trigger(internal.CmdUser)
	}

	return nil
}

func (l *LoopConn) onIdle() {
	now := atomic.LoadInt64(&l.nowSec)
	if l.lastCheck == 0 {
		l.lastCheck = now
		return
	}

	if (l.checkInterval > 0) && (now-l.lastCheck >= l.checkInterval) {
		//wrapper.Debugf("loop: %d check idle now:%d last:%d int:%d", l.idx, now, l.lastCheck, l.checkInterval)
		l.lastCheck = now
		if l.idleLimit > 0 {
			for e := l.checkList.Front(); e != nil; {
				c := e.Value.(connection.EngineConn)
				checker := l.touchMap[c.ID()]

				//wrapper.Infof("loop: %d check cid:%d ts:%d limit:%d diff:%d int:%d", l.idx, c.ID(), checker.touch, l.idleLimit, now-checker.touch, l.checkInterval)
				if checker != nil && now-checker.touch > l.idleLimit {
					//wrapper.Errorf("loop: %d timeout del client id:%d fd:%d now:%d touch:%d idle:%d limit:%d left:%d", l.idx, c.ID(), c.Fd(), now, checker.touch, now-checker.touch, l.idleLimit, l.checkList.Len())
					next := e.Next()
					l.checkList.Remove(e)
					e = next
					delete(l.touchMap, c.ID())
					l.closeConn(c, CLOSE_BY_ANTI_IDLE)
				} else {
					break
				}
			}
		}
	}
}

func (l *LoopConn) processCmd(ev internal.Event) {
	switch ev {
	case internal.EventStop:
		l.shutdown()
		return
	case internal.EventIdle:
		l.onIdle()
	}

	accepted := 0
	for {
		if tuning.MaxAcceptPerEvent > 0 && accepted >= tuning.MaxAcceptPerEvent {
			break
		}
		select {
		case c := <-l.acceptChan:
			l.attachClient(c)
			accepted++
		default:
			goto doneAccept
		}
	}
doneAccept:

	consumed := 0
	for {
		if tuning.MaxCmdPerEvent > 0 && consumed >= tuning.MaxCmdPerEvent {
			break
		}
		select {
		case cmd := <-l.cmdChan:
			l.consumeCmd(cmd)
			consumed++
		default:
			goto doneCmd
		}
	}
doneCmd:

	consumed = 0
	for {
		if tuning.MaxCmdPerEvent > 0 && consumed >= tuning.MaxCmdPerEvent {
			break
		}
		select {
		case cmd := <-l.cmdDialChan:
			l.consumeCmd(cmd)
			consumed++
		default:
			goto doneCmdDial
		}
	}
doneCmdDial:

	flushedConns := 0
	for fd := range l.cmdFlushSet {
		if tuning.MaxCmdFlushConnsPerEvent > 0 && flushedConns >= tuning.MaxCmdFlushConnsPerEvent {
			break
		}
		delete(l.cmdFlushSet, fd)

		c := l.connByFD(fd)
		if c == nil || c.Fd() == 0 {
			continue
		}

		_, err := l.drainWrite(c)
		if err != nil && err != unix.EAGAIN {
			l.closeConn(c, CLOSE_SEND_ERR)
			continue
		}

		if c.GetType() == connection.TYPE_CONN || c.GetType() == connection.TYPE_CONN_TLS {
			pending := c.PendingWrite()
			if c.IsClient() {
				if pending >= ringbuffer.RingBufferGuardByteSize {
					atomic.AddInt32(&l.cmddrop, 1)
					wrapper.Errorf("id:%d fd:%d pending:%d too much pending, drop", c.ID(), c.Fd(), pending)
					l.closeConn(c, CLOSE_SEND_ERR)
					continue
				}
			} else {
				if pending > l.cliSbufLimit {
					wrapper.Infof("loop %d processCmd id:%d fd:%d pending:%d limit:%d, maybe recv too slow, drop", l.idx, c.ID(), c.Fd(), pending, l.cliSbufLimit)
					l.closeConn(c, CLOSE_BY_SLOW_READER)
					continue
				}
			}
		}

		l.updateWriteFd(c)
		flushedConns++
	}

	atomic.StoreInt32(&l.triggered, 0)
	if len(l.cmdChan) > 0 || len(l.cmdDialChan) > 0 || len(l.acceptChan) > 0 || len(l.cmdFlushSet) > 0 {
		if atomic.CompareAndSwapInt32(&l.triggered, 0, 1) {
			_ = l.poll.Trigger(internal.CmdUser)
		}
	}
	//wrapper.Debug("loop %d processCmd end", l.idx)
}

func (l *LoopConn) consumeCmd(cmd *CmdData) {
	op := cmd.cmd
	defer func() {
		cmd.reset()
		cmdpool.Put(cmd)
	}()

	atomic.AddInt32(&l.cmdconsume, 1)

	c := l.connByFD(cmd.fd)
	if c == nil {
		return
	}

	if op == connection.CMD_DATA {
		if c.ID() != cmd.id || cmd.size == 0 || c.Fd() == 0 {
			return
		}

		c.EnqueueWrite(cmd.data, cmd.size)
		cmd.data = nil //防止被释放
		l.cmdFlushSet[c.Fd()] = true

		if c.Fd() == 0 {
			return
		}
		if c.GetType() == connection.TYPE_CONN || c.GetType() == connection.TYPE_CONN_TLS {
			pending := c.PendingWrite()
			if c.IsClient() {
				if pending >= ringbuffer.RingBufferGuardByteSize {
					atomic.AddInt32(&l.cmddrop, 1)
					wrapper.Errorf("id:%d fd:%d pending:%d too much pending, drop", c.ID(), c.Fd(), pending)
					l.closeConn(c, CLOSE_SEND_ERR)
					return
				}
			} else {
				if pending > l.cliSbufLimit {
					wrapper.Infof("loop %d processCmd id:%d fd:%d pending:%d limit:%d, maybe recv too slow, drop", l.idx, c.ID(), c.Fd(), pending, l.cliSbufLimit)
					l.closeConn(c, CLOSE_BY_SLOW_READER)
					return
				}
			}
		}
		return
	}

	if op == connection.CMD_CLOSE {
		wrapper.Errorf("loop %d close fd:%d by cmd", l.idx, cmd.fd)
		l.closeConn(c, CLOSE_BY_CLOSE_CMD)
		return
	}

	// Unknown cmd.
	wrapper.Debugf("loop %d processCmd error fd:%d id:%d unknown cmd", l.idx, cmd.id, cmd.fd)
}
