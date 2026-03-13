package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/bernardhu/connaxis"
	"github.com/bernardhu/connaxis/eventloop"
	evhandler "github.com/bernardhu/connaxis/evhandler"
)

type handler struct {
	evhandler.ConnaxisHttpHandler
}

func (h *handler) OnReady(s eventloop.IServer) {
	log.Printf("ready: listen on %v (loops: %d)", s.GetListenAddrs(), s.GetWorkerNum())
}

func main() {
	var path string
	var workers int
	var queueSize int
	flag.StringVar(&path, "p", "conf/connaxis.conf", "config file path")
	flag.IntVar(&workers, "workers", 0, "http worker count (default: GOMAXPROCS)")
	flag.IntVar(&queueSize, "queue", 1024, "http job queue size per worker")
	flag.Parse()

	h := &handler{}
	h.SetHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}), evhandler.HTTPHandlerOptions{
		Workers:   workers,
		QueueSize: queueSize,
	})

	if err, _ := connaxis.Serve(h, path); err != nil {
		log.Fatal(err)
	}
}
