package websocket

import (
	"testing"

	"github.com/bernardhu/connaxis/pool"
)

type echoUserData struct{}

func (echoUserData) OnUserData(ctx *WsCtx) []byte {
	return ctx.Data
}

func buildMaskedTextFrame(payload []byte) []byte {
	mask := [4]byte{0x12, 0x34, 0x56, 0x78}
	hdr := 2 + 4
	if len(payload) > 125 {
		hdr = 2 + 2 + 4
	}

	out := make([]byte, hdr+len(payload))
	out[0] = 0x81
	if len(payload) <= 125 {
		out[1] = 0x80 | byte(len(payload))
		out[2] = mask[0]
		out[3] = mask[1]
		out[4] = mask[2]
		out[5] = mask[3]
		copy(out[6:], payload)
		cipher(out[6:], mask, 0)
		return out
	}

	out[1] = 0x80 | 126
	out[2] = byte(len(payload) >> 8)
	out[3] = byte(len(payload))
	out[4] = mask[0]
	out[5] = mask[1]
	out[6] = mask[2]
	out[7] = mask[3]
	copy(out[8:], payload)
	cipher(out[8:], mask, 0)
	return out
}

func BenchmarkProcessHandshake(b *testing.B) {
	pool.Setup(17)

	req := []byte("GET / HTTP/1.1\r\n" +
		"Host: example.com\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"\r\n")

	cli := &WsClient{}
	if n, _ := parseHandshake(cli, req); n <= 0 {
		b.Fatalf("handshake parse failed")
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		cli.Upgraded = false
		cli.nonce = []byte("dGhlIHNhbXBsZSBub25jZQ==")
		if _, close := processHandshake(cli); close {
			b.Fatalf("unexpected close")
		}
	}
}

func BenchmarkProcessFrameEchoText(b *testing.B) {
	pool.Setup(17)

	payload := make([]byte, 1024)
	for i := range payload {
		payload[i] = 'a'
	}
	frame := buildMaskedTextFrame(payload)

	cli := &WsClient{Upgraded: true, Cid: 1}
	h := echoUserData{}

	if n, _ := parseFrame(frame); n != len(frame) {
		b.Fatalf("frame parse mismatch n=%d len=%d", n, len(frame))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, close := processFrame(cli, frame, h); close {
			b.Fatalf("unexpected close")
		}
	}
}
