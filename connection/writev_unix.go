package connection

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

func writev(fd int, iovecs []unix.Iovec) (int, error) {
	if len(iovecs) == 0 {
		return 0, nil
	}
	n, _, errno := unix.Syscall(unix.SYS_WRITEV, uintptr(fd), uintptr(unsafe.Pointer(&iovecs[0])), uintptr(len(iovecs)))
	if errno != 0 {
		return int(n), errno
	}
	return int(n), nil
}
