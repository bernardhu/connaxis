package connection

import (
	"testing"

	"github.com/bernardhu/connaxis/pool"
	"github.com/bernardhu/connaxis/tuning"
	"golang.org/x/sys/unix"
)

func TestWriteQueueEnqueueCoalesceConsumeAndClear(t *testing.T) {
	pool.Setup(17)

	oldCoalesce := tuning.WriteQueueCoalesceMaxBytes
	oldCompact := tuning.WriteQueueCompactMaxBytes
	oldShrink := tuning.WriteQueueShrinkCap
	defer func() {
		tuning.WriteQueueCoalesceMaxBytes = oldCoalesce
		tuning.WriteQueueCompactMaxBytes = oldCompact
		tuning.WriteQueueShrinkCap = oldShrink
	}()

	tuning.WriteQueueCoalesceMaxBytes = 128
	tuning.WriteQueueCompactMaxBytes = 4 * 1024
	tuning.WriteQueueShrinkCap = 4 * 1024

	var q writeQueue

	owner1 := make([]byte, 16)
	copy(owner1, []byte("abcd"))
	q.enqueue(owner1, 4)
	if got := q.pending(); got != 4 {
		t.Fatalf("pending after first enqueue got=%d want=4", got)
	}
	if got := string(q.peek()); got != "abcd" {
		t.Fatalf("peek after first enqueue got=%q want=%q", got, "abcd")
	}

	owner2 := make([]byte, 16)
	copy(owner2, []byte("EFG"))
	q.enqueue(owner2, 3)
	if got := len(q.items); got != 1 {
		t.Fatalf("queue entries after coalesce got=%d want=1", got)
	}
	if got := q.pending(); got != 7 {
		t.Fatalf("pending after coalesce got=%d want=7", got)
	}
	if got := string(q.peek()); got != "abcdEFG" {
		t.Fatalf("peek after coalesce got=%q want=%q", got, "abcdEFG")
	}

	q.consume(2)
	if got := q.pending(); got != 5 {
		t.Fatalf("pending after partial consume got=%d want=5", got)
	}
	if got := string(q.peek()); got != "cdEFG" {
		t.Fatalf("peek after partial consume got=%q want=%q", got, "cdEFG")
	}

	q.consume(100)
	if got := q.pending(); got != 0 {
		t.Fatalf("pending after full consume got=%d want=0", got)
	}
	if got := q.peek(); got != nil {
		t.Fatalf("peek after full consume got=%v want=nil", got)
	}
	if q.head != 0 || len(q.items) != 0 {
		t.Fatalf("queue should reset to empty head=%d len=%d", q.head, len(q.items))
	}

	owner3 := make([]byte, 8)
	copy(owner3, []byte("zzzz"))
	q.enqueue(owner3, 4)
	q.clear()
	if got := q.pending(); got != 0 {
		t.Fatalf("pending after clear got=%d want=0", got)
	}
	if q.head != 0 || len(q.items) != 0 {
		t.Fatalf("queue should be empty after clear head=%d len=%d", q.head, len(q.items))
	}
}

func TestFlushWriteQueueBadFD(t *testing.T) {
	pool.Setup(17)

	var q writeQueue
	var send int32

	owner := make([]byte, 8)
	copy(owner, []byte("abcd"))
	q.enqueue(owner, 4)

	n, err := flushWriteQueue(0, &q, 0, &send)
	if n != 0 {
		t.Fatalf("flush bytes with bad fd got=%d want=0", n)
	}
	if err != unix.EBADF {
		t.Fatalf("flush error with bad fd got=%v want=%v", err, unix.EBADF)
	}
	if got := q.pending(); got != 4 {
		t.Fatalf("pending after bad flush got=%d want=4", got)
	}
	if send != 0 {
		t.Fatalf("send counter after bad flush got=%d want=0", send)
	}
}
