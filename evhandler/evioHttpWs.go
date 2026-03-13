package evhandler

import (
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/eventloop"
	"github.com/bernardhu/connaxis/websocket"
	"github.com/bernardhu/connaxis/wrapper"
	"github.com/evanphx/wildcat"
)

type HttpWsHandler interface {
	OnUserData(*websocket.WsCtx) []byte
	DispatchHTTP(c connection.AppConn, req *http.Request)
}

type ConnaxisHttpWsHandler struct {
	evhttp *ConnaxisHttpHandler
	ws     *websocket.WsHandler

	clis sync.Map
	wsID sync.Map
	// online includes both http and ws connections.
	online   atomic.Int64
	wsOnline atomic.Int64
	h        HttpWsHandler
}

func (h *ConnaxisHttpWsHandler) OnReady(s eventloop.IServer) {
	wrapper.Debugf("http/ws server started on listen on %v, (loops: %d)", s.GetListenAddrs(), s.GetWorkerNum())
}

func (h *ConnaxisHttpWsHandler) OnClosed(c connection.AppConn, err error) {
	h.trackClosed(c.ID())
	if h.ws != nil {
		h.ws.OnClosed(c, err)
	} else {
		c.SetContext(nil)
	}
	wrapper.Debugf("conn %s closed", c.RemoteAddr().String())
}

func (h *ConnaxisHttpWsHandler) OnConnected(c connection.ProtoConn) {
	wrapper.Debugf("conn %s connected", c.RemoteAddr().String())
	c.SetPktHandler(h)
	h.trackConnected(c)
}

func (h *ConnaxisHttpWsHandler) ParsePacket(c connection.ProtoConn, in *[]byte) (int, int) {
	if h.evhttp == nil {
		h.evhttp = new(ConnaxisHttpHandler)
	}
	return h.evhttp.ParsePacket(c, in)
}

func (h *ConnaxisHttpWsHandler) OnData(c connection.ProtoConn, in *[]byte) ([]byte, bool) {
	v := c.Context()
	if m, ok := v.(*unsupportedHTTP); ok {
		wrapper.Errorf("unsupported http headers")
		return httpErrorResponse(m.code), true
	}
	hp, ok := v.(*wildcat.HTTPParser)
	if !ok {
		wrapper.Errorf("invalid http ws context type")
		return nil, true
	}
	c.SetContext(nil)
	defer releaseHTTPParser(hp)

	req, err := h.evhttp.buildRequest(c, hp, in)
	if err != nil {
		wrapper.Errorf("build req fail, err:%v", err)
		return nil, true
	}

	if isWebSocketUpgrade(req) {
		if h.ws == nil {
			wrapper.Errorf("ws handler not set")
			return nil, true
		}
		h.markWS(c.ID())
		c.UpdatePktHandler(h.ws)
		cli := &websocket.WsClient{
			Upgraded: false,
			Cid:      c.ID(),
		}
		c.SetContext(cli)
		length, _ := h.ws.ParsePacket(c, in)
		if length <= 0 || length > len(*in) {
			wrapper.Errorf("invalid ws handshake packet, len:%d in:%d", length, len(*in))
			return nil, true
		}
		wsIn := (*in)[:length]
		wrapper.Debugf("see upgrade will try ws")

		//if req.Header.Get("Sec-WebSocket-Protocol") == "msg-json" {
		//	pl := "{\"data\":\"connected\",\"event\":\"sys\"}"
		//	data := websocket.BuildWsPkt([]byte(pl), websocket.MessageText)
		//	c.AddCmd(1, &data)
		//}
		return h.ws.OnData(c, &wsIn)
	}

	return h.evhttp.dispatchRequest(c, req)
}

func (h *ConnaxisHttpWsHandler) Stat(print bool) {
	httpOnline := h.HTTPOnline()
	wsOnline := h.WSOnline()

	wrapper.Gauge("qps.connaxis.httpws.conn.http", httpOnline)
	wrapper.Gauge("qps.connaxis.httpws.conn.ws", wsOnline)
	wrapper.Gauge("qps.connaxis.httpws.conn.tcp", 0)
	if print {
		wrapper.Infof("qps.connaxis.httpws conn stats, http:%d ws:%d tcp:%d", httpOnline, wsOnline, 0)
	}
}

func isWebSocketUpgrade(req *http.Request) bool {
	if req == nil {
		return false
	}
	conn := strings.ToLower(req.Header.Get("Connection"))
	if !strings.Contains(conn, "upgrade") {
		return false
	}
	if !strings.EqualFold(req.Header.Get("Upgrade"), "websocket") {
		return false
	}
	if req.Header.Get("Sec-WebSocket-Key") == "" {
		return false
	}
	if req.Header.Get("Sec-WebSocket-Version") != "13" {
		return false
	}
	return true
}

func (h *ConnaxisHttpWsHandler) Init(handler HttpWsHandler) {
	h.h = handler
	h.evhttp = new(ConnaxisHttpHandler)
	h.evhttp.SetDispatcher(HttpDispatchFunc(handler.DispatchHTTP))
	h.ws = new(websocket.WsHandler)
	h.ws.SetUserDataHandler(handler)
}

func (h *ConnaxisHttpWsHandler) HTTPOnline() int64 {
	online := h.online.Load()
	wsOnline := h.wsOnline.Load()
	if wsOnline > online {
		return 0
	}
	return online - wsOnline
}

func (h *ConnaxisHttpWsHandler) WSOnline() int64 {
	return h.wsOnline.Load()
}

func (h *ConnaxisHttpWsHandler) GetCli(connId uint64) connection.AppConn {
	val, ok := h.clis.Load(connId)
	if ok {
		c := val.(connection.AppConn)
		return c
	} else {
		return nil
	}
}

func (h *ConnaxisHttpWsHandler) trackConnected(c connection.AppConn) {
	id := c.ID()
	if _, loaded := h.clis.LoadOrStore(id, c); !loaded {
		h.online.Add(1)
		return
	}
	// Refresh stored conn in case the wrapper changed.
	h.clis.Store(id, c)
}

func (h *ConnaxisHttpWsHandler) trackClosed(id uint64) {
	if _, ok := h.clis.LoadAndDelete(id); ok {
		h.online.Add(-1)
	}
	if _, ok := h.wsID.LoadAndDelete(id); ok {
		h.wsOnline.Add(-1)
	}
}

func (h *ConnaxisHttpWsHandler) markWS(id uint64) {
	if _, loaded := h.wsID.LoadOrStore(id, true); !loaded {
		h.wsOnline.Add(1)
	}
}
