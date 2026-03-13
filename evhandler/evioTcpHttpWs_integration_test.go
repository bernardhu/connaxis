package evhandler

import (
	"errors"
	"net"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/eventloop"
	"github.com/bernardhu/connaxis/websocket"
)

type testAddr string

func (a testAddr) Network() string { return "tcp" }
func (a testAddr) String() string  { return string(a) }

type fakeProtoConn struct {
	id     uint64
	ctx    interface{}
	pkt    connection.IPktHandler
	remote net.Addr
	local  net.Addr
	in     []byte

	closed atomic.Bool
	cmds   []struct {
		cmd  int
		data []byte
	}
}

func (c *fakeProtoConn) ID() uint64                            { return c.id }
func (c *fakeProtoConn) Context() interface{}                  { return c.ctx }
func (c *fakeProtoConn) SetContext(v interface{})              { c.ctx = v }
func (c *fakeProtoConn) LocalAddr() net.Addr                   { return c.local }
func (c *fakeProtoConn) RemoteAddr() net.Addr                  { return c.remote }
func (c *fakeProtoConn) GetLocalAddr() string                  { return c.local.String() }
func (c *fakeProtoConn) GetRemoteAddr() string                 { return c.remote.String() }
func (c *fakeProtoConn) IsClient() bool                        { return false }
func (c *fakeProtoConn) GetPktHandler() connection.IPktHandler { return c.pkt }
func (c *fakeProtoConn) SetPktHandler(h connection.IPktHandler) {
	if c.pkt == nil {
		c.pkt = h
	}
}
func (c *fakeProtoConn) UpdatePktHandler(h connection.IPktHandler) { c.pkt = h }
func (c *fakeProtoConn) Close() error {
	c.closed.Store(true)
	return nil
}
func (c *fakeProtoConn) AddCmd(cmd int, data []byte) error {
	dup := append([]byte(nil), data...)
	c.cmds = append(c.cmds, struct {
		cmd  int
		data []byte
	}{cmd: cmd, data: dup})
	return nil
}

func (c *fakeProtoConn) setInput(in []byte) {
	c.in = append(c.in[:0], in...)
}

type fakeHTTPWSHandler struct {
	dispatchN   atomic.Int64
	userDataN   atomic.Int64
	lastReqPath string
}

func (h *fakeHTTPWSHandler) DispatchHTTP(c connection.AppConn, req *http.Request) {
	_ = c
	h.dispatchN.Add(1)
	if req != nil && req.URL != nil {
		h.lastReqPath = req.URL.Path
	}
	if req != nil && req.Body != nil {
		_ = req.Body.Close()
	}
}

func (h *fakeHTTPWSHandler) OnUserData(ctx *websocket.WsCtx) []byte {
	h.userDataN.Add(1)
	if ctx == nil {
		return nil
	}
	return ctx.Data
}

type fakeTCPHandler struct {
	onReadyN    atomic.Int64
	onConnected atomic.Int64
	onClosedN   atomic.Int64
	parseN      atomic.Int64
	onDataN     atomic.Int64
}

func (h *fakeTCPHandler) OnReady(s eventloop.IServer) { _ = s; h.onReadyN.Add(1) }
func (h *fakeTCPHandler) OnClosed(c connection.AppConn, err error) {
	_ = c
	_ = err
	h.onClosedN.Add(1)
}
func (h *fakeTCPHandler) OnConnected(c connection.ProtoConn) {
	h.onConnected.Add(1)
	c.SetPktHandler(h)
}
func (h *fakeTCPHandler) ParsePacket(c connection.ProtoConn, in *[]byte) (int, int) {
	_ = c
	h.parseN.Add(1)
	return len(*in), len(*in)
}
func (h *fakeTCPHandler) OnData(c connection.ProtoConn, in *[]byte) ([]byte, bool) {
	_ = c
	h.onDataN.Add(1)
	return append([]byte(nil), (*in)...), false
}
func (h *fakeTCPHandler) Stat(bool) {}

func driveOnePacket(c *fakeProtoConn) ([]byte, bool, int, error) {
	h := c.GetPktHandler()
	if h == nil {
		return nil, false, 0, errors.New("nil packet handler")
	}
	data := c.in

	n, _ := h.ParsePacket(c, &data)
	if n <= 0 {
		return nil, false, n, nil
	}
	if n > len(data) {
		return nil, false, n, errors.New("invalid packet length")
	}

	pkt := data[:n]
	out, closeConn := c.GetPktHandler().OnData(c, &pkt)
	return out, closeConn, n, nil
}

func makeHTTPReq(path string) []byte {
	return []byte("GET " + path + " HTTP/1.1\r\nHost: test\r\nConnection: keep-alive\r\n\r\n")
}

func makeWSUpgradeReq(path string) []byte {
	return []byte(
		"GET " + path + " HTTP/1.1\r\n" +
			"Host: test\r\n" +
			"Upgrade: websocket\r\n" +
			"Connection: Upgrade\r\n" +
			"Sec-WebSocket-Version: 13\r\n" +
			"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n",
	)
}

func makeMaskedTextFrame(payload []byte) []byte {
	mask := [4]byte{1, 2, 3, 4}
	out := make([]byte, 2+4+len(payload))
	out[0] = 0x81
	out[1] = byte(0x80 | len(payload))
	copy(out[2:6], mask[:])
	for i := 0; i < len(payload); i++ {
		out[6+i] = payload[i] ^ mask[i%4]
	}
	return out
}

func newFakeConn(id uint64) *fakeProtoConn {
	return &fakeProtoConn{
		id:     id,
		remote: testAddr("127.0.0.1:12345"),
		local:  testAddr("127.0.0.1:8080"),
	}
}

func TestConnaxisTcpHttpWsRouteTCP(t *testing.T) {
	var router ConnaxisTcpHttpWsHandler
	httpws := &fakeHTTPWSHandler{}
	tcp := &fakeTCPHandler{}
	router.Init(httpws, tcp)

	conn := newFakeConn(1)
	router.OnConnected(conn)

	conn.setInput([]byte{0x01, 0x02, 0x03})
	out, closeConn, n, err := driveOnePacket(conn)
	if err != nil {
		t.Fatal(err)
	}
	if closeConn {
		t.Fatalf("unexpected close")
	}
	if n != 3 || len(out) != 3 {
		t.Fatalf("unexpected tcp echo n=%d out=%d", n, len(out))
	}
	if tcp.onConnected.Load() != 1 || tcp.onDataN.Load() != 1 {
		t.Fatalf("tcp handler not hit, connected=%d ondata=%d", tcp.onConnected.Load(), tcp.onDataN.Load())
	}
	if httpws.dispatchN.Load() != 0 || httpws.userDataN.Load() != 0 {
		t.Fatalf("http/ws handler should not be hit")
	}
	if router.tcpOnline.Load() != 1 {
		t.Fatalf("tcp online mismatch: %d", router.tcpOnline.Load())
	}

	router.OnClosed(conn, nil)
	if tcp.onClosedN.Load() != 1 {
		t.Fatalf("tcp close callback not called")
	}
	if router.tcpOnline.Load() != 0 {
		t.Fatalf("tcp online should be 0 after close")
	}
}

func TestConnaxisTcpHttpWsRouteHTTP(t *testing.T) {
	var router ConnaxisTcpHttpWsHandler
	httpws := &fakeHTTPWSHandler{}
	tcp := &fakeTCPHandler{}
	router.Init(httpws, tcp)

	conn := newFakeConn(2)
	router.OnConnected(conn)

	conn.setInput(makeHTTPReq("/health"))
	out, closeConn, n, err := driveOnePacket(conn)
	if err != nil {
		t.Fatal(err)
	}
	if closeConn {
		t.Fatalf("unexpected close")
	}
	if n <= 0 {
		t.Fatalf("invalid packet len: %d", n)
	}
	if len(out) != 0 {
		t.Fatalf("http path should not return direct payload")
	}
	if httpws.dispatchN.Load() != 1 {
		t.Fatalf("http dispatch not hit")
	}
	if httpws.lastReqPath != "/health" {
		t.Fatalf("unexpected path: %s", httpws.lastReqPath)
	}
	if tcp.onConnected.Load() != 0 {
		t.Fatalf("tcp path should not be hit")
	}
	if router.httpws.HTTPOnline() != 1 || router.httpws.WSOnline() != 0 {
		t.Fatalf("http/ws online mismatch, http=%d ws=%d", router.httpws.HTTPOnline(), router.httpws.WSOnline())
	}
}

func TestConnaxisTcpHttpWsUpgradeToWS(t *testing.T) {
	var router ConnaxisTcpHttpWsHandler
	httpws := &fakeHTTPWSHandler{}
	tcp := &fakeTCPHandler{}
	router.Init(httpws, tcp)

	conn := newFakeConn(3)
	router.OnConnected(conn)

	conn.setInput(makeWSUpgradeReq("/ws"))
	hsOut, closeConn, n, err := driveOnePacket(conn)
	if err != nil {
		t.Fatal(err)
	}
	if closeConn {
		t.Fatalf("unexpected close on handshake")
	}
	if n <= 0 {
		t.Fatalf("invalid handshake packet len: %d", n)
	}
	if len(hsOut) == 0 {
		t.Fatalf("missing ws handshake response")
	}
	if string(hsOut[:12]) != "HTTP/1.1 101" {
		t.Fatalf("invalid handshake response: %q", string(hsOut))
	}
	if httpws.dispatchN.Load() != 0 {
		t.Fatalf("upgrade request should not go to http dispatch")
	}
	if router.httpws.WSOnline() != 1 {
		t.Fatalf("ws online mismatch: %d", router.httpws.WSOnline())
	}

	conn.setInput(makeMaskedTextFrame([]byte("hello")))
	frameOut, closeConn, n, err := driveOnePacket(conn)
	if err != nil {
		t.Fatal(err)
	}
	if closeConn {
		t.Fatalf("unexpected close on ws frame")
	}
	if n <= 0 || len(frameOut) == 0 {
		t.Fatalf("invalid ws frame processing n=%d out=%d", n, len(frameOut))
	}
	if frameOut[0]&0x0f != 0x1 {
		t.Fatalf("expected text frame echo, got opcode=%d", frameOut[0]&0x0f)
	}
	if httpws.userDataN.Load() != 1 {
		t.Fatalf("ws user handler not hit")
	}

	router.OnClosed(conn, nil)
	if router.httpws.WSOnline() != 0 {
		t.Fatalf("ws online should be 0 after close")
	}
}
