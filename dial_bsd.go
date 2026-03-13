//go:build darwin || netbsd || freebsd || openbsd || dragonfly
// +build darwin netbsd freebsd openbsd dragonfly

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

	pos := 0
	lastsend := 0
	for ; pos < cnt; pos++ {
		n, err := unix.Write(d.fd, d.bufs[pos])
		lastsend = n
		if err != nil {
			if err == unix.EAGAIN {
				break
			} else {
				return err
			}
		} else {
			if lastsend < len(d.bufs[pos]) {
				break
			}
		}

	}

	if lastsend < len(d.bufs[pos]) {
		d.bufs[pos] = d.bufs[pos][lastsend:]
	}
	d.bufs = d.bufs[pos:]
	if len(d.bufs) == 0 {
		d.bufs = [][]byte{}
	}

	d.bufLen = len(d.bufs)
	d.lastFlush = time.Now().UnixNano() / 1000000

	return nil
}
