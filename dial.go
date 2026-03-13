package connaxis

import (
	"errors"
	"net"
	"os"
	"reflect"
	"sync"
	"syscall"
	"unsafe"

	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/internal/tls"
	"github.com/bernardhu/connaxis/wrapper"
	"golang.org/x/sys/unix"
)

type IDialWatch interface {
	OnUpdate(bool)
}

type Dialer struct {
	cid uint64

	key     string //大类
	subkey  string
	typ     string
	f       *os.File
	network string
	addr    string

	fd         int
	bufs       [][]byte
	lastFlush  int64
	lastProbe  int64
	bufLen     int
	lock       sync.Mutex
	local      net.Addr
	remote     net.Addr
	watch      IDialWatch
	underlying connection.EngineConn

	closeCb func(key, addr string, id uint64)
	onCb    func(key, addr string, id uint64)
	probeCb func(string) []byte
	score   int
}

func (d *Dialer) bootup(p *DialParam) error {
	conn, err := d.dial(p)
	if err != nil {
		wrapper.Debugf("Dialer %s add err:%v", p.Addr, err)
		return err
	}

	d.local = conn.LocalAddr()
	d.remote = conn.RemoteAddr()
	if err := d.system(conn); err != nil {
		wrapper.Debugf("Dialer %s system err:%v", p.Addr, err)
		return err
	}

	if err := unix.SetsockoptInt(d.fd, unix.SOL_SOCKET, unix.SO_SNDBUF, 1024*1024); err != nil {
		_ = unix.Close(d.fd)
		wrapper.Errorf("bootup set fd:%d SO_SNDBUF 1M err :%v", d.fd, err)
		return err
	}

	return nil
}

func (d *Dialer) dial(p *DialParam) (net.Conn, error) {
	var err error
	d.network, d.addr, err = parseAddr(p.Addr)
	if err != nil {
		return nil, err
	}
	d.onCb = p.On
	d.closeCb = p.Close
	d.probeCb = p.Probe

	var conn net.Conn

	if d.network != "tcp" {
		return conn, errors.New("unsupported network: only tcp is supported")
	}
	if p.SslType == connection.TYPE_CONN_TLS || p.SslType == connection.TYPE_CONN {
		wrapper.Debugf("dial conn")
		_, err := net.ResolveTCPAddr("tcp", d.addr)
		if err != nil {
			return conn, err
		}

		conn, err = net.DialTimeout("tcp", d.addr, p.Duration) //net.DialTCP("tcp", nil, ra)
		if err != nil {
			return conn, err
		}
	} else {
		return conn, errors.New("not supported")
	}

	return conn, nil
}

// system takes the net listener and detaches it from it's parent
// event loop, grabs the file descriptor, and makes it non-blocking.
func (d *Dialer) system(con net.Conn) error {
	var err error
	switch conn := con.(type) {
	case *net.TCPConn:
		d.f, err = conn.File()
		conn.Close()
	case *tls.Conn:
		netln := reflect.ValueOf(conn).Elem().FieldByName("conn")
		netln = reflect.NewAt(netln.Type(), unsafe.Pointer(netln.UnsafeAddr())).Elem()
		netconn := netln.Interface().(*net.TCPConn)
		d.f, err = netconn.File()
		netconn.Close()
	default:
		return errors.New("unsupport connection type")
	}

	if err != nil {
		if d.fd != 0 {
			d.f.Close()
			d.fd = 0
		}
		return err
	}
	d.fd = int(d.f.Fd())
	return syscall.SetNonblock(d.fd, true)
}

func (d *Dialer) Underlying() connection.EngineConn {
	return d.underlying
}

func (d *Dialer) SetWatcher(watch IDialWatch) {
	d.watch = watch
}

func (d *Dialer) GetID() uint64 {
	return d.cid
}

func (d *Dialer) Addr() string {
	return d.addr
}

func (d *Dialer) AddBuf(buf []byte) int {
	d.lock.Lock()
	d.bufs = append(d.bufs, buf)
	d.bufLen = len(d.bufs)
	d.lock.Unlock()

	return d.bufLen
}

func (d *Dialer) Flush() error {
	if d.bufLen > 1 {
		return d.flushVec()
	} else if d.bufLen == 1 {
		return d.flushOne()
	}
	return nil
}

func (d *Dialer) BufLen() int {
	return d.bufLen
}

func (d *Dialer) LastFlush() int64 {
	return d.lastFlush
}

func (d *Dialer) Close() {
	if d.watch != nil {
		d.watch.OnUpdate(false)
	}

	if d.closeCb != nil {
		d.closeCb(d.key, d.addr, d.cid)
	}

	d.f.Close()
	d.f = nil

	wrapper.Debugf("close dailer:%d fd:%d %s:%s:%s", d.cid, d.fd, d.network, d.addr, d.key)
}
