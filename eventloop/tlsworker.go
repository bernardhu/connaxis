package eventloop

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/wrapper"
)

const (
	defaultTLSHandshakePoolSize      = 128
	defaultTLSHandshakePoolMinActive = 8
)

type TLSHandshakeWorker struct {
	loops      []IEVLoop
	maxWorkers int
	minWorkers int

	mu          sync.RWMutex
	workers     int
	idleWorkers int

	pending  int32
	stopping bool
	stopOnce sync.Once
	wg       sync.WaitGroup
}

func NewTLSHandshakeWorker(maxWorkers int, loops []IEVLoop) *TLSHandshakeWorker {
	if maxWorkers <= 0 || maxWorkers > defaultTLSHandshakePoolSize {
		maxWorkers = defaultTLSHandshakePoolSize
	}
	minWorkers := defaultTLSHandshakePoolMinActive
	if minWorkers > maxWorkers {
		minWorkers = maxWorkers
	}

	return &TLSHandshakeWorker{
		loops:      loops,
		maxWorkers: maxWorkers,
		minWorkers: minWorkers,
	}
}

func (w *TLSHandshakeWorker) Start() {
	w.mu.Lock()
	w.workers = w.minWorkers
	w.idleWorkers = w.minWorkers
	w.mu.Unlock()
}

func (w *TLSHandshakeWorker) Stop() {
	w.stopOnce.Do(func() {
		w.mu.Lock()
		w.stopping = true
		w.mu.Unlock()
		w.wg.Wait()
	})
}

func (w *TLSHandshakeWorker) Submit(c *connection.ATLSConn) error {
	if c == nil {
		return errors.New("nil tls conn")
	}
	w.mu.Lock()
	if w.stopping {
		w.mu.Unlock()
		return errors.New("tls handshake worker stopped")
	}
	if w.idleWorkers > 0 {
		w.idleWorkers--
	} else if w.workers < w.maxWorkers {
		w.workers++
	} else {
		w.mu.Unlock()
		return errors.New("tls handshake pool full")
	}
	atomic.AddInt32(&w.pending, 1)
	w.wg.Add(1)
	w.mu.Unlock()

	go func() {
		w.handleConn(c)
		w.recycleWorker()
		w.wg.Done()
	}()
	return nil
}

func (w *TLSHandshakeWorker) Pending() int32 {
	return atomic.LoadInt32(&w.pending)
}

func (w *TLSHandshakeWorker) Stats() (pending int32, maxWorkers, workers, idleWorkers int) {
	pending = atomic.LoadInt32(&w.pending)
	w.mu.RLock()
	maxWorkers = w.maxWorkers
	workers = w.workers
	idleWorkers = w.idleWorkers
	w.mu.RUnlock()
	return pending, maxWorkers, workers, idleWorkers
}

func (w *TLSHandshakeWorker) recycleWorker() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopping {
		return
	}
	if w.idleWorkers < w.workers {
		w.idleWorkers++
	}
}

func (w *TLSHandshakeWorker) handleConn(c *connection.ATLSConn) {
	begin := time.Now()
	if err := c.CompleteServerHandshake(); err != nil {
		atomic.AddInt32(&w.pending, -1)
		cost := time.Since(begin) / time.Millisecond
		if connection.TlsHandshakeTimeout > 0 && time.Since(c.HandshakeAt()) >= connection.TlsHandshakeTimeout {
			wrapper.Errorf("tls handshake timeout fd:%d cost:%dms timeout:%s", c.Fd(), cost, connection.TlsHandshakeTimeout)
			wrapper.Increment("connaxis.tls.handshake.error.timeout")
		} else {
			wrapper.Errorf("tls handshake err:%v fd:%d cost:%dms", err, c.Fd(), cost)
			wrapper.Increment("connaxis.tls.handshake.error." + tlsHandshakeErrorReason(err))
		}
		_ = c.Close()
		return
	}

	loop := w.loops[c.AcceptLoopID()]
	atomic.AddInt32(&w.pending, -1)
	wrapper.Timing("connaxis.tls.handshake.latency", time.Since(begin))
	wrapper.Increment("connaxis.tls.handshake.success")
	if err := loop.AddClient(c); err != nil {
		_ = c.Close()
	}
}

func tlsHandshakeErrorReason(err error) string {
	if err == nil {
		return "unknown"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "connection reset by peer"):
		return "reset"
	case strings.Contains(msg, "unknown certificate"):
		return "unknown_certificate"
	case strings.Contains(msg, "unsupported versions"):
		return "unsupported_version"
	case strings.Contains(msg, "bad certificate"):
		return "bad_certificate"
	case strings.Contains(msg, "first record does not look like a tls handshake"):
		return "not_tls"
	case strings.Contains(msg, "closed") || strings.Contains(msg, "use of closed network connection"):
		return "closed"
	default:
		return "other"
	}
}
