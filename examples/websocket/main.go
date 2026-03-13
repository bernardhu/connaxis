package main

import (
	"flag"
	"log"

	"github.com/bernardhu/connaxis"
	"github.com/bernardhu/connaxis/eventloop"
	"github.com/bernardhu/connaxis/websocket"
)

type wsUserHandler struct{}

func (h *wsUserHandler) OnUserData(ctx *websocket.WsCtx) []byte {
	return ctx.Data
}

type handler struct {
	websocket.WsHandler
}

func (h *handler) OnReady(s eventloop.IServer) {
	log.Printf("ready: listen on %v (loops: %d)", s.GetListenAddrs(), s.GetWorkerNum())
}

func main() {
	var path string
	flag.StringVar(&path, "p", "conf/connaxis.conf", "config file path")
	flag.Parse()

	h := &handler{}
	h.SetUserDataHandler(&wsUserHandler{})

	if err, _ := connaxis.Serve(h, path); err != nil {
		log.Fatal(err)
	}
}
