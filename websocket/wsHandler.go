package websocket

import (
	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/wrapper"
)

type UserDataHandler interface {
	OnUserData(*WsCtx) []byte
}

type WsHandler struct {
	uHandler UserDataHandler

	// EnablePlainHTTP allows a minimal HTTP response for non-WebSocket GET requests.
	// Default is false to preserve strict WebSocket-only behavior.
	EnablePlainHTTP bool
}

func (ws *WsHandler) OnClosed(c connection.AppConn, err error) {
	wrapper.Debugf("OnClosed id:%d", c.ID())
	if ctx := c.Context(); ctx != nil {
		if cli, ok := ctx.(*WsClient); ok {
			cli.resetMessageState()
			cli.closePMD()
		}
	}
	c.SetContext(nil)
}

func (ws *WsHandler) OnConnected(c connection.ProtoConn) {
	wrapper.Debugf("OnConnected id:%d", c.ID())
	c.SetPktHandler(ws)
	cli := &WsClient{
		Upgraded:       false,
		Cid:            c.ID(),
		AllowPlainHTTP: ws.EnablePlainHTTP,
	}
	c.SetContext(cli)
}

func (ws *WsHandler) ParsePacket(c connection.ProtoConn, in *[]byte) (length, expect int) {
	//wrapper.Debugf("ParsePacket fd:%d len:%d", c.ID(), len(*in))
	cli := c.Context().(*WsClient)
	if !cli.Upgraded {
		return parseHandshake(cli, *in)
	}
	return parseFrame(*in)
}

func (ws *WsHandler) OnData(c connection.ProtoConn, in *[]byte) (out []byte, close bool) {
	//wrapper.Debugf("OnData fd:%d len:%d", c.ID(), len(*in))
	cli := c.Context().(*WsClient)

	if !cli.Upgraded {
		return processHandshake(cli)
	}
	return processFrame(cli, *in, ws.uHandler)
}

func (ws *WsHandler) SetUserDataHandler(uh UserDataHandler) {
	ws.uHandler = uh
}

func (ws *WsHandler) Stat(bool) {
}
