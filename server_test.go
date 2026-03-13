package connaxis

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/eventloop"
)

type fakeLoop struct {
	id int
}

func (f *fakeLoop) Init(idx, size, chansize, pktsizelimit, cliSbufLimit int) {}
func (f *fakeLoop) AddListener(_ eventloop.IListener)                        {}
func (f *fakeLoop) Run()                                                     {}
func (f *fakeLoop) SetWg(_ *sync.WaitGroup)                                  {}
func (f *fakeLoop) SetHandler(_ eventloop.IHandler)                          {}
func (f *fakeLoop) SetSelector(_ eventloop.ISelector)                        {}
func (f *fakeLoop) AddClient(_ connection.EngineConn) error                  { return nil }
func (f *fakeLoop) AllocID(_ connection.EngineConn)                          {}
func (f *fakeLoop) SetPollWait(_ int)                                        {}
func (f *fakeLoop) Online() int32                                            { return 0 }
func (f *fakeLoop) DialCnt() int32                                           { return 0 }
func (f *fakeLoop) SyncTime(_, _ int64)                                      {}
func (f *fakeLoop) SetIDGen(_ eventloop.IDGenerator)                         {}
func (f *fakeLoop) Stop()                                                    {}
func (f *fakeLoop) Stat(_ int64, _ bool)                                     {}
func (f *fakeLoop) Id() int                                                  { return f.id }

func TestSelectLoopRoundRobin(t *testing.T) {
	s := &Server{}
	s.balance = RoundRobin
	s.loops = []eventloop.IEVLoop{
		&fakeLoop{id: 0},
		&fakeLoop{id: 1},
	}

	first := s.SelectLoop(123)
	second := s.SelectLoop(456)
	third := s.SelectLoop(789)

	if first == nil || second == nil || third == nil {
		t.Fatalf("SelectLoop returned nil")
	}
	if first.Id() != 0 || second.Id() != 1 || third.Id() != 0 {
		t.Fatalf("unexpected round robin order: %d %d %d", first.Id(), second.Id(), third.Id())
	}
}

func TestServeByConfigNil(t *testing.T) {
	if err, _ := ServeByConfig(nil, nil, true); err == nil {
		t.Fatalf("expected error for nil config")
	}
}

func TestLoadInvalidConfig(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "bad.json")
	if err := os.WriteFile(path, []byte("{invalid"), 0o600); err != nil {
		t.Fatalf("write temp: %v", err)
	}

	if _, err := load(path); err == nil {
		t.Fatalf("expected error for invalid json")
	}
}
