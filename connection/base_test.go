package connection

import "testing"

type mockCmdReceiver struct {
	cmd  int
	fd   int
	dial bool
	id   uint64
	data []byte
	err  error
}

func (m *mockCmdReceiver) AddCmd(cmd, fd int, dial bool, id uint64, data []byte) error {
	m.cmd = cmd
	m.fd = fd
	m.dial = dial
	m.id = id
	m.data = data
	return m.err
}

func TestBaseAddCmdForwardsFields(t *testing.T) {
	m := &mockCmdReceiver{}
	c := &Base{}
	c.SetReceiver(m)
	c.SetFd(123)
	c.SetID(456)
	c.Client = true

	payload := []byte("hello")
	if err := c.AddCmd(CMD_DATA, payload); err != nil {
		t.Fatalf("AddCmd returned err=%v", err)
	}

	if m.cmd != CMD_DATA || m.fd != 123 || !m.dial || m.id != 456 {
		t.Fatalf("forwarded fields mismatch cmd=%d fd=%d dial=%v id=%d", m.cmd, m.fd, m.dial, m.id)
	}
	if string(m.data) != "hello" {
		t.Fatalf("forwarded payload mismatch got=%q want=%q", string(m.data), "hello")
	}
}

func TestBaseGetSendGetRecvReset(t *testing.T) {
	c := &Base{}
	c.send = 11
	c.recv = 7

	if got := c.GetSend(false); got != 11 {
		t.Fatalf("GetSend(false) got=%d want=11", got)
	}
	if got := c.GetSend(true); got != 11 {
		t.Fatalf("GetSend(true) got=%d want=11", got)
	}
	if got := c.GetSend(false); got != 0 {
		t.Fatalf("GetSend(false) after reset got=%d want=0", got)
	}

	if got := c.GetRecv(false); got != 7 {
		t.Fatalf("GetRecv(false) got=%d want=7", got)
	}
	if got := c.GetRecv(true); got != 7 {
		t.Fatalf("GetRecv(true) got=%d want=7", got)
	}
	if got := c.GetRecv(false); got != 0 {
		t.Fatalf("GetRecv(false) after reset got=%d want=0", got)
	}
}
