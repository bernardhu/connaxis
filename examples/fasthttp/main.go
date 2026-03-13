package main

import (
	"flag"
	"log"
	"runtime"

	"github.com/bernardhu/connaxis"
	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/eventloop"
	evhandler "github.com/bernardhu/connaxis/evhandler"
	"github.com/valyala/fasthttp"
)

type fastJob struct {
	conn connection.AppConn
	req  *fasthttp.Request
}

type handler struct {
	evhandler.ConnaxisFastHTTPHandler
	queue     chan *fastJob
	workers   int
	queueSize int
}

func (h *handler) OnReady(s eventloop.IServer) {
	log.Printf("ready: listen on %v (loops: %d)", s.GetListenAddrs(), s.GetWorkerNum())
}

func (h *handler) DispatchFast(c connection.AppConn, req *fasthttp.Request) {
	// Copy request for async processing.
	dup := fasthttp.AcquireRequest()
	req.CopyTo(dup)
	evhandler.ReleaseRequest(req)
	h.queue <- &fastJob{conn: c, req: dup}
}

func (h *handler) Start() {
	h.queue = make(chan *fastJob, h.queueSize)
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
				resp := fasthttp.AcquireResponse()
				resp.SetStatusCode(fasthttp.StatusOK)
				resp.SetBodyString("ok")

				out, err := evhandler.EncodeResponse(resp)
				fasthttp.ReleaseResponse(resp)
				evhandler.ReleaseRequest(job.req)
				if err == nil {
					_ = job.conn.AddCmd(connection.CMD_DATA, out)
				}
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

	h := &handler{}
	h.workers = workers
	h.queueSize = queueSize
	h.Start()
	h.SetDispatcher(h)

	if err, _ := connaxis.Serve(h, path); err != nil {
		log.Fatal(err)
	}
}
