//go:build darwin || netbsd || freebsd || openbsd || dragonfly || linux
// +build darwin netbsd freebsd openbsd dragonfly linux

package connaxis

import (
	"errors"
	"net"
	"os"
	"reflect"
	"syscall"
	"unsafe"

	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/eventloop"
	"github.com/bernardhu/connaxis/internal/tls"
)

type listener struct {
	ln      net.Listener
	lnaddr  net.Addr
	ep      eventloop.IEVEndpoint
	f       *os.File
	addr    string
	fd      int
	tlsmode int
	config  *tls.Config
}

func (ln *listener) Close() {
	if ln.f != nil {
		ln.f.Close()
		ln.f = nil
		ln.fd = 0
	} else if ln.fd != 0 {
		_ = syscall.Close(ln.fd)
		ln.fd = 0
	}
	if ln.ln != nil {
		ln.ln.Close()
		ln.ln = nil
	}
}

// system takes the net listener and detaches it from it's parent
// event loop, grabs the file descriptor, and makes it non-blocking.
func (ln *listener) system() error {
	var err error
	switch netln := ln.ln.(type) {
	case *net.TCPListener:
		ln.f, err = netln.File()
		netln.Close()
	default:
		if ln.tlsmode != connection.TYPE_CONN {
			// XXX: This is really BAD!!! Only way currently to get the underlying
			// connection of the tls.Conn. At least until
			// https://github.com/golang/go/issues/29257 is solved.
			netln := reflect.ValueOf(ln.ln).Elem().FieldByName("Listener")
			netln = reflect.NewAt(netln.Type(), unsafe.Pointer(netln.UnsafeAddr())).Elem()
			netconn := netln.Interface().(*net.TCPListener)
			ln.f, err = netconn.File()
			//netconn.Close()
		} else {
			err = errors.New("unsupported listener type: only tcp is supported")
		}
	}
	if err != nil {
		ln.Close()
		return err
	}
	ln.fd = int(ln.f.Fd())
	return syscall.SetNonblock(ln.fd, true)
}

func (ln *listener) Fd() int {
	return ln.fd
}

func (ln *listener) TlsMode() int {
	return ln.tlsmode
}

func (ln *listener) TlsConfig() *tls.Config {
	return ln.config
}

func (ln *listener) Addr() net.Addr {
	return ln.ln.Addr()
}

func (ln *listener) ListenAddr() string {
	return ln.addr
}

func (ln *listener) GetEndpoint() eventloop.IEVEndpoint {
	return ln.ep
}
