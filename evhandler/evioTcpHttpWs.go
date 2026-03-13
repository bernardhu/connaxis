package evhandler

import (
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/eventloop"
	"github.com/bernardhu/connaxis/websocket"
	"github.com/bernardhu/connaxis/wrapper"
)

type TcpProtoHandler interface {
	connection.IPktHandler
	OnReady(s eventloop.IServer)
	OnClosed(c connection.AppConn, err error)
	OnConnected(c connection.ProtoConn)
}

type connMode uint8

const (
	connModeUnknown connMode = iota
	connModeHTTP
	connModeWS
	connModeTCP
)

type ConnaxisTcpHttpWsHandler struct {
	httpws ConnaxisHttpWsHandler
	tcp    TcpProtoHandler

	tcpConns  sync.Map
	tcpOnline atomic.Int64
}

func (h *ConnaxisTcpHttpWsHandler) Init(httpWsHandler HttpWsHandler, tcpHandler TcpProtoHandler) {
	h.httpws.Init(httpWsHandler)
	h.tcp = tcpHandler
}

func (h *ConnaxisTcpHttpWsHandler) OnReady(s eventloop.IServer) {
	if h.tcp != nil {
		h.tcp.OnReady(s)
	}
	h.httpws.OnReady(s)

	wrapper.Debugf("tcp/http/ws server started on listen on %v, (loops: %d)", s.GetListenAddrs(), s.GetWorkerNum())
}

func (h *ConnaxisTcpHttpWsHandler) OnClosed(c connection.AppConn, err error) {
	id := c.ID()

	if _, ok := h.tcpConns.LoadAndDelete(id); ok {
		h.tcpOnline.Add(-1)
		if h.tcp != nil {
			h.tcp.OnClosed(c, err)
		}
		return
	}

	if _, ok := h.httpws.clis.Load(id); ok {
		h.httpws.OnClosed(c, err)
		return
	}

	c.SetContext(nil)
}

func (h *ConnaxisTcpHttpWsHandler) OnConnected(c connection.ProtoConn) {
	c.SetPktHandler(h)
}

func (h *ConnaxisTcpHttpWsHandler) ParsePacket(c connection.ProtoConn, in *[]byte) (int, int) {
	if _, ok := h.tcpConns.Load(c.ID()); ok {
		c.UpdatePktHandler(h.tcp)
		return h.tcp.ParsePacket(c, in)
	}

	mode, expect := sniffProtoByFirstLine(*in)
	switch mode {
	case connModeHTTP:
		return h.routeHTTP(c, in)
	case connModeTCP:
		return h.routeTCP(c, in)
	default:
		return 0, expect
	}
}

func (h *ConnaxisTcpHttpWsHandler) OnData(c connection.ProtoConn, in *[]byte) ([]byte, bool) {
	if _, ok := h.tcpConns.Load(c.ID()); ok && h.tcp != nil {
		return h.tcp.OnData(c, in)
	}
	if _, ok := h.httpws.clis.Load(c.ID()); ok {
		return h.httpws.OnData(c, in)
	}
	wrapper.Errorf("invalid tcp/http/ws state id:%d", c.ID())
	return nil, true
}

func (h *ConnaxisTcpHttpWsHandler) Stat(print bool) {
	httpOnline := h.httpws.HTTPOnline()
	wsOnline := h.httpws.WSOnline()
	tcpOnline := h.tcpOnline.Load()

	wrapper.Gauge("qps.connaxis.tcphttpws.conn.http", httpOnline)
	wrapper.Gauge("qps.connaxis.tcphttpws.conn.ws", wsOnline)
	wrapper.Gauge("qps.connaxis.tcphttpws.conn.tcp", tcpOnline)
	if print {
		wrapper.Infof("qps.connaxis.tcphttpws conn stats, http:%d ws:%d tcp:%d", httpOnline, wsOnline, tcpOnline)
	}

	if h.tcp != nil {
		h.tcp.Stat(print)
	}
}

func (h *ConnaxisTcpHttpWsHandler) GetCli(connId uint64) connection.AppConn {
	return h.httpws.GetCli(connId)
}

func (h *ConnaxisTcpHttpWsHandler) DispatchHTTP(c connection.AppConn, req *http.Request) {
	if h.httpws.h != nil {
		h.httpws.h.DispatchHTTP(c, req)
	}
}

func (h *ConnaxisTcpHttpWsHandler) OnUserData(ctx *websocket.WsCtx) []byte {
	if h.httpws.h != nil {
		return h.httpws.h.OnUserData(ctx)
	}
	return nil
}

func (h *ConnaxisTcpHttpWsHandler) routeHTTP(c connection.ProtoConn, in *[]byte) (int, int) {
	h.httpws.trackConnected(c)
	c.UpdatePktHandler(&h.httpws)
	return h.httpws.ParsePacket(c, in)
}

func (h *ConnaxisTcpHttpWsHandler) routeTCP(c connection.ProtoConn, in *[]byte) (int, int) {
	if h.tcp == nil {
		wrapper.Errorf("tcp handler not set")
		return -1, -1
	}

	id := c.ID()
	if _, ok := h.tcpConns.Load(id); !ok {
		h.tcpConns.Store(id, true)
		h.tcpOnline.Add(1)
		h.tcp.OnConnected(c)
		c.UpdatePktHandler(h.tcp)
	}
	return h.tcp.ParsePacket(c, in)
}

func sniffProtoByFirstLine(in []byte) (connMode, int) {
	if len(in) == 0 {
		return connModeUnknown, 1
	}

	methodEnd := -1
	limit := len(in)
	if limit > 16 {
		limit = 16
	}

	for i := 0; i < limit; i++ {
		ch := in[i]
		if ch == ' ' {
			methodEnd = i
			break
		}
		if ch < 'A' || ch > 'Z' {
			return connModeTCP, 0
		}
	}

	if methodEnd == -1 {
		if limit < len(in) {
			return connModeTCP, 0
		}
		if isHTTPMethodPrefix(in[:limit]) {
			return connModeUnknown, len(in) + 1
		}
		return connModeTCP, 0
	}

	if isHTTPMethod(in[:methodEnd]) {
		return connModeHTTP, 0
	}
	return connModeTCP, 0
}

func isHTTPMethodPrefix(in []byte) bool {
	if len(in) == 0 {
		return true
	}
	for _, method := range httpMethods {
		if len(in) > len(method) {
			continue
		}
		match := true
		for i := 0; i < len(in); i++ {
			if in[i] != method[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func isHTTPMethod(in []byte) bool {
	for _, method := range httpMethods {
		if len(in) != len(method) {
			continue
		}
		match := true
		for i := 0; i < len(in); i++ {
			if in[i] != method[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

var httpMethods = [][]byte{
	[]byte("GET"),
	[]byte("POST"),
	[]byte("PUT"),
	[]byte("PATCH"),
	[]byte("DELETE"),
	[]byte("HEAD"),
	[]byte("OPTIONS"),
	[]byte("CONNECT"),
	[]byte("TRACE"),
}
