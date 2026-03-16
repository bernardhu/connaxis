package evhandler

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type ResponseWriter struct {
	buf         *bytes.Buffer
	body        *bytes.Buffer
	code        int
	header      http.Header
	wroteHeader bool
}

func (r *ResponseWriter) Init() {
	r.code = 200
	r.header = make(http.Header)
	r.buf = bytes.NewBuffer(make([]byte, 1024))
	r.buf.Reset()
	r.body = bytes.NewBuffer(make([]byte, 1024))
	r.body.Reset()
}

func (r *ResponseWriter) Reset() {
	r.code = 200
	for k, _ := range r.header {
		r.header.Del(k)
	}
	r.buf.Reset()
	r.body.Reset()
	r.wroteHeader = false
}

func (r *ResponseWriter) Header() http.Header {
	return r.header
}

func (r *ResponseWriter) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.code = code
	r.wroteHeader = true
}

func (r *ResponseWriter) Write(buf []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(r.code)
	}
	return r.body.Write(buf)
}

func (r *ResponseWriter) Bytes() []byte {
	r.buf.Reset()
	status := fmt.Sprintf("HTTP/1.1 %d %s\r\n", r.code, http.StatusText(r.code))
	r.buf.WriteString(status)

	if r.header.Get("Content-Length") == "" {
		r.header.Set("Content-Length", strconv.Itoa(r.body.Len()))
	}
	for k, v := range r.header {
		r.buf.WriteString(k)
		r.buf.Write(cColon)

		if len(v) == 1 {
			r.buf.WriteString(v[0])
		} else {
			r.buf.WriteString(strings.Join(v, ", "))
		}

		r.buf.Write(cCRLF)
	}
	r.buf.Write(cCRLF)
	r.buf.Write(r.body.Bytes())
	return r.buf.Bytes()
}
