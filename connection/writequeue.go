package connection

import (
	"github.com/bernardhu/connaxis/pool"
	"github.com/bernardhu/connaxis/tuning"
)

type writeEntry struct {
	owner []byte
	size  int
	off   int
}

type writeQueue struct {
	items []writeEntry
	head  int
	bytes int
}

func (q *writeQueue) pending() int {
	return q.bytes
}

func (q *writeQueue) enqueue(owner []byte, size int) {
	if size <= 0 {
		pool.GAlloctor.Put(owner)
		return
	}

	if tuning.WriteQueueCoalesceMaxBytes > 0 && size <= tuning.WriteQueueCoalesceMaxBytes && len(q.items) > 0 {
		tailIndex := len(q.items) - 1
		tail := &q.items[tailIndex]
		avail := len(tail.owner) - tail.size + tail.off

		if avail >= size && tail.off > 0 && len(tail.owner)-tail.size < size { //空间够，但是尾部不够
			if tail.size-tail.off <= tuning.WriteQueueCompactMaxBytes {
				copy(tail.owner, tail.owner[tail.off:tail.size])
				tail.size, tail.off = tail.size-tail.off, 0
			}
		}

		if len(tail.owner)-tail.size >= size {
			copy(tail.owner[tail.size:], owner[:size])
			tail.size += size
			q.bytes += size
			pool.GAlloctor.Put(owner)
			return
		}
	}

	q.items = append(q.items, writeEntry{owner: owner, size: size})
	q.bytes += size
}

func (q *writeQueue) peek() []byte {
	if q.bytes == 0 {
		return nil
	}
	item := &q.items[q.head]
	if item.off >= item.size {
		return nil
	}
	return item.owner[item.off:item.size]
}

func (q *writeQueue) consume(n int) {
	if n <= 0 || q.bytes == 0 {
		return
	}
	if n > q.bytes {
		n = q.bytes
	}

	for n > 0 && q.bytes > 0 {
		item := &q.items[q.head]
		remain := item.size - item.off
		if n < remain {
			item.off += n
			q.bytes -= n
			return
		}

		n -= remain
		q.bytes -= remain
		pool.GAlloctor.Put(item.owner)
		item.owner = nil
		item.size = 0
		item.off = 0
		q.head++
		if q.head >= len(q.items) {
			q.resetEmpty()
			return
		}
	}

	if q.head > 0 && q.head*2 >= len(q.items) {
		copy(q.items, q.items[q.head:])
		q.items = q.items[:len(q.items)-q.head]
		q.head = 0
	}
}

func (q *writeQueue) clear() {
	for i := q.head; i < len(q.items); i++ {
		item := &q.items[i]
		pool.GAlloctor.Put(item.owner)
		item.owner = nil
		item.size = 0
		item.off = 0
	}
	q.resetEmpty()
}

func (q *writeQueue) resetEmpty() {
	if tuning.WriteQueueShrinkCap > 0 && cap(q.items) > tuning.WriteQueueShrinkCap {
		q.items = nil
	} else {
		q.items = q.items[:0]
	}
	q.head = 0
	q.bytes = 0
}
