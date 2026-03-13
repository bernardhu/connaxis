package ringbuffer

import (
	"fmt"

	"github.com/bernardhu/connaxis/internal"
	"github.com/bernardhu/connaxis/pool"
	"github.com/bernardhu/connaxis/wrapper"
)

const (
	OpRead = iota
	OpWrite
)

var emptyBuf = []byte{}

// RingBufferInitByteSize ...
var RingBufferInitByteSize = 16 * 1024

// RingBufferLimitByteSize ...
var RingBufferLimitByteSize = 1024 * 1024

var RingBufferGuardByteSize = 8 * 1024 * 1024 //8MB

// RingBuffer is a circular buffer
type RingBuffer struct {
	buf  []byte
	size int
	r    int // next position to read
	w    int // next position to write
	mu   internal.Fakelock
}

// NewRingBuffer returns a new RingBuffer whose buffer has the given size.
func NewRingBuffer() *RingBuffer {
	return &RingBuffer{
		buf:  emptyBuf,
		size: 0,
		r:    0,
		w:    0,
	}
}

// Read ...
func (r *RingBuffer) Read(p *[]byte, length int, update bool) int {
	if len(*p) == 0 || length > len(*p) {
		return -1
	}

	//wrapper.Infof("rb input:%d, w:%d, r:%d, update %v\n", length, r.w, r.r, update)
	r.mu.Lock()
	if r.w == r.r {
		r.mu.Unlock()
		return 0
	}

	n := r.w - r.r
	if n > length {
		n = length
	}

	start := r.r % r.size
	if start+n < r.size {
		copy(*p, r.buf[start:start+n])
	} else {
		copyed := r.size - start
		copy(*p, r.buf[start:r.size])
		copy((*p)[copyed:], r.buf[0:n-copyed])
	}

	if update {
		r.r = r.r + n
	}
	r.mu.Unlock()
	return n
}

// Write ...
func (r *RingBuffer) Write(p *[]byte, length int, update bool) int {
	if r.size == 0 {
		r.buf = pool.GAlloctor.Get(length)
		r.size = len(r.buf)
	}
	if len(*p) == 0 || length > len(*p) {
		return -1
	}

	r.mu.Lock()
	avail := r.size - (r.w - r.r)
	if avail < length {
		if r.resize(length-avail+r.size) < 0 {
			//recv too slow
			return -2
		}
	}

	start := r.w % r.size
	if start+length < r.size {
		copy(r.buf[start:], *p)
	} else {
		copyed := r.size - start
		copy(r.buf[start:], (*p)[:copyed])
		copy(r.buf[0:], (*p)[copyed:])
	}

	if update {
		r.w = r.w + length
		//wrapper.Debugf("rb write %d, cap:%d has:%d", length, r.size, r.w-r.r)
	}
	r.mu.Unlock()
	return length
}

// Forward ...
func (r *RingBuffer) Forward(step, mode int) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	if mode == OpRead {
		if r.r+step > r.w {
			return -1
		}

		r.r = r.r + step

		if r.r == r.w {
			r.r = 0
			r.w = 0
		}
	} else {
		if r.w+step-r.r > r.size {
			return -1
		}

		r.w = r.w + step
	}

	//wrapper.Debugf("after forward r.r:%d r.w:%d", r.r, r.w)
	return 0
}

// Capacity ...
func (r *RingBuffer) Capacity() int {
	return r.size
}

// Has ...
func (r *RingBuffer) Has() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.w - r.r
}

// Free ...
func (r *RingBuffer) Free() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.size - (r.w - r.r)
}

// Reset the read pointer and writer pointer to zero.
func (r *RingBuffer) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.r = 0
	r.w = 0
	pool.GAlloctor.Put(r.buf)
	r.buf = emptyBuf
	r.size = 0
}

// Peek2 returns the readable bytes as up to two slices.
// The second slice is non-nil only when the data wraps around the end.
func (r *RingBuffer) Peek2() (head, tail []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.size == 0 || r.w == r.r {
		return nil, nil
	}

	n := r.w - r.r
	start := r.r % r.size
	if start+n <= r.size {
		return r.buf[start : start+n], nil
	}
	head = r.buf[start:r.size]
	tail = r.buf[0 : start+n-r.size]
	return head, tail
}

// PeekWrite returns a contiguous slice of writable bytes.
// The returned slice may be smaller than length if it would wrap around the end.
// After writing n bytes into the returned slice, call Forward(n, OpWrite).
func (r *RingBuffer) PeekWrite(length int) []byte {
	if length <= 0 {
		return nil
	}
	if r.size == 0 {
		r.buf = pool.GAlloctor.Get(length)
		if r.buf == nil {
			return nil
		}
		r.size = len(r.buf)
		r.r = 0
		r.w = 0
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	avail := r.size - (r.w - r.r)
	if avail < length {
		if r.resize(length-avail+r.size) < 0 {
			return nil
		}
	}

	start := r.w % r.size
	if start+length <= r.size {
		return r.buf[start : start+length]
	}
	return r.buf[start:r.size]
}

// Bytes ...
func (r *RingBuffer) AlignBytes() []byte {
	if r.r%r.size == 0 {
		//wrapper.Debugf("r.r already align to head, direct return")
		return r.buf
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	buf := pool.GAlloctor.Get(r.size)
	n := r.w - r.r
	if n > 0 {
		start := r.r % r.size
		if start+n < r.size {
			copy(buf, r.buf[start:start+n])
		} else {
			copyed := r.size - start
			copy(buf, r.buf[start:r.size])
			copy(buf[copyed:], r.buf[0:n-copyed])
		}
		r.w = n
	} else {
		r.w = 0
	}

	pool.GAlloctor.Put(r.buf)
	r.buf = buf
	r.r = 0
	return r.buf
}

// Bytes ...
func (r *RingBuffer) Update(buf []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	wrapper.Debugf("rb update org:%d now:%d", len(r.buf), len(buf))
	pool.GAlloctor.Put(r.buf)
	r.buf = buf
	r.size = len(buf)
	r.w = 0
	r.r = 0
}

// Truncate ...
func (r *RingBuffer) Truncate() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.r = 0
	r.w = 0
}

func (r *RingBuffer) resize(resize int) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	buf := pool.GAlloctor.Get(resize)
	if buf == nil {
		wrapper.Errorf("rb resize fail, resize:%d", resize)
		return -1
	}
	//wrapper.Infof("rb resize buf from %d->%d", r.size, len(buf))
	n := r.w - r.r
	if n > 0 {
		start := r.r % r.size
		if start+n < r.size {
			copy(buf, r.buf[start:start+n])
		} else {
			copyed := r.size - start
			copy(buf, r.buf[start:r.size])
			copy(buf[copyed:], r.buf[0:n-copyed])
		}
		r.w = n
	} else {
		r.w = 0
	}

	pool.GAlloctor.Put(r.buf)
	r.buf = buf
	r.size = len(buf)
	r.r = 0

	return 0
}

func (r *RingBuffer) Status() string {
	return fmt.Sprintf("size:%d r:%d w:%d", r.size, r.r, r.w)
}
