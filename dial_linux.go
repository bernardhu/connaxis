package connaxis

import (
	"time"

	"golang.org/x/sys/unix"
)

func (d *Dialer) flushOne() error {
	d.lock.Lock()
	defer d.lock.Unlock()

	sum := len(d.bufs[0])

	n, err := unix.Write(d.fd, d.bufs[0])
	if err != nil {
		if err == unix.EAGAIN {
			return nil
		} else {
			return err
		}
	}

	if sum == n {
		d.bufs = [][]byte{}
	} else {
		d.bufs[0] = d.bufs[0][n:]
	}

	d.bufLen = len(d.bufs)
	d.lastFlush = time.Now().UnixNano() / 1000000
	return nil
}

func (d *Dialer) flushVec() error {
	d.lock.Lock()
	defer d.lock.Unlock()

	sum := 0
	cnt := 0
	for _, b := range d.bufs {
		sum += len(b)
		cnt += 1
		if cnt == 512 {
			break
		}
	}

	n, err := unix.Writev(d.fd, d.bufs[:cnt])
	if err != nil {
		if err == unix.EAGAIN {
			return nil
		} else {
			return err
		}
	}

	if n < sum {
		var pos int
		for i := range d.bufs {
			np := len(d.bufs[i])
			if n < np {
				d.bufs[i] = d.bufs[i][n:]
				pos = i
				break
			}
			n -= np
		}

		d.bufs = d.bufs[pos:]
		d.bufLen = len(d.bufs)
		d.lastFlush = time.Now().UnixNano() / 1000000
	} else {
		d.bufs = [][]byte{}
	}
	return nil
}
