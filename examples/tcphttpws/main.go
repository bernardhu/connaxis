package main

import (
	"flag"
	"log"
	"net/http"
	"runtime"

	"github.com/bernardhu/connaxis"
	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/eventloop"
	evhandler "github.com/bernardhu/connaxis/evhandler"
	"github.com/bernardhu/connaxis/websocket"
)

type httpJob struct {
	conn connection.AppConn
	req  *http.Request
}

type httpWsApp struct {
	queue   chan *httpJob
	workers int
}

func (h *httpWsApp) DispatchHTTP(c connection.AppConn, req *http.Request) {
	job := &httpJob{conn: c, req: req}
	select {
	case h.queue <- job:
	default:
		w := new(evhandler.ResponseWriter)
		w.Init()
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("busy"))
		_ = c.AddCmd(connection.CMD_DATA, w.Bytes())
		if req != nil && req.Body != nil {
			_ = req.Body.Close()
		}
	}
}

func (h *httpWsApp) OnUserData(ctx *websocket.WsCtx) []byte {
	return ctx.Data
}

func (h *httpWsApp) start() {
	workers := h.workers
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	if workers < 1 {
		workers = 1
	}
	for i := 0; i < workers; i++ {
		go func() {
			for job := range h.queue {
				w := new(evhandler.ResponseWriter)
				w.Init()
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
				_ = job.conn.AddCmd(connection.CMD_DATA, w.Bytes())
				if job.req != nil && job.req.Body != nil {
					_ = job.req.Body.Close()
				}
			}
		}()
	}
}

type tcpEchoHandler struct{}

func (h *tcpEchoHandler) OnReady(s eventloop.IServer) {
}

func (h *tcpEchoHandler) OnClosed(c connection.AppConn, err error) {
}

func (h *tcpEchoHandler) OnConnected(c connection.ProtoConn) {
	c.SetPktHandler(h)
}

func (h *tcpEchoHandler) ParsePacket(c connection.ProtoConn, in *[]byte) (int, int) {
	return len(*in), len(*in)
}

func (h *tcpEchoHandler) OnData(c connection.ProtoConn, in *[]byte) ([]byte, bool) {
	return *in, false
}

func (h *tcpEchoHandler) Stat(bool) {}

func main() {
	var path string
	var workers int
	var queueSize int
	flag.StringVar(&path, "p", "conf/connaxis.conf", "config file path")
	flag.IntVar(&workers, "workers", 0, "http worker count (default: GOMAXPROCS)")
	flag.IntVar(&queueSize, "queue", 1024, "http job queue size")
	flag.Parse()

	app := &httpWsApp{
		queue:   make(chan *httpJob, queueSize),
		workers: workers,
	}
	app.start()

	tcp := &tcpEchoHandler{}
	var h evhandler.ConnaxisTcpHttpWsHandler
	h.Init(app, tcp)

	if err, _ := connaxis.Serve(&h, path); err != nil {
		log.Fatal(err)
	}
}
