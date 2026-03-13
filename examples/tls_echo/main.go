package main

import (
	"flag"
	"log"

	"github.com/bernardhu/connaxis"
	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/eventloop"
)

type handler struct{}

func (h *handler) OnReady(s eventloop.IServer) {
	log.Printf("ready: listen on %v (loops: %d)", s.GetListenAddrs(), s.GetWorkerNum())
}

func (h *handler) OnClosed(c connection.AppConn, err error) {
	_ = err
	log.Printf("closed: %s", c.GetRemoteAddr())
}

func (h *handler) OnConnected(c connection.ProtoConn) {
	c.SetPktHandler(h)
	log.Printf("connected: %s", c.GetRemoteAddr())
}

func (h *handler) ParsePacket(c connection.ProtoConn, in *[]byte) (int, int) {
	_ = c
	return len(*in), len(*in)
}

func (h *handler) OnData(c connection.ProtoConn, in *[]byte) ([]byte, bool) {
	_ = c
	return *in, false
}

func (h *handler) Stat(bool) {}

func main() {
	var path string
	flag.StringVar(&path, "p", "conf/connaxisssl.conf", "config file path")
	flag.Parse()

	var h handler
	if err, _ := connaxis.Serve(&h, path); err != nil {
		log.Fatal(err)
	}
}
