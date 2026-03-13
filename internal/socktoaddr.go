package internal

import (
	"net"

	"golang.org/x/sys/unix"
)

// SockaddrToAddr returns a go/net friendly address
func SockaddrToAddr(sa unix.Sockaddr) net.Addr {
	switch sa := sa.(type) {
	case *unix.SockaddrInet4:
		return &net.TCPAddr{
			IP:   append([]byte{}, sa.Addr[:]...),
			Port: sa.Port,
		}
	case *unix.SockaddrInet6:
		var zone string
		if sa.ZoneId != 0 {
			if ifi, err := net.InterfaceByIndex(int(sa.ZoneId)); err == nil {
				zone = ifi.Name
			}
		}
		return &net.TCPAddr{
			IP:   append([]byte{}, sa.Addr[:]...),
			Port: sa.Port,
			Zone: zone,
		}
	case *unix.SockaddrUnix:
		return &net.UnixAddr{Net: "unix", Name: sa.Name}
	}
	return nil
}
