package evhandler

import (
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"sync"

	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/eventloop"
	"github.com/bernardhu/connaxis/wrapper"
	"github.com/evanphx/wildcat"
)

type ConnaxisHttpHandler struct {
	dispatch HttpDispatcher
}

func (h *ConnaxisHttpHandler) OnReady(s eventloop.IServer) {
	wrapper.Debugf("echo server started on listen on %v, (loops: %d)", s.GetListenAddrs(), s.GetWorkerNum())
}

func (h *ConnaxisHttpHandler) OnClosed(c connection.AppConn, err error) {
	wrapper.Debugf("conn %s closed", c.RemoteAddr().String())
}

func (h *ConnaxisHttpHandler) OnConnected(c connection.ProtoConn) {
	wrapper.Debugf("conn %s connected", c.RemoteAddr().String())
	c.SetPktHandler(h)
}

type HttpDispatcher interface {
	DispatchHTTP(c connection.AppConn, req *http.Request)
}

type HttpDispatchFunc func(c connection.AppConn, req *http.Request)

func (f HttpDispatchFunc) DispatchHTTP(c connection.AppConn, req *http.Request) {
	f(c, req)
}

type HTTPHandlerOptions struct {
	Workers   int
	QueueSize int // queue length per worker
}

type httpJob struct {
	conn connection.AppConn
	req  *http.Request
}

type httpHandlerDispatcher struct {
	handler http.Handler
	queues  []chan *httpJob

	rwPool sync.Pool
}

var (
	httpParserPool = sync.Pool{
		New: func() interface{} {
			return wildcat.NewHTTPParser()
		},
	}
)

func NewHTTPHandlerDispatcher(handler http.Handler, opts HTTPHandlerOptions) HttpDispatcher {
	if handler == nil {
		return nil
	}

	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	if workers < 1 {
		workers = 1
	}

	queueSize := opts.QueueSize
	if queueSize <= 0 {
		queueSize = 1024
	}

	d := &httpHandlerDispatcher{
		handler: handler,
		queues:  make([]chan *httpJob, workers),
		rwPool: sync.Pool{
			New: func() interface{} {
				w := new(ResponseWriter)
				w.Init()
				return w
			},
		},
	}

	for i := 0; i < workers; i++ {
		ch := make(chan *httpJob, queueSize)
		d.queues[i] = ch
		go d.runWorker(ch)
	}
	return d
}

func (d *httpHandlerDispatcher) DispatchHTTP(c connection.AppConn, req *http.Request) {
	job := &httpJob{conn: c, req: req}
	worker := int(c.ID() % uint64(len(d.queues)))

	select {
	case d.queues[worker] <- job:
	default:
		wrapper.Increment("qps.connaxis.http.reject.queue_full")
		_ = c.AddCmd(connection.CMD_CLOSE, nil)
		if req != nil && req.Body != nil {
			_ = req.Body.Close()
		}
	}
}

func (d *httpHandlerDispatcher) runWorker(queue <-chan *httpJob) {
	for job := range queue {
		d.serve(job)
	}
}

func (d *httpHandlerDispatcher) serve(job *httpJob) {
	w := d.rwPool.Get().(*ResponseWriter)
	w.Reset()

	defer func() {
		if rec := recover(); rec != nil {
			wrapper.Errorf("http handler panic: %v", rec)
			_ = job.conn.AddCmd(connection.CMD_DATA, httpErrorResponse(http.StatusInternalServerError))
		}
		if job.req != nil && job.req.Body != nil {
			_ = job.req.Body.Close()
		}
		d.rwPool.Put(w)
	}()

	d.handler.ServeHTTP(w, job.req)
	if w.Deferred() {
		return
	}
	if err := job.conn.AddCmd(connection.CMD_DATA, w.Bytes()); err != nil {
		wrapper.Increment("qps.connaxis.http.reject.addcmd_error")
	}
}

func acquireHTTPParser() *wildcat.HTTPParser {
	hp := httpParserPool.Get().(*wildcat.HTTPParser)
	resetHTTPParser(hp)
	return hp
}

func releaseHTTPParser(hp *wildcat.HTTPParser) {
	if hp == nil {
		return
	}
	resetHTTPParser(hp)
	httpParserPool.Put(hp)
}

func resetHTTPParser(hp *wildcat.HTTPParser) {
	hp.Method = nil
	hp.Path = nil
	hp.Version = nil
	hp.TotalHeaders = len(hp.Headers)
	for i := range hp.Headers {
		hp.Headers[i].Name = nil
		hp.Headers[i].Value = nil
	}
}

func (h *ConnaxisHttpHandler) ParsePacket(c connection.ProtoConn, in *[]byte) (int, int) {
	if tooLarge, headerLen := headerTooLarge(*in); tooLarge {
		wrapper.Increment("qps.connaxis.http.reject.header_too_large")
		c.SetContext(unsupportedHdrLarge)
		return headerLen, headerLen
	}

	hp := acquireHTTPParser()
	hl, err := hp.Parse(*in)
	if err != nil {
		releaseHTTPParser(hp)
		if err == wildcat.ErrMissingData {
			return 0, len(*in) + 1
		}
		wrapper.Increment("qps.connaxis.http.reject.parse_error")
		return -1, -1
	}
	if marker := unsupportedHeaderMarker(hp); marker != nil {
		releaseHTTPParser(hp)
		switch marker {
		case unsupportedChunked:
			wrapper.Increment("qps.connaxis.http.reject.chunked")
		case unsupportedExpect:
			wrapper.Increment("qps.connaxis.http.reject.expect_continue")
		}
		c.SetContext(marker)
		return hl, hl
	}

	pkglen := hl
	cl := int(hp.ContentLength())
	if MaxBodyBytes > 0 && cl > MaxBodyBytes {
		releaseHTTPParser(hp)
		wrapper.Increment("qps.connaxis.http.reject.body_too_large")
		c.SetContext(unsupportedBodyLarge)
		return hl, hl
	}
	if cl > 0 {
		pkglen = hl + cl
	}

	if len(*in) >= pkglen {
		c.SetContext(hp)
		return pkglen, pkglen
	}

	releaseHTTPParser(hp)
	return 0, pkglen
}

func (h *ConnaxisHttpHandler) OnData(c connection.ProtoConn, in *[]byte) ([]byte, bool) {
	wrapper.Debugf("recv: %s remote: %s", string(*in), c.RemoteAddr())
	v := c.Context()
	if m, ok := v.(*unsupportedHTTP); ok {
		wrapper.Errorf("unsupported http headers")
		return httpErrorResponse(m.code), true
	}
	hp, ok := v.(*wildcat.HTTPParser)
	if !ok {
		wrapper.Errorf("invalid http context type")
		return nil, true
	}
	c.SetContext(nil)
	defer releaseHTTPParser(hp)

	req, err := h.buildRequest(c, hp, in)
	if err != nil {
		wrapper.Errorf("build req fail, err:%v", err)
		return nil, true
	}

	return h.dispatchRequest(c, req)
}

func (h *ConnaxisHttpHandler) Stat(bool) {
}

func (h *ConnaxisHttpHandler) SetDispatcher(dispatch HttpDispatcher) {
	h.dispatch = dispatch
}

func (h *ConnaxisHttpHandler) SetHTTPHandler(handler http.Handler, opts HTTPHandlerOptions) {
	h.dispatch = NewHTTPHandlerDispatcher(handler, opts)
}

func (h *ConnaxisHttpHandler) dispatchRequest(c connection.AppConn, req *http.Request) ([]byte, bool) {
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	if h.dispatch == nil {
		wrapper.Errorf("http dispatcher not set")
		return nil, true
	}
	h.dispatch.DispatchHTTP(c, req)
	return nil, false
}

func (h *ConnaxisHttpHandler) buildRequest(c connection.AppConn, hp *wildcat.HTTPParser, in *[]byte) (*http.Request, error) {
	u, err := url.Parse(fmt.Sprintf("http://%s%s", string(hp.Host()), string(hp.Path)))
	if err != nil {
		return nil, err
	}

	var protoMajor int
	var protoMinor int

	switch string(hp.Version) {
	case "HTTP/0.9":
		protoMinor = 9
	case "HTTP/1.0":
		protoMajor = 1
	case "HTTP/1.1":
		protoMajor = 1
		protoMinor = 1
	}

	req := http.Request{
		Method:        string(hp.Method),
		URL:           u,
		Proto:         string(hp.Version),
		ProtoMajor:    protoMajor,
		ProtoMinor:    protoMinor,
		Header:        ConvertHeader(hp),
		ContentLength: hp.ContentLength(),
		Host:          string(hp.Host()),
		RequestURI:    string(hp.Path),
		RemoteAddr:    c.RemoteAddr().String(),
	}

	cl := int(hp.ContentLength())
	if cl > 0 {
		body := make([]byte, cl)
		copy(body, (*in)[len(*in)-cl:])
		req.Body = &sizedBodyReader{cl, body}
	} else {
		req.Body = &sizedBodyReader{0, nil}
	}

	return &req, nil
}
