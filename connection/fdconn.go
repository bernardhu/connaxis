package connection

import (
	"net"
	"time"

	"golang.org/x/sys/unix"
)

type fdTimeoutError struct{}

func (*fdTimeoutError) Error() string   { return "i/o timeout" }
func (*fdTimeoutError) Timeout() bool   { return true }
func (*fdTimeoutError) Temporary() bool { return true }

type fdConn struct {
	fd     int
	local  net.Addr
	remote net.Addr
}

func newFDConn(fd int, local, remote net.Addr) *fdConn {
	return &fdConn{fd: fd, local: local, remote: remote}
}

func (c *fdConn) Read(p []byte) (int, error) {
	n, err := unix.Read(c.fd, p)
	if n < 0 {
		n = 0
	}
	if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
		return n, &fdTimeoutError{}
	}
	return n, err
}

func (c *fdConn) Write(p []byte) (int, error) {
	n, err := unix.Write(c.fd, p)
	if n < 0 {
		n = 0
	}
	if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
		return n, &fdTimeoutError{}
	}
	return n, err
}

func (c *fdConn) Close() error {
	err := unix.Shutdown(c.fd, unix.SHUT_RDWR)
	if err == unix.ENOTCONN || err == unix.EINVAL {
		return nil
	}
	return err
}

func (c *fdConn) LocalAddr() net.Addr {
	return c.local
}

func (c *fdConn) RemoteAddr() net.Addr {
	return c.remote
}

func (c *fdConn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

func (c *fdConn) SetReadDeadline(t time.Time) error {
	return setSocketDeadline(c.fd, unix.SO_RCVTIMEO, t)
}

func (c *fdConn) SetWriteDeadline(t time.Time) error {
	return setSocketDeadline(c.fd, unix.SO_SNDTIMEO, t)
}

func setSocketDeadline(fd int, opt int, t time.Time) error {
	tv := unix.Timeval{}
	if !t.IsZero() {
		d := time.Until(t)
		if d <= 0 {
			d = time.Microsecond
		}
		tv = unix.NsecToTimeval(d.Nanoseconds())
	}
	return unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, opt, &tv)
}
