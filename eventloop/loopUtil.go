package eventloop

import (
	"github.com/bernardhu/connaxis/pool"
	"github.com/bernardhu/connaxis/tuning"
	"golang.org/x/sys/unix"
)

var cmdpool = pool.NewObjectPool(func() *CmdData {
	return new(CmdData)
})

func buildSimpleID(seq uint32, fd int, lpid int) uint64 {
	return uint64(seq)<<32 | uint64(fd)<<16 | uint64(lpid)
}

func setSocketOptions(fd int) error {
	if tuning.AcceptSocketNoDelay {
		if err := unix.SetsockoptInt(fd, unix.IPPROTO_TCP, unix.TCP_NODELAY, 1); err != nil {
			return err
		}
	}

	if tuning.AcceptSocketSendBufBytes > 0 {
		if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_SNDBUF, tuning.AcceptSocketSendBufBytes); err != nil {
			return err
		}
	}

	return nil
}
