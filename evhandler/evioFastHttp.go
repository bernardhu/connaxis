package evhandler

import (
	"bufio"
	"bytes"

	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/eventloop"
	"github.com/bernardhu/connaxis/wrapper"
	"github.com/valyala/fasthttp"
)

var (
	fastHeaderContentLength    = []byte("Content-Length:")
	fastHeaderTransferEncoding = []byte("Transfer-Encoding:")
	fastHeaderExpect           = []byte("Expect:")
	fastHeaderChunked          = []byte("chunked")
	fastTokenContinue          = []byte("100-continue")
)

type unsupportedFastHTTP struct {
	code int
}

var (
	unsupportedChunkedFast   = &unsupportedFastHTTP{code: fasthttp.StatusNotImplemented}
	unsupportedExpectFast    = &unsupportedFastHTTP{code: fasthttp.StatusExpectationFailed}
	unsupportedHdrLargeFast  = &unsupportedFastHTTP{code: fasthttp.StatusRequestHeaderFieldsTooLarge}
	unsupportedBodyLargeFast = &unsupportedFastHTTP{code: fasthttp.StatusRequestEntityTooLarge}
)

type FastDispatcher interface {
	DispatchFast(c connection.AppConn, req *fasthttp.Request)
}

type FastDispatchFunc func(c connection.AppConn, req *fasthttp.Request)

func (f FastDispatchFunc) DispatchFast(c connection.AppConn, req *fasthttp.Request) {
	f(c, req)
}

// ConnaxisFastHTTPHandler is a fasthttp-based handler adapter.
// The dispatcher owns req and must call ReleaseRequest when done.
type ConnaxisFastHTTPHandler struct {
	dispatch FastDispatcher
}

func (h *ConnaxisFastHTTPHandler) OnReady(s eventloop.IServer) {
	wrapper.Debugf("fasthttp server started on listen on %v, (loops: %d)", s.GetListenAddrs(), s.GetWorkerNum())
}

func (h *ConnaxisFastHTTPHandler) OnClosed(c connection.AppConn, err error) {
	wrapper.Debugf("conn %s closed", c.RemoteAddr().String())
}

func (h *ConnaxisFastHTTPHandler) OnConnected(c connection.ProtoConn) {
	wrapper.Debugf("conn %s connected", c.RemoteAddr().String())
	c.SetPktHandler(h)
}

func (h *ConnaxisFastHTTPHandler) ParsePacket(c connection.ProtoConn, in *[]byte) (int, int) {
	data := *in
	if MaxHeaderBytes > 0 {
		if headerEnd := bytes.Index(data, []byte("\r\n\r\n")); headerEnd == -1 {
			if len(data) > MaxHeaderBytes {
				wrapper.Increment("qps.connaxis.fasthttp.reject.header_too_large")
				c.SetContext(unsupportedHdrLargeFast)
				return len(data), len(data)
			}
		}
	}
	headerEnd := bytes.Index(data, []byte("\r\n\r\n"))
	if headerEnd == -1 {
		return 0, -1
	}

	headerLen := headerEnd + 4
	if MaxHeaderBytes > 0 && headerLen > MaxHeaderBytes {
		wrapper.Increment("qps.connaxis.fasthttp.reject.header_too_large")
		c.SetContext(unsupportedHdrLargeFast)
		return headerLen, headerLen
	}
	cl, chunked, expectContinue, ok := parseContentLength(data[:headerLen])
	if !ok {
		wrapper.Increment("qps.connaxis.fasthttp.reject.parse_error")
		return -1, -1
	}
	if chunked {
		wrapper.Increment("qps.connaxis.fasthttp.reject.chunked")
		c.SetContext(unsupportedChunkedFast)
		return headerLen, headerLen
	}
	if expectContinue {
		wrapper.Increment("qps.connaxis.fasthttp.reject.expect_continue")
		c.SetContext(unsupportedExpectFast)
		return headerLen, headerLen
	}

	total := headerLen + cl
	if MaxBodyBytes > 0 && cl > MaxBodyBytes {
		wrapper.Increment("qps.connaxis.fasthttp.reject.body_too_large")
		c.SetContext(unsupportedBodyLargeFast)
		return headerLen, headerLen
	}
	if len(data) >= total {
		return total, total
	}
	return 0, total
}

func (h *ConnaxisFastHTTPHandler) OnData(c connection.ProtoConn, in *[]byte) ([]byte, bool) {
	if h.dispatch == nil {
		wrapper.Errorf("fasthttp dispatcher not set")
		return nil, true
	}

	if m, ok := c.Context().(*unsupportedFastHTTP); ok {
		resp := fasthttp.AcquireResponse()
		resp.SetStatusCode(m.code)
		resp.SetBodyString(fasthttp.StatusMessage(m.code))
		out, err := EncodeResponse(resp)
		fasthttp.ReleaseResponse(resp)
		if err != nil {
			return nil, true
		}
		return out, true
	}

	req := fasthttp.AcquireRequest()
	if err := req.Read(bufio.NewReader(bytes.NewReader(*in))); err != nil {
		fasthttp.ReleaseRequest(req)
		wrapper.Increment("qps.connaxis.fasthttp.reject.parse_error")
		wrapper.Errorf("fasthttp parse error: %v", err)
		return nil, true
	}

	h.dispatch.DispatchFast(c, req)
	return nil, false
}

func (h *ConnaxisFastHTTPHandler) Stat(bool) {
}

func (h *ConnaxisFastHTTPHandler) SetDispatcher(dispatch FastDispatcher) {
	h.dispatch = dispatch
}

func ReleaseRequest(req *fasthttp.Request) {
	fasthttp.ReleaseRequest(req)
}

func ReleaseResponse(resp *fasthttp.Response) {
	fasthttp.ReleaseResponse(resp)
}

func EncodeResponse(resp *fasthttp.Response) ([]byte, error) {
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if err := resp.Write(bw); err != nil {
		return nil, err
	}
	if err := bw.Flush(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func parseContentLength(header []byte) (int, bool, bool, bool) {
	var cl int
	var hasCL bool
	var chunked bool
	var expectContinue bool
	start := 0

	for start < len(header) {
		end := bytes.IndexByte(header[start:], '\n')
		if end == -1 {
			break
		}

		line := header[start : start+end]
		start += end + 1
		if len(line) == 0 || (len(line) == 1 && line[0] == '\r') {
			break
		}
		if line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}

		if len(line) >= len(fastHeaderContentLength) && bytes.EqualFold(line[:len(fastHeaderContentLength)], fastHeaderContentLength) {
			if v, ok := parseContentLengthValue(line); ok {
				cl = v
				hasCL = true
			} else {
				return 0, false, false, false
			}
		}

		if len(line) >= len(fastHeaderTransferEncoding) && bytes.EqualFold(line[:len(fastHeaderTransferEncoding)], fastHeaderTransferEncoding) {
			lower := bytes.ToLower(line)
			if bytes.Contains(lower, fastHeaderChunked) {
				chunked = true
			}
		}

		if len(line) >= len(fastHeaderExpect) && bytes.EqualFold(line[:len(fastHeaderExpect)], fastHeaderExpect) {
			lower := bytes.ToLower(line)
			if bytes.Contains(lower, fastTokenContinue) {
				expectContinue = true
			}
		}
	}

	if !hasCL {
		return 0, chunked, expectContinue, true
	}
	return cl, chunked, expectContinue, true
}

func parseContentLengthValue(line []byte) (int, bool) {
	pos := bytes.IndexByte(line, ':')
	if pos == -1 {
		return 0, false
	}
	pos++
	for pos < len(line) && (line[pos] == ' ' || line[pos] == '\t') {
		pos++
	}
	if pos >= len(line) {
		return 0, false
	}
	n := 0
	for pos < len(line) {
		ch := line[pos]
		if ch < '0' || ch > '9' {
			break
		}
		n = n*10 + int(ch-'0')
		pos++
	}
	return n, true
}
