package common

import (
	"bytes"
)

var httpResponse = []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: keep-alive\r\n\r\nok")

// ParseHTTPRequest returns true if a full request header is present.
// It does not parse method/path for fairness; only detects end of headers.
func ParseHTTPRequest(buf []byte) bool {
	return bytes.Contains(buf, []byte("\r\n\r\n"))
}

func HTTPResponse() []byte {
	return httpResponse
}
