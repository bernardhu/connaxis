//go:build linux

package internal

import (
	"errors"
	"net"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

var ErrKTLSUnsupported = errors.New("ktls unsupported")

// SystemSupportKTLS reports whether the current system likely supports Linux kTLS.
// It checks for the tls kernel module and tries enabling the TCP ULP "tls" on a connected socket.
func SystemSupportKTLS() (bool, error) {
	if _, err := os.Stat("/sys/module/tls"); err != nil {
		return false, err
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return false, err
	}
	defer ln.Close()

	acceptErr := make(chan error, 1)
	accepted := make(chan struct{}, 1)
	go func() {
		conn, err2 := ln.Accept()
		if err2 != nil {
			acceptErr <- err2
			return
		}
		accepted <- struct{}{}
		_ = conn.Close()
		acceptErr <- nil
	}()

	conn, err := net.DialTimeout("tcp", ln.Addr().String(), 2*time.Second)
	if err != nil {
		return false, err
	}
	defer conn.Close()

	select {
	case <-accepted:
	case err := <-acceptErr:
		if err != nil {
			return false, err
		}
		return false, errors.New("ktls probe accept closed unexpectedly")
	case <-time.After(2 * time.Second):
		return false, errors.New("ktls probe accept timeout")
	}

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return false, errors.New("not TCPConn")
	}

	raw, err := tcpConn.SyscallConn()
	if err != nil {
		return false, err
	}

	var serr error
	if err := raw.Control(func(fd uintptr) {
		serr = unix.SetsockoptString(int(fd), unix.IPPROTO_TCP, unix.TCP_ULP, "tls")
	}); err != nil {
		return false, err
	}
	if serr != nil {
		switch serr {
		case unix.EOPNOTSUPP, unix.ENOPROTOOPT, unix.EINVAL:
			return false, ErrKTLSUnsupported
		default:
			return false, serr
		}
	}

	if err := <-acceptErr; err != nil {
		return false, err
	}

	return true, nil
}
