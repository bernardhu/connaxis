package connection

import (
	"net"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

const maxWritevIovecs = 64

// Conn ...
type Conn struct {
	Base
	closecb CloseCb
	opencb  OpenCb
}

func (c *Conn) RemoteAddr() net.Addr {
	return c.remote
}

func (c *Conn) LocalAddr() net.Addr {
	return c.local
}

func (c *Conn) SetRemote(addr net.Addr) {
	c.remote = addr
	c.remoteAddr = ""
}

func (c *Conn) SetCloseCB(cb CloseCb) {
	c.closecb = cb
}

func (c *Conn) SetOpenCB(cb OpenCb) {
	c.opencb = cb
}

func (c *Conn) SetLocal(addr net.Addr) {
	c.local = addr
	c.localAddr = ""
}

func (c *Conn) GetLocalAddr() string {
	if c.local == nil {
		return ""
	} else {
		if c.localAddr == "" {
			c.localAddr = c.local.String()
		}
		return c.localAddr
	}
}

func (c *Conn) GetRemoteAddr() string {
	if c.remote == nil {
		return ""
	} else {
		if c.remoteAddr == "" {
			c.remoteAddr = c.remote.String()
		}
		return c.remoteAddr
	}
}

func (c *Conn) Read(b []byte) (int, error) {
	r, e := unix.Read(c.fd, b)
	if e == unix.EAGAIN {
		//wrapper.Debugf("eagin set r from %d to 0", r)
		r = 0
	}
	if r > 0 {
		atomic.AddInt32(&c.recv, int32(r))
	}
	//wrapper.Debugf("conn bufcap:%d read:%d e:%v", len(b), r, e)
	return r, e
}

func (c *Conn) Write(b []byte) (int, error) {
	if c.fd == 0 {
		return 0, unix.EBADF
	}

	n, e := unix.Write(c.fd, b)
	if n > 0 {
		atomic.AddInt32(&c.send, int32(n))
	}
	return n, e
}

func (c *Conn) Close() error {
	c.clearWrite()
	err := unix.Close(c.fd)
	if c.closecb != nil {
		c.closecb.OnClose(c.ctx)
	}
	c.fd = 0
	return err
}

func (c *Conn) Open() {
	if c.opencb != nil {
		c.opencb.OnOpen(c.ctx)
	}
}

func (c *Conn) SetDeadline(t time.Time) error {
	return nil
}

func (c *Conn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *Conn) SetWriteDeadline(t time.Time) error {
	return nil
}

func (c *Conn) GetType() int {
	return TYPE_CONN
}

func (c *Conn) FlushN(maxBytes int) (int, error) {
	return flushWriteQueue(c.fd, &c.wq, maxBytes, &c.send)
}

func (c *Conn) ParsePacket(in *[]byte) (length, expect int) {
	return c.h.ParsePacket(c, in)
}

func (c *Conn) OnData(in *[]byte) (out []byte, close bool) {
	return c.h.OnData(c, in)
}

func (c *Conn) Destroy() {}
