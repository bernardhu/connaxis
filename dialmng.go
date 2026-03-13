package connaxis

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/internal/tls"
	"github.com/bernardhu/connaxis/ringbuffer"
	"github.com/bernardhu/connaxis/wrapper"
)

type DialerMng struct {
	print     bool
	online    int32
	lastProbe int64
	dialers   sync.Map
	status    sync.Map
	inactive  sync.Map
	srv       *Server
	mux       sync.Mutex
}

type DialerStatus struct {
	except int32 //最大数量
	alive  int32 //存活数量
	co     int32 //当前同时请求的协程数
	para   *DialParam
}

type DialParam struct {
	Addr     string
	Cert     string
	Key      string
	Typ      string
	SslType  int
	Duration time.Duration
	On       func(string, string, uint64)
	Close    func(string, string, uint64)
	Probe    func(string) []byte
	Handler  connection.IPktHandler
}

func (m *DialerMng) addDialer(para *DialParam, max int) {
	var status *DialerStatus
	key := para.Key + "_" + para.Addr

	_, ok := m.inactive.Load(key)
	if ok {
		wrapper.Errorf("key: %s addr:%s inactive", key, para.Addr)
		return
	}

	m.mux.Lock()
	v, ok := m.status.Load(key)
	if ok {
		status = v.(*DialerStatus)
	} else {
		status = new(DialerStatus)
		status.para = para
		atomic.StoreInt32(&status.except, int32(max))
		atomic.StoreInt32(&status.alive, 0)
		atomic.StoreInt32(&status.co, 0)
		m.status.Store(key, status)
	}
	m.mux.Unlock()

	co := atomic.AddInt32(&status.co, 1)
	if co <= 0 {
		atomic.StoreInt32(&status.co, 0)
		wrapper.Errorf("key: %s addr:%s reset co", key, para.Addr)
		return
	}

	if atomic.LoadInt32(&status.alive) >= atomic.LoadInt32(&status.except) {
		atomic.AddInt32(&status.co, -1)
		wrapper.Infof("add dail %s skip, alive:%d max:%d", key, status.alive, status.except)
		return
	}

	if co > int32(max) {
		atomic.AddInt32(&status.co, -1)
		wrapper.Infof("add dail %s skip, alive:%d, co:%d max:%d", key, status.alive, co, status.except)
		return
	}

	d := new(Dialer)
	d.score = 180
	d.key = para.Key
	d.typ = para.Typ
	d.subkey = key
	err := d.bootup(para)
	if err != nil {
		atomic.AddInt32(&status.co, -1)
		wrapper.Errorf("add dail %s fail, err:%v", para.Addr, err)
		return
	}

	m.attach(d, para)
}

func (m *DialerMng) updateDialer(op, key, addr string) {
	skey := key + "_" + addr
	if op == "add" {
		m.inactive.Delete(skey)
	} else {
		m.inactive.Store(skey, addr)
	}
}

func (m *DialerMng) lookup(id uint64) *Dialer {
	//wrapper.Infof("lookup id %d", id)
	val, ok := m.dialers.Load(id)
	if ok {
		return val.(*Dialer)
	}
	//wrapper.Infof("lookup id %d not found", id)
	return nil
}

func (m *DialerMng) attach(d *Dialer, para *DialParam) {
	loop := m.srv.SelectLoop(d.fd)

	if para.Cert == "" {
		c := &connection.Conn{}
		c.Client = true
		c.SetFd(d.fd)
		c.SetRemote(d.remote)
		c.SetLocal(d.local)
		c.SetRecvbuf(ringbuffer.NewRingBuffer())
		c.SetCloseCB(m)
		c.SetOpenCB(m)
		c.SetContext(d)
		if para.Handler != nil {
			c.SetPktHandler(para.Handler)
		}
		d.underlying = c
		loop.AllocID(c)
		d.cid = c.ID()
		_ = loop.AddClient(c)

		wrapper.Debugf("add cid %d fd:%d to loop: %d", c.ID(), c.Fd(), loop.Id())
	} else {
		if para.SslType == connection.TYPE_CONN_TLS {
			cfg := &tls.Config{
				InsecureSkipVerify: true,
				MaxVersion:         tls.VersionTLS13,
			}
			//nolint:staticcheck // nil context intentionally triggers connection layer's default TLS handshake timeout.
			c, err := connection.NewTLSConnClient(nil, d.fd, cfg)
			if err != nil {
				if v, ok := m.status.Load(d.subkey); ok {
					status := v.(*DialerStatus)
					atomic.AddInt32(&status.co, -1)
				}
				if d.f != nil {
					_ = d.f.Close()
					d.f = nil
				}
				d.fd = 0
				wrapper.Errorf("dial tls handshake fail, addr:%s err:%v", d.subkey, err)
				return
			}
			c.Client = true
			c.SetRemote(d.remote)
			c.SetLocal(d.local)
			c.SetCloseCB(m)
			c.SetOpenCB(m)
			c.SetContext(d)
			if para.Handler != nil {
				c.SetPktHandler(para.Handler)
			}

			d.underlying = c
			loop.AllocID(c)
			d.cid = c.ID()

			_ = loop.AddClient(c)
			wrapper.Debugf("add cid %d fd:%d to loop: %d", c.ID(), c.Fd(), loop.Id())
		}
	}
	if d.watch != nil {
		d.watch.OnUpdate(true)
	}
}

func (m *DialerMng) KeepAlive() {
	timer := time.NewTicker(time.Second)
	defer timer.Stop()

	for range timer.C {
		m.status.Range(func(key, value interface{}) bool {
			status := value.(*DialerStatus)
			_, ok := m.inactive.Load(key)
			if ok {
				wrapper.Errorf("key: %s addr:%s inactive, del", key, status.para.Addr)
				m.status.Delete(key)
				return true
			}

			cur := atomic.LoadInt32(&status.alive)
			max := atomic.LoadInt32(&status.except)
			for ; cur < max; cur++ {
				wrapper.Infof("key:%v cur: %d max: %d request: %s", key, cur, max, status.para.Addr)
				go m.addDialer(status.para, int(max))
			}
			return true
		})
	}
}

func (m *DialerMng) Polling() {
	for {
		now := time.Now().UnixNano() / 1000000
		m.dialers.Range(func(k, v interface{}) bool {
			d := v.(*Dialer)
			if d.BufLen() > 16 || now-d.LastFlush() >= 5 {
				err := d.Flush()
				if err != nil || d.BufLen() > 5000 {
					_ = d.underlying.AddCmd(2, nil)
					wrapper.Errorf("DialerMng close dailer:%d fd:%d %s:%s:%s err:%v, len:%d", d.cid, d.fd, d.network, d.addr, d.key, err, d.BufLen())
				}
			}

			return true
		})
		time.Sleep(time.Millisecond * 5)
	}
}

func (m *DialerMng) OnClose(itf interface{}) {
	d := itf.(*Dialer)
	_, ok := m.dialers.Load(d.cid)
	if !ok {
		return
	}
	atomic.AddInt32(&m.online, -1)
	d.Close()
	v, ok := m.status.Load(d.subkey)
	alive := int32(0)
	if ok {
		status := v.(*DialerStatus)
		alive = atomic.AddInt32(&status.alive, -1)
	}

	m.dialers.Delete(d.cid)

	wrapper.Infof("DialerMng close dailer:%d fd:%d %s:%s:%s alive %d", d.cid, d.fd, d.network, d.addr, d.key, alive)
}

func (m *DialerMng) OnOpen(itf interface{}) {
	d := itf.(*Dialer)
	v, ok := m.status.Load(d.subkey)
	if ok {
		status := v.(*DialerStatus)

		atomic.AddInt32(&m.online, 1)
		alive := atomic.AddInt32(&status.alive, 1)
		m.dialers.Store(d.cid, d)
		wrapper.Infof("DialerMng add set dailer:%d fd:%d cid:%d %s:%s:%s alive %d", d.cid, d.fd, d.cid, d.network, d.addr, d.key, alive)
		if d.onCb != nil {
			d.onCb(d.key, d.addr, d.cid)
		}
		atomic.AddInt32(&status.co, -1)
	} else {
		_ = d.underlying.AddCmd(2, nil)
		wrapper.Infof("DialerMng del dailer:%d fd:%d cid:%d %s:%s:%s no status found", d.cid, d.fd, d.cid, d.network, d.addr, d.key)
	}

}

func (m *DialerMng) Stat(now int64, print bool) {
	wrapper.Gauge("qps.connaxis.dails", int64(m.online))

	if now > m.lastProbe+5 { //5s probe 一次
		m.dialers.Range(func(k, v interface{}) bool {
			d := v.(*Dialer)
			if d.underlying != nil {
				send := d.underlying.GetSend(true)
				recv := d.underlying.GetRecv(true)
				if recv == 0 && d.probeCb != nil {
					probes := d.probeCb(d.typ)
					if len(probes) > 0 {
						lastRecv := d.underlying.GetLastRecv()
						if lastRecv == 0 {
							d.underlying.SetLastRecv(now)
						} else {
							if d.lastProbe-lastRecv > 10 {
								_ = d.underlying.AddCmd(2, nil)
								wrapper.Errorf("DialerMng close dailer:%d fd:%d %s:%s:%s lastRecv:%d now:%d diff:%d", d.cid, d.fd, d.network, d.addr, d.key, lastRecv, now, now-lastRecv)
								return true
							}
						}

						wrapper.Infof("DialerMng try probe:%d fd:%d %s:%s:%s", d.cid, d.fd, d.network, d.addr, d.key)
						_ = d.underlying.AddCmd(1, probes)
						d.lastProbe = now
					}
				}
				if m.print {
					if send > 0 {
						if recv == 0 {
							d.score = d.score - 1
						} else if send/recv > 10 {
							d.score = d.score - 1
						} else {
							d.score = d.score + 1
						}
					} else {
						if recv >= 0 {
							d.score = d.score + 1
						}
					}

					if d.score > 180 {
						d.score = 180
					}

					if print {
						wrapper.Infof("dial id:%d addr:%s send:%d recv:%d score:%d", d.cid, d.addr, send, recv, d.score)
					}

					if d.score < 0 {
						//d.underlying.Close()
						if print {
							wrapper.Infof("dial id:%d addr:%s send:%d recv:%d score:%d too high, kill", d.cid, d.addr, send, recv, d.score)
						}
					}
				}
			}
			return true
		})

		m.lastProbe = now
	}

}
