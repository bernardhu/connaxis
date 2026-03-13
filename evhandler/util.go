package evhandler

import (
	"bytes"
	"io"
	"net/http"

	"github.com/evanphx/wildcat"
)

var (
	cColon = []byte(": ")
	cCRLF  = []byte("\r\n")

	headerTransferEncoding = []byte("Transfer-Encoding")
	headerExpect           = []byte("Expect")
	tokenChunked           = []byte("chunked")
	tokenContinue          = []byte("100-continue")
)

type unsupportedHTTP struct {
	code int
}

var (
	unsupportedChunked   = &unsupportedHTTP{code: http.StatusNotImplemented}
	unsupportedExpect    = &unsupportedHTTP{code: http.StatusExpectationFailed}
	unsupportedHdrLarge  = &unsupportedHTTP{code: http.StatusRequestHeaderFieldsTooLarge}
	unsupportedBodyLarge = &unsupportedHTTP{code: http.StatusRequestEntityTooLarge}
)

// MaxHeaderBytes limits the maximum HTTP header size for evhandler (0 = unlimited).
var MaxHeaderBytes = 64 * 1024

// MaxBodyBytes limits the maximum HTTP body size for evhandler (0 = unlimited).
var MaxBodyBytes = 8 * 1024 * 1024

type sizedBodyReader struct {
	size int
	rest []byte
}

func (br *sizedBodyReader) Read(buf []byte) (int, error) {
	if br.size == 0 || br.rest == nil {
		return 0, io.EOF
	}

	if len(buf) < len(br.rest) {
		copy(buf, br.rest[:len(buf)])

		br.rest = br.rest[len(buf):]
		br.size -= len(buf)
		return len(buf), nil
	} else {
		l := len(br.rest)
		copy(buf, br.rest)

		br.rest = nil
		br.size -= l
		return l, nil
	}
}

func (br *sizedBodyReader) Close() error {
	return nil
}

func ConvertHeader(hp *wildcat.HTTPParser) http.Header {
	header := make(http.Header)

	for _, h := range hp.Headers {
		if h.Name == nil {
			continue
		}

		header.Add(string(h.Name), string(h.Value))
		//header[string(h.Name)] = append(header[string(h.Name)], string(h.Value))
	}

	return header
}

func unsupportedHeaderMarker(hp *wildcat.HTTPParser) *unsupportedHTTP {
	if headerHasToken(hp.FindAllHeaders(headerTransferEncoding), tokenChunked) {
		return unsupportedChunked
	}
	if headerHasExact(hp.FindAllHeaders(headerExpect), tokenContinue) {
		return unsupportedExpect
	}
	return nil
}

func headerTooLarge(in []byte) (bool, int) {
	if MaxHeaderBytes <= 0 {
		return false, 0
	}
	idx := bytes.Index(in, []byte("\r\n\r\n"))
	if idx >= 0 {
		headerLen := idx + 4
		if headerLen > MaxHeaderBytes {
			return true, headerLen
		}
		return false, headerLen
	}
	if len(in) > MaxHeaderBytes {
		return true, len(in)
	}
	return false, 0
}

func httpErrorResponse(code int) []byte {
	text := StatusText(code)
	if text == "" {
		code = http.StatusBadRequest
		text = StatusText(code)
	}
	body := text + "\n"
	return []byte(
		"HTTP/1.1 " + itoa(code) + " " + text + "\r\n" +
			"Content-Type: text/plain\r\n" +
			"Connection: close\r\n" +
			"Content-Length: " + itoa(len(body)) + "\r\n\r\n" +
			body,
	)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

func headerHasToken(values [][]byte, token []byte) bool {
	for _, v := range values {
		if len(v) == 0 {
			continue
		}
		start := 0
		for i := 0; i <= len(v); i++ {
			if i == len(v) || v[i] == ',' {
				part := trimSpace(v[start:i])
				if equalFold(part, token) {
					return true
				}
				start = i + 1
			}
		}
	}
	return false
}

func headerHasExact(values [][]byte, token []byte) bool {
	for _, v := range values {
		part := trimSpace(v)
		if equalFold(part, token) {
			return true
		}
	}
	return false
}

func trimSpace(in []byte) []byte {
	start := 0
	for start < len(in) {
		if in[start] != ' ' && in[start] != '\t' {
			break
		}
		start++
	}
	end := len(in)
	for end > start {
		if in[end-1] != ' ' && in[end-1] != '\t' {
			break
		}
		end--
	}
	return in[start:end]
}

func equalFold(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ra := a[i]
		rb := b[i]
		if ra >= 'A' && ra <= 'Z' {
			ra = ra + ('a' - 'A')
		}
		if rb >= 'A' && rb <= 'Z' {
			rb = rb + ('a' - 'A')
		}
		if ra != rb {
			return false
		}
	}
	return true
}
