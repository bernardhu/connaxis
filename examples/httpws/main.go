package main

import (
	"flag"
	"log"
	"net/http"
	"runtime"

	"github.com/bernardhu/connaxis"
	"github.com/bernardhu/connaxis/connection"
	evhandler "github.com/bernardhu/connaxis/evhandler"
	"github.com/bernardhu/connaxis/websocket"
)

type httpJob struct {
	conn connection.AppConn
	req  *http.Request
}

type httpWsHandler struct {
	queue     chan *httpJob
	workers   int
	queueSize int
}

func (h *httpWsHandler) DispatchHTTP(c connection.AppConn, req *http.Request) {
	h.queue <- &httpJob{conn: c, req: req}
}

func (h *httpWsHandler) OnUserData(ctx *websocket.WsCtx) []byte {
	return ctx.Data
}

func (h *httpWsHandler) Start() {
	h.queue = make(chan *httpJob, h.queueSize)
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
				w.Write([]byte("ok"))
				_ = job.conn.AddCmd(connection.CMD_DATA, w.Bytes())
			}
		}()
	}
}

func main() {
	var path string
	var workers int
	var queueSize int
	flag.StringVar(&path, "p", "conf/connaxis.conf", "config file path")
	flag.IntVar(&workers, "workers", 0, "http worker count (default: GOMAXPROCS)")
	flag.IntVar(&queueSize, "queue", 1024, "http job queue size")
	flag.Parse()

	var h evhandler.ConnaxisHttpWsHandler
	impl := &httpWsHandler{}
	impl.workers = workers
	impl.queueSize = queueSize
	impl.Start()
	h.Init(impl)

	if err, _ := connaxis.Serve(&h, path); err != nil {
		log.Fatal(err)
	}
}
