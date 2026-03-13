package connection

import (
	"sync/atomic"

	"golang.org/x/sys/unix"
)

func flushWriteQueue(fd int, wq *writeQueue, maxBytes int, send *int32) (int, error) {
	if fd == 0 {
		return 0, unix.EBADF
	}
	if wq.pending() == 0 {
		return 0, nil
	}

	var iovecs [maxWritevIovecs]unix.Iovec
	iovcnt := 0

	remaining := maxBytes

	for i := wq.head; i < len(wq.items) && iovcnt < len(iovecs); i++ {
		item := &wq.items[i]
		if item.off >= item.size {
			continue
		}

		b := item.owner[item.off:item.size]
		if maxBytes > 0 {
			if remaining <= 0 {
				break
			}
			if len(b) > remaining {
				b = b[:remaining]
			}
			remaining -= len(b)
		}

		iovecs[iovcnt].Base = &b[0]
		iovecs[iovcnt].SetLen(len(b))
		iovcnt++
	}

	if iovcnt == 0 {
		return 0, nil
	}

	n, err := writev(fd, iovecs[:iovcnt])
	if err != nil && err == unix.EAGAIN && n < 0 {
		n = 0
	}
	if n <= 0 {
		return n, err
	}

	atomic.AddInt32(send, int32(n))
	wq.consume(n)
	return n, err
}
