package websocket

import (
	"bytes"
	"testing"

	"github.com/bernardhu/connaxis/pool"
)

func TestParseHandshakeComplete(t *testing.T) {
	pool.Setup(17)

	req := []byte("GET / HTTP/1.1\r\n" +
		"Host: example.com\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"\r\n")

	cli := &WsClient{}
	n, expect := parseHandshake(cli, req)
	if n != len(req) || expect != len(req) {
		t.Fatalf("unexpected parse result n=%d expect=%d len=%d", n, expect, len(req))
	}
}

func TestParseHandshakeConnectionTokenList(t *testing.T) {
	pool.Setup(17)

	req := []byte("GET / HTTP/1.1\r\n" +
		"Host: example.com\r\n" +
		"Upgrade: WebSocket\r\n" +
		"Connection: keep-alive, Upgrade\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"\r\n")

	cli := &WsClient{}
	n, expect := parseHandshake(cli, req)
	if n != len(req) || expect != len(req) {
		t.Fatalf("unexpected parse result n=%d expect=%d len=%d", n, expect, len(req))
	}
}

func TestWriteAcceptKeyRFCExample(t *testing.T) {
	dst := make([]byte, 28)
	n := writeAcceptKey(dst, []byte("dGhlIHNhbXBsZSBub25jZQ=="))
	if n != 28 {
		t.Fatalf("unexpected len=%d", n)
	}
	if got, want := string(dst), "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="; got != want {
		t.Fatalf("unexpected accept got=%q want=%q", got, want)
	}
}

func TestParseHandshakePartial(t *testing.T) {
	pool.Setup(17)

	part1 := []byte("GET / HTTP/1.1\r\nHost: example.com\r\nUpgrade: websocket\r\n")
	cli := &WsClient{}
	n, expect := parseHandshake(cli, part1)
	if n != 0 {
		t.Fatalf("expected partial n=0, got %d", n)
	}
	if expect <= 0 {
		t.Fatalf("expected expect>0 for partial, got %d", expect)
	}
}

func TestParseHandshakePermessageDeflateNegotiation(t *testing.T) {
	pool.Setup(17)

	req := []byte("GET / HTTP/1.1\r\n" +
		"Host: example.com\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Extensions: permessage-deflate; client_no_context_takeover; client_max_window_bits\r\n" +
		"\r\n")

	cli := &WsClient{}
	n, expect := parseHandshake(cli, req)
	if n != len(req) || expect != len(req) {
		t.Fatalf("unexpected parse result n=%d expect=%d len=%d", n, expect, len(req))
	}
	if !cli.perMessageDeflate {
		t.Fatalf("expected permessage-deflate negotiation")
	}

	resp, closeConn := processHandshake(cli)
	if closeConn {
		t.Fatalf("unexpected close")
	}
	if !bytes.Contains(resp, extPermessageDeflateServerLine) {
		t.Fatalf("missing permessage-deflate response header: %q", string(resp))
	}
}

func TestParseHandshakePermessageDeflateOfferWithoutClientNoContext(t *testing.T) {
	pool.Setup(17)

	req := []byte("GET / HTTP/1.1\r\n" +
		"Host: example.com\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Extensions: permessage-deflate\r\n" +
		"\r\n")

	cli := &WsClient{}
	n, expect := parseHandshake(cli, req)
	if n != len(req) || expect != len(req) {
		t.Fatalf("unexpected parse result n=%d expect=%d len=%d", n, expect, len(req))
	}
	if cli.perMessageDeflate {
		t.Fatalf("unexpected permessage-deflate negotiation")
	}
}

func TestParseHandshakePlainHTTPWhenEnabled(t *testing.T) {
	pool.Setup(17)

	req := []byte("GET / HTTP/1.1\r\n" +
		"Host: example.com\r\n" +
		"User-Agent: test\r\n" +
		"Connection: close\r\n" +
		"\r\n")

	cli := &WsClient{AllowPlainHTTP: true}
	n, expect := parseHandshake(cli, req)
	if n != len(req) || expect != len(req) {
		t.Fatalf("unexpected parse result n=%d expect=%d len=%d", n, expect, len(req))
	}
	if !cli.PlainHTTP {
		t.Fatalf("expected plain HTTP fallback mode")
	}

	resp, closeConn := processHandshake(cli)
	if !closeConn {
		t.Fatalf("expected plain HTTP response to close connection")
	}
	if !bytes.Contains(resp, []byte("HTTP/1.1 200 OK")) {
		t.Fatalf("unexpected response: %q", string(resp))
	}
}

func TestParseHandshakePlainHTTP10WhenEnabled(t *testing.T) {
	pool.Setup(17)

	req := []byte("GET / HTTP/1.0\r\n\r\n")

	cli := &WsClient{AllowPlainHTTP: true}
	n, expect := parseHandshake(cli, req)
	if n != len(req) || expect != len(req) {
		t.Fatalf("unexpected parse result n=%d expect=%d len=%d", n, expect, len(req))
	}
	if !cli.PlainHTTP {
		t.Fatalf("expected plain HTTP fallback mode")
	}
}

func TestParseHandshakeWebSocketHTTP10Rejected(t *testing.T) {
	pool.Setup(17)

	req := []byte("GET / HTTP/1.0\r\n" +
		"Host: example.com\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"\r\n")

	cli := &WsClient{AllowPlainHTTP: true}
	n, expect := parseHandshake(cli, req)
	if n != -1 || expect != -1 {
		t.Fatalf("expected parse error for HTTP/1.0 websocket upgrade, got n=%d expect=%d", n, expect)
	}
}

func TestParseHandshakePlainHTTPWhenDisabled(t *testing.T) {
	pool.Setup(17)

	req := []byte("GET / HTTP/1.1\r\n" +
		"Host: example.com\r\n" +
		"Connection: close\r\n" +
		"\r\n")

	cli := &WsClient{}
	n, expect := parseHandshake(cli, req)
	if n != -1 || expect != -1 {
		t.Fatalf("expected strict WebSocket parse error, got n=%d expect=%d", n, expect)
	}
}
