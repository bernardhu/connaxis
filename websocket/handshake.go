package websocket

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"net/http"

	"github.com/bernardhu/connaxis/wrapper"
)

var httpGet = []byte(http.MethodGet)
var upgrade = "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: "
var plainHTTPResponse = []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 3\r\nConnection: close\r\n\r\nok\n")

func writeAcceptKey(dst []byte, nonce []byte) int {
	const magic = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	if len(dst) < 28 {
		return 0
	}
	if len(nonce) != 24 {
		return 0
	}

	var p [24 + len(magic)]byte
	copy(p[:24], nonce)
	copy(p[24:], magic)

	sum := sha1.Sum(p[:])
	base64.StdEncoding.Encode(dst[:28], sum[:])
	return 28
}

// 0 partial, -1 err
func parseHandshake(cli *WsClient, in []byte) (int, int) {
	if !bytes.Contains(in, []byte("\r\n\r\n")) {
		return 0, len(in) + 1
	}
	cli.perMessageDeflate = false
	cli.PlainHTTP = false

	//wrapper.Debugf("parseHandshake in:%s", string(in))
	input := in
	line, pos := getLine(input)
	//wrapper.Debugf("getLine first line:%s", string(line))
	if pos == -1 {
		return 0, len(in) + 1
	}

	reqLine := parseRequestLine(line, cli.AllowPlainHTTP)
	//wrapper.Debugf("parseRequestLine first line:%v", reqLine)
	if reqLine == nil {
		return -1, -1
	}

	size := pos + 1
	state := 0
	for {
		line = input[size:]
		//wrapper.Debugf("left line:%s...%d,%v", string(line), len(line), line)
		if len(line) == 0 {
			break //no more
		}

		line, pos = getLine(line)
		if pos == -1 {
			return 0, -1
		}
		size = size + pos + 1

		if len(line) == 0 {
			break //no more
		}

		key, val := parseHeaderLine(line)
		//wrapper.Debugf("parsed key:%s, val:%s,", string(key), string(val))
		if key == nil && val == nil {
			wrapper.Errorf("parse line fail:%s", string(line))
			return -1, -1
		}

		if equal(key, headerHost) {
			state = state | headerSeenHost
			//wrapper.Debugf("see host")
		}

		if equal(key, headerUpgrade) {
			if !equalFoldASCII(val, specHeaderValueWsLower) {
				return -1, -1
			}
			state = state | headerSeenUpgrade

			//wrapper.Debugf("see upgrade")
		}

		if equal(key, headerConnection) {
			if !containsTokenFoldASCII(val, specHeaderValueUpgradeLower) && !cli.AllowPlainHTTP {
				return -1, -1
			}
			if containsTokenFoldASCII(val, specHeaderValueUpgradeLower) {
				state = state | headerSeenConnection
			}
			//wrapper.Debugf("see connection")
		}

		if equal(key, headerSecVersion) {
			if !equal(val, specHeaderValueSecVersion) {
				return -1, -1
			}
			state = state | headerSeenSecVersion
			//wrapper.Debugf("see sec version")
		}

		if equal(key, headerSecKey) {
			if len(val) != 24 {
				return -1, -1
			}
			state = state | headerSeenSecKey
			cli.nonce = val
			//wrapper.Debugf("see key")
		}

		if equal(key, headerSecExtensions) {
			if !cli.perMessageDeflate && negotiatePermessageDeflate(val) {
				cli.perMessageDeflate = true
			}
		}

		if equal(key, headerSecProtocol) {
			//todo
		}

	}

	if state == headerSeenAll {
		// RFC 6455 requires HTTP/1.1 for WebSocket upgrade.
		if reqLine.minor < 1 {
			return -1, -1
		}
		wrapper.Debugf("all parsed size:%d,", size)
		return size, size
	}
	if cli.AllowPlainHTTP &&
		(state&(headerSeenUpgrade|headerSeenConnection|headerSeenSecVersion|headerSeenSecKey)) == 0 {
		if reqLine.minor == 0 || (state&headerSeenHost) != 0 {
			cli.PlainHTTP = true
			return size, size
		}
	}
	return -1, -1
}

func processHandshake(cli *WsClient) ([]byte, bool) {
	if cli.PlainHTTP {
		cli.PlainHTTP = false
		cli.nonce = nil
		return plainHTTPResponse, true
	}
	cli.Upgraded = true

	size := len(upgrade) + 28 + 2 + 2
	if cli.perMessageDeflate {
		size += len(extPermessageDeflateServerLine)
	}

	buf := make([]byte, size)
	pos := copy(buf, stringToBytes(upgrade))
	pos += writeAcceptKey(buf[pos:], cli.nonce)
	pos += copy(buf[pos:], stringToBytes("\r\n"))
	if cli.perMessageDeflate {
		pos += copy(buf[pos:], extPermessageDeflateServerLine)
	}
	pos += copy(buf[pos:], stringToBytes("\r\n"))
	cli.nonce = nil

	return buf[:pos], false
}

func getLine(in []byte) ([]byte, int) {
	pos := bytes.IndexByte(in, '\n')
	if pos >= 0 {
		if pos > 0 && in[pos-1] == '\r' {
			//wrapper.Debugf("getLine line:%s pos:%d", string(in[:pos-1]), pos)
			return in[:pos-1], pos
		}
		//wrapper.Debugf("getLine line:%s pos:%d", string(in[:pos]), pos)
		return in[:pos], pos
	}

	return nil, -1
}

// first line should in the format of: GET uri version
func parseRequestLine(in []byte, allowHTTP10 bool) *httpRequestLine {
	pos1 := bytes.IndexByte(in, ' ')
	if pos1 <= 0 || pos1+1 >= len(in) {
		return nil
	}
	pos2 := bytes.IndexByte(in[pos1+1:], ' ')
	if pos2 <= 0 {
		return nil
	}

	method := in[:pos1]
	uri := in[pos1+1 : pos2+pos1+1]
	proto := in[pos1+pos2+2:]
	if len(proto) < 8 { // HTTP/x.y
		return nil
	}

	pos := bytes.IndexByte(proto, '.')
	if pos <= 0 || pos+1 >= len(proto) {
		return nil
	}
	if proto[pos-1] < '0' || proto[pos-1] > '9' || proto[pos+1] < '0' || proto[pos+1] > '9' {
		return nil
	}
	major := int(proto[pos-1] - '0')
	minor := int(proto[pos+1] - '0')

	if major != 1 || minor < 0 {
		return nil
	}
	if !allowHTTP10 && minor < 1 {
		return nil
	}

	if !equal(method, httpGet) {
		return nil
	}

	return &httpRequestLine{
		method: method,
		uri:    uri,
		major:  major,
		minor:  minor,
	}
}

func parseHeaderLine(in []byte) ([]byte, []byte) {
	pos := bytes.IndexByte(in, ':')

	if pos == -1 {
		return nil, nil
	}

	k := trim(in[:pos])
	canonicalizeBytes(k)
	v := trim(in[pos+1:])

	return k, v
}

func equal(a []byte, b []byte) bool {
	if len(a) != len(b) {
		return false
	}

	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func equalFoldASCII(a []byte, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ac := a[i]
		bc := b[i]
		if 'A' <= ac && ac <= 'Z' {
			ac = ac + ('a' - 'A')
		}
		if 'A' <= bc && bc <= 'Z' {
			bc = bc + ('a' - 'A')
		}
		if ac != bc {
			return false
		}
	}
	return true
}

func containsTokenFoldASCII(headerVal []byte, token []byte) bool {
	for len(headerVal) > 0 {
		seg := headerVal
		if idx := bytes.IndexByte(headerVal, ','); idx >= 0 {
			seg = headerVal[:idx]
			headerVal = headerVal[idx+1:]
		} else {
			headerVal = nil
		}
		seg = trim(seg)
		if len(seg) == 0 {
			continue
		}
		if equalFoldASCII(seg, token) {
			return true
		}
	}
	return false
}

func trim(in []byte) []byte {
	var i, j int
	for i = 0; i < len(in) && (in[i] == ' ' || in[i] == '\t'); {
		i = i + 1
	}
	for j = len(in); j > i && (in[j-1] == ' ' || in[j-1] == '\t'); {
		j = j - 1
	}
	return in[i:j]
}
