//go:build darwin || netbsd || freebsd || openbsd || dragonfly || linux
// +build darwin netbsd freebsd openbsd dragonfly linux

package connaxis

import (
	"errors"
	"math/rand"
	"net"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/eventloop"
	"github.com/bernardhu/connaxis/internal/tls"
	"github.com/bernardhu/connaxis/tuning"
	"github.com/bernardhu/connaxis/wrapper"
)

type Server struct {
	handler eventloop.IHandler
	cfg     *EvConfig

	dispatch  eventloop.IEVLoop   // master loop
	loops     []eventloop.IEVLoop // all the loops
	tlsWorker *eventloop.TLSHandshakeWorker

	wg   sync.WaitGroup // loop close waitgroup
	cond *sync.Cond     // shutdown signaler
	done chan struct{}  // closed when clean() is done

	shutdownSignaled bool

	balance   LoadBalance // load balancing method
	workerNum int         // worker numbers

	dmng *DialerMng

	online          int32
	dials           int32
	lastCheckOnline int64

	nowSec int64
	nowMs  int64

	seq uint32 // connection count
}

// waitForShutdown waits for a signal to shutdown
func (s *Server) waitForShutdown() {
	s.cond.L.Lock()
	for !s.shutdownSignaled {
		s.cond.Wait()
	}
	s.cond.L.Unlock()
}

// signalShutdown signals a shutdown an begins server closing
func (s *Server) signalShutdown() {
	s.cond.L.Lock()
	s.shutdownSignaled = true
	s.cond.Broadcast()
	s.cond.L.Unlock()
}

func (s *Server) setLBStrategy(strategy string) {
	switch strategy {
	case "lru":
		s.balance = LeastConnections
	case "rand":
		s.balance = Random
	case "hash":
		s.balance = Hash
	default:
		s.balance = RoundRobin
	}
}

func (s *Server) setWorker(num int) {
	s.workerNum = num
	if num <= 0 {
		if num == 0 {
			s.workerNum = 1
		} else {
			s.workerNum = runtime.NumCPU()
		}
	}
}

func (s *Server) GetListenAddrs() []net.Addr {
	addrs := make([]net.Addr, 0, len(s.cfg.ListenAddrs))
	for _, ep := range s.cfg.ListenAddrs {
		addrs = append(addrs, ep)
	}
	return addrs
}

func (s *Server) GetWorkerNum() int {
	return s.workerNum
}

func (s *Server) GetLoad() int32 {
	now := atomic.LoadInt64(&s.nowSec)
	old := atomic.SwapInt64(&s.lastCheckOnline, now)
	if old < now {
		total := int32(0)
		dials := int32(0)
		for _, v := range s.loops {
			total = total + v.Online()
			dials = dials + v.DialCnt()
		}
		atomic.StoreInt32(&s.online, total)
		atomic.StoreInt32(&s.dials, dials)
		if total > 0 {
			wrapper.Debugf("calc online:%d dials:%d", total, dials)
		}
		return total
	}
	return atomic.LoadInt32(&s.online)
}

func (s *Server) GetDials() int32 {
	return atomic.LoadInt32(&s.dials)
}

func (s *Server) start() error {
	cfg := s.cfg
	if cfg == nil {
		return errors.New("nil config")
	}
	var tlsConfig *tls.Config
	// create loops locally.
	eventloop.InitFDTable()
	for i := 0; i < s.workerNum; i++ {
		l := &eventloop.LoopConn{}
		l.Init(i, cfg.BufSize, cfg.ChanSize, cfg.PktSizeLimit, cfg.CliSendBufLimit)
		l.SetPollWait(cfg.PollWait)
		l.SetWg(&s.wg)
		l.SetHandler(s.handler)
		l.SetSelector(s)
		l.SetMax(int32(cfg.MaxAcceptFD))
		l.SetCheck(int64(cfg.IdleCheckInt), int64(cfg.IdleLimit))
		l.SetIDGen(cfg.IDGen)

		s.loops = append(s.loops, l)
	}

	if strings.ToLower(strings.TrimSpace(cfg.SslMode)) == "tls" {
		var err error
		tlsConfig, err = buildTLSConfig(cfg)
		if err != nil {
			return err
		}
		s.tlsWorker = eventloop.NewTLSHandshakeWorker(tuning.TlsHandshakeWorkers, s.loops)
		s.tlsWorker.Start()
		for _, loop := range s.loops {
			l, _ := loop.(*eventloop.LoopConn)
			l.SetTLSWorker(s.tlsWorker)
		}
	}

	// bind listeners to every loop (SO_REUSEPORT sockets per loop).
	for _, lp := range s.loops {
		for _, ep := range cfg.ListenAddrs {
			ln, err := openListener(ep, cfg, tlsConfig)
			if err != nil {
				if s.tlsWorker != nil {
					s.tlsWorker.Stop()
				}
				return err
			}
			wrapper.Debugf("input:%v net:%s, addr:%s, reuse:%t", ep, ep.Network(), ep.String(), true)
			lp.AddListener(ln)
		}
	}

	// run loops after all listeners are bound.
	for _, lp := range s.loops {
		s.wg.Add(1)
		go lp.Run()
	}
	return nil
}

func (s *Server) stat(print bool) {
	ticker := time.NewTicker(time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-s.done:
				return
			case t := <-ticker.C:
				nowSec := t.Unix()
				nowMs := t.UnixNano() / 1000000
				atomic.StoreInt64(&s.nowSec, nowSec)
				atomic.StoreInt64(&s.nowMs, nowMs)

				//syncTime
				if s.dispatch != nil {
					s.dispatch.SyncTime(nowSec, nowMs)
				}
				for _, v := range s.loops {
					v.SyncTime(nowSec, nowMs)
				}

				if print {
					wrapper.Info("----start to print stat info----")
				}
				if s.dispatch != nil {
					s.dispatch.Stat(nowSec, print)
				}

				if s.dmng != nil {
					s.dmng.Stat(nowSec, print)
				}
				if s.handler != nil {
					s.handler.Stat(print)
				}
				total := int32(0)
				for _, v := range s.loops {
					v.Stat(nowSec, print)
					total = total + v.Online()
				}
				atomic.StoreInt32(&s.online, total)
				wrapper.Gauge("qps.connaxis.online", int64(total))
			}
		}
	}()
}

func (s *Server) Stop() {
	wrapper.Info("stop server.....")
	s.signalShutdown()
	if s.done != nil {
		<-s.done
	}
}

func serve(h eventloop.IHandler, config *EvConfig, standalone bool) (error, *Server) {
	if config == nil {
		return errors.New("nil config"), nil
	}
	s := &Server{}
	s.cfg = config
	s.handler = h
	s.cond = sync.NewCond(&sync.Mutex{})
	s.done = make(chan struct{})
	s.setLBStrategy(config.LbStrategy)
	s.setWorker(config.Ncpu)
	s.dmng = &DialerMng{
		srv: s,
	}

	wrapper.Debug("startting..")
	if strings.ToLower(strings.TrimSpace(config.SslMode)) == "tls" {
		engine := connection.ResolveTLSEngine(config.TlsEngine)
		if engine == connection.TLSEngineKTLS {
			applyKTLSPolicy(config)
		}
		connection.SetTLSEngine(engine)
	}
	if err := s.start(); err != nil {
		return err, nil
	}

	if standalone {
		defer s.clean()
	} else {
		go s.clean()
	}

	if h != nil {
		h.OnReady(s)
	}

	s.stat(config.PrintStat)

	if s.cfg.DialKeepAlive {
		go s.dmng.KeepAlive()
	}

	if s.cfg.DialPolling {
		go s.dmng.Polling()
	}

	//time.Sleep(time.Second *65)
	//s.stop()
	return nil, s
}

func (s *Server) clean() {
	defer close(s.done)

	s.waitForShutdown()

	// notify all loops to shutdown; loop owns listener lifecycle.
	if s.dispatch != nil {
		s.dispatch.Stop()
	}

	if s.tlsWorker != nil {
		s.tlsWorker.Stop()
	}

	for _, l := range s.loops {
		l.Stop()
	}

	// wait on all loops to complete reading events
	s.wg.Wait()
	wrapper.Flush()
}

func (s *Server) SelectLoop(id int) eventloop.IEVLoop {
	switch s.balance {
	case Random:
		return s.loops[rand.Intn(len(s.loops))]
	case Hash:
		idx := id % len(s.loops)
		return s.loops[idx]
	case LeastConnections:
		online := int32(10000000)
		sel := 0
		for k, l := range s.loops {
			cur := l.Online()
			if online > cur {
				online = cur
				sel = k
			}
		}

		return s.loops[sel]
	default:
		cur := atomic.LoadUint32(&s.seq)
		idx := cur % uint32(len(s.loops))
		atomic.AddUint32(&s.seq, 1)
		return s.loops[idx]
	}
}

func (s *Server) AddDialer(para *DialParam, max int) {
	wrapper.Debugf("AddDialer addr:%s", para.Addr)
	s.dmng.addDialer(para, max)
}

func (s *Server) DialerUpdate(op, key, addr string) {
	wrapper.Debugf("DialerUpdate op:%s key:%s addr:%s", op, key, addr)
	s.dmng.updateDialer(op, key, addr)
}

func (s *Server) Lookup(id uint64) *Dialer {
	return s.dmng.lookup(id)
}

func (s *Server) Close(d *Dialer) {
	s.dmng.OnClose(d)
}
