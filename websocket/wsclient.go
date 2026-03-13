package websocket

import (
	"github.com/bernardhu/connaxis/pool"
)

type WsClient struct {
	Upgraded bool
	Cid      uint64

	AllowPlainHTTP bool
	PlainHTTP      bool

	nonce   []byte
	firstop byte

	perMessageDeflate  bool
	fragmentCompressed bool

	buf      *clientBuf
	inflater pmdInflater
	UserData interface{}
}

func (c *WsClient) resetBuf() {
	if c.buf != nil {
		c.buf.reset()
	}
}

func (c *WsClient) resetMessageState() {
	c.firstop = 0
	c.fragmentCompressed = false
	c.resetBuf()
}

type clientBuf struct {
	buf         []byte
	writeOffset int
	checkOffset int
}

func (c *clientBuf) write(in []byte) bool {
	left := len(c.buf) - c.writeOffset
	if left >= len(in) {
		copy(c.buf[c.writeOffset:], in)
		c.writeOffset = c.writeOffset + len(in)
	} else {
		buf := pool.GAlloctor.Get(c.writeOffset + len(in))
		if buf == nil {
			return false
		}
		//wrapper.Debugf("clientBuf resize to %d", len(buf))
		if c.writeOffset > 0 {
			copy(buf, c.buf[:c.writeOffset])
		}
		copy(buf[c.writeOffset:], in)
		pool.GAlloctor.Put(c.buf)
		c.buf = buf
		c.writeOffset = c.writeOffset + len(in)
	}

	return true
}

func (c *clientBuf) bytes() []byte {
	if c.writeOffset == 0 {
		return []byte{}
	}
	return c.buf[:c.writeOffset]
}

func (c *clientBuf) size() int {
	return c.writeOffset
}

func (c *clientBuf) reset() {
	c.checkOffset = 0
	c.writeOffset = 0
	pool.GAlloctor.Put(c.buf)
	c.buf = nil
}

func (c *WsClient) closePMD() {
	c.inflater.close()
}
