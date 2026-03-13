package eventloop

import (
	"testing"

	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/pool"
)

type fixedIDGenerator struct {
	id uint64
}

func (g fixedIDGenerator) GetID() uint64 {
	return g.id
}

func TestBuildSimpleID(t *testing.T) {
	got := buildSimpleID(0x11223344, 0x5566, 0x0077)
	want := uint64(0x11223344)<<32 | uint64(0x5566)<<16 | uint64(0x0077)
	if got != want {
		t.Fatalf("buildSimpleID got=0x%x want=0x%x", got, want)
	}
}

func TestCmdDataReset(t *testing.T) {
	pool.Setup(17)

	cmd := &CmdData{
		cmd:  1,
		fd:   2,
		id:   3,
		data: make([]byte, 1024),
		size: 512,
	}

	cmd.reset()

	if cmd.cmd != 0 || cmd.fd != 0 || cmd.id != 0 || cmd.size != 0 {
		t.Fatalf("reset fields mismatch cmd=%d fd=%d id=%d size=%d", cmd.cmd, cmd.fd, cmd.id, cmd.size)
	}
	if cmd.data != nil {
		t.Fatalf("reset data should be nil")
	}
}

func TestLoopConnAllocIDWithAndWithoutGenerator(t *testing.T) {
	l := &LoopConn{idx: 7}
	c := &connection.Conn{}
	c.SetFd(33)

	l.AllocID(c)
	want1 := buildSimpleID(1, 33, 7)
	if got := c.ID(); got != want1 {
		t.Fatalf("AllocID fallback got=%d want=%d", got, want1)
	}

	l.SetIDGen(fixedIDGenerator{id: 999})
	l.AllocID(c)
	if got := c.ID(); got != 999 {
		t.Fatalf("AllocID with generator got=%d want=999", got)
	}
}

func TestLoopConnAddClientChannelFull(t *testing.T) {
	l := &LoopConn{
		acceptChan: make(chan connection.EngineConn, 1),
		triggered:  1, // skip poll trigger path in this unit test
	}

	c1 := &connection.Conn{}
	c1.SetFd(1)
	c2 := &connection.Conn{}
	c2.SetFd(2)

	if err := l.AddClient(c1); err != nil {
		t.Fatalf("first AddClient err=%v", err)
	}
	if err := l.AddClient(c2); err == nil {
		t.Fatalf("second AddClient expected chan full error")
	}
}
