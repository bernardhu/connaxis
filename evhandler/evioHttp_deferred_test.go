package evhandler

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bernardhu/connaxis/connection"
)

func TestHTTPHandlerDispatcherDeferredResponseSkipsImmediateWrite(t *testing.T) {
	dispatcher := NewHTTPHandlerDispatcher(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		rw, ok := w.(*ResponseWriter)
		if !ok {
			t.Fatalf("writer type = %T", w)
		}
		rw.Defer()
	}), HTTPHandlerOptions{Workers: 1, QueueSize: 1})

	conn := newFakeConn(1)
	req, err := http.NewRequest(http.MethodGet, "/hello", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	dispatcher.DispatchHTTP(conn, req)
	waitUntil(t, func() bool { return len(conn.cmds) == 0 })
	if len(conn.cmds) != 0 {
		t.Fatalf("cmds = %d, want 0", len(conn.cmds))
	}
}

func TestHTTPHandlerDispatcherWritesImmediateResponseWhenNotDeferred(t *testing.T) {
	dispatcher := NewHTTPHandlerDispatcher(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}), HTTPHandlerOptions{Workers: 1, QueueSize: 1})

	conn := newFakeConn(2)
	req, err := http.NewRequest(http.MethodGet, "/hello", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	dispatcher.DispatchHTTP(conn, req)
	waitUntil(t, func() bool { return len(conn.cmds) == 1 })
	if len(conn.cmds) != 1 {
		t.Fatalf("cmds = %d, want 1", len(conn.cmds))
	}
	if conn.cmds[0].cmd != connection.CMD_DATA {
		t.Fatalf("cmd = %d, want %d", conn.cmds[0].cmd, connection.CMD_DATA)
	}
	if !strings.Contains(string(conn.cmds[0].data), "HTTP/1.1 200 OK") {
		t.Fatalf("response = %q", string(conn.cmds[0].data))
	}
}

func waitUntil(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}
