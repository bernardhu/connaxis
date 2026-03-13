package websocket

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"testing"

	"github.com/bernardhu/connaxis/pool"
	"github.com/bernardhu/connaxis/tuning"
)

type recordUserHandler struct {
	got [][]byte
}

func (h *recordUserHandler) OnUserData(ctx *WsCtx) []byte {
	h.got = append(h.got, append([]byte(nil), ctx.Data...))
	return ctx.Data
}

func buildClientFrame(opCode byte, fin bool, masked bool, payload []byte) []byte {
	return buildClientFrameWithRSV(opCode, fin, masked, 0, payload)
}

func buildClientFrameWithRSV(opCode byte, fin bool, masked bool, rsv byte, payload []byte) []byte {
	b0 := opCode & 0x0f
	if fin {
		b0 |= 0x80
	}
	b0 |= (rsv & 0x7) << 4

	mask := [4]byte{0x01, 0x02, 0x03, 0x04}
	maskBit := byte(0)
	if masked {
		maskBit = 0x80
	}

	header := []byte{b0, 0}
	switch {
	case len(payload) <= 125:
		header[1] = maskBit | byte(len(payload))
	case len(payload) <= 65535:
		header[1] = maskBit | 126
		ext := make([]byte, 2)
		binary.BigEndian.PutUint16(ext, uint16(len(payload)))
		header = append(header, ext...)
	default:
		header[1] = maskBit | 127
		ext := make([]byte, 8)
		binary.BigEndian.PutUint64(ext, uint64(len(payload)))
		header = append(header, ext...)
	}

	if masked {
		header = append(header, mask[:]...)
		maskedPayload := append([]byte(nil), payload...)
		cipher(maskedPayload, mask, 0)
		return append(header, maskedPayload...)
	}
	return append(header, payload...)
}

func compressPMDForTest(t *testing.T, payload []byte) []byte {
	t.Helper()

	var out bytes.Buffer
	w, err := flate.NewWriter(&out, flate.DefaultCompression)
	if err != nil {
		t.Fatalf("create flate writer: %v", err)
	}
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("flate write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("flate close: %v", err)
	}

	compressed := out.Bytes()
	if len(compressed) >= 4 && bytes.Equal(compressed[len(compressed)-4:], extPMDMessageTail) {
		compressed = compressed[:len(compressed)-4]
	}
	return append([]byte(nil), compressed...)
}

func TestProcessFrameTextEchoMasked(t *testing.T) {
	pool.Setup(17)

	cli := &WsClient{Upgraded: true, Cid: 1}
	uh := &recordUserHandler{}

	frame := buildClientFrame(0x1, true, true, []byte("hello"))
	out, closed := processFrame(cli, frame, uh)
	if closed {
		t.Fatalf("unexpected close")
	}
	if len(uh.got) != 1 || string(uh.got[0]) != "hello" {
		t.Fatalf("unexpected handler got=%q", uh.got)
	}
	if len(out) == 0 {
		t.Fatalf("expected outgoing frame")
	}

	if len(out) < 2 {
		t.Fatalf("invalid outgoing frame")
	}
	if out[0] != 0x81 {
		t.Fatalf("unexpected opcode/fin: 0x%x", out[0])
	}
	if out[1]&0x80 != 0 {
		t.Fatalf("server frame must be unmasked")
	}
	payloadLen := int(out[1] & 0x7f)
	if payloadLen != 5 || !bytes.Equal(out[2:], []byte("hello")) {
		t.Fatalf("unexpected outgoing payload: %q", string(out[2:]))
	}
}

func TestProcessFrameRejectUnmaskedWhenRequired(t *testing.T) {
	pool.Setup(17)

	old := tuning.WSRequireClientMask
	tuning.WSRequireClientMask = true
	t.Cleanup(func() { tuning.WSRequireClientMask = old })

	cli := &WsClient{Upgraded: true, Cid: 1}
	uh := &recordUserHandler{}

	frame := buildClientFrame(0x1, true, false, []byte("hello"))
	out, closed := processFrame(cli, frame, uh)
	if !closed {
		t.Fatalf("expected close")
	}
	if out[0] != 0x88 {
		t.Fatalf("expected close frame, got 0x%x", out[0])
	}
	if out[1] != 2 {
		t.Fatalf("expected close payload len=2, got %d", out[1])
	}
	code := int(out[2])<<8 | int(out[3])
	if code != 1002 {
		t.Fatalf("unexpected close code=%d", code)
	}
}

func TestProcessFrameCompressedTextEcho(t *testing.T) {
	pool.Setup(17)

	cli := &WsClient{Upgraded: true, Cid: 1, perMessageDeflate: true}
	uh := &recordUserHandler{}

	plain := []byte("hello compressed")
	payload := compressPMDForTest(t, plain)
	frame := buildClientFrameWithRSV(0x1, true, true, 4, payload)

	out, closed := processFrame(cli, frame, uh)
	if closed {
		t.Fatalf("unexpected close")
	}
	if len(uh.got) != 1 || !bytes.Equal(uh.got[0], plain) {
		t.Fatalf("unexpected handler payload got=%q", uh.got)
	}
	if len(out) < 2 || out[0] != 0x81 {
		t.Fatalf("unexpected output frame: %x", out)
	}
	if !bytes.Equal(out[2:], plain) {
		t.Fatalf("unexpected output payload: %q", out[2:])
	}
}

func TestProcessFrameRejectCompressedWithoutNegotiation(t *testing.T) {
	pool.Setup(17)

	cli := &WsClient{Upgraded: true, Cid: 1}
	uh := &recordUserHandler{}

	plain := []byte("hello")
	payload := compressPMDForTest(t, plain)
	frame := buildClientFrameWithRSV(0x1, true, true, 4, payload)

	out, closed := processFrame(cli, frame, uh)
	if !closed {
		t.Fatalf("expected close")
	}
	if len(out) != 4 || out[0] != 0x88 || out[1] != 0x02 {
		t.Fatalf("expected close frame with status code, got=%x", out)
	}
	code := int(out[2])<<8 | int(out[3])
	if code != 1002 {
		t.Fatalf("expected protocol error 1002, got=%d", code)
	}
}

func TestProcessFramePingPong(t *testing.T) {
	pool.Setup(17)

	cli := &WsClient{Upgraded: true, Cid: 1}
	uh := &recordUserHandler{}

	frame := buildClientFrame(0x9, true, true, []byte("x"))
	out, closed := processFrame(cli, frame, uh)
	if closed {
		t.Fatalf("unexpected close")
	}
	if len(uh.got) != 0 {
		t.Fatalf("handler should not be called for ping")
	}
	if out[0] != 0x8a {
		t.Fatalf("expected pong opcode, got 0x%x", out[0])
	}
	if out[1] != 1 || out[2] != 'x' {
		t.Fatalf("unexpected pong payload: %x", out)
	}
}

func TestProcessFrameFragmentationReassembly(t *testing.T) {
	pool.Setup(17)

	cli := &WsClient{Upgraded: true, Cid: 1}
	uh := &recordUserHandler{}

	f1 := buildClientFrame(0x1, false, true, []byte("hel"))
	f2 := buildClientFrame(0x0, false, true, []byte("lo "))
	f3 := buildClientFrame(0x0, true, true, []byte("world"))

	if out, closed := processFrame(cli, f1, uh); closed || len(out) > 0 {
		t.Fatalf("unexpected close on fragment 1")
	}
	if out, closed := processFrame(cli, f2, uh); closed || len(out) > 0 {
		t.Fatalf("unexpected close on fragment 2")
	}
	out, closed := processFrame(cli, f3, uh)
	if closed {
		t.Fatalf("unexpected close on final fragment")
	}
	if len(uh.got) != 1 || string(uh.got[0]) != "hello world" {
		t.Fatalf("unexpected reassembled=%q", uh.got)
	}
	if len(out) == 0 {
		t.Fatalf("expected one echo frame")
	}
}

func TestProcessFrameFragmentedEmptyMessageEcho(t *testing.T) {
	pool.Setup(17)

	cli := &WsClient{Upgraded: true, Cid: 1}
	uh := &recordUserHandler{}

	f1 := buildClientFrame(0x1, false, true, []byte{})
	f2 := buildClientFrame(0x0, false, true, []byte{})
	f3 := buildClientFrame(0x0, true, true, []byte{})

	if out, closed := processFrame(cli, f1, uh); closed || len(out) > 0 {
		t.Fatalf("unexpected close on fragment 1")
	}
	if out, closed := processFrame(cli, f2, uh); closed || len(out) > 0 {
		t.Fatalf("unexpected close on fragment 2")
	}
	out, closed := processFrame(cli, f3, uh)
	if closed {
		t.Fatalf("unexpected close on final fragment")
	}
	if len(uh.got) != 1 || len(uh.got[0]) != 0 {
		t.Fatalf("expected one empty payload callback, got=%v", uh.got)
	}
	if len(out) != 2 || out[0] != 0x81 || out[1] != 0x00 {
		t.Fatalf("expected empty text frame echo, got=%x", out)
	}
}

func TestProcessFrameFragmentedTextSplitRuneEcho(t *testing.T) {
	pool.Setup(17)

	cli := &WsClient{Upgraded: true, Cid: 1}
	uh := &recordUserHandler{}

	f1 := buildClientFrame(0x1, false, true, []byte{0xce})
	f2 := buildClientFrame(0x0, true, true, []byte{0xba})

	if out, closed := processFrame(cli, f1, uh); closed || len(out) > 0 {
		t.Fatalf("unexpected close on first split-rune fragment")
	}
	out, closed := processFrame(cli, f2, uh)
	if closed {
		t.Fatalf("unexpected close on final split-rune fragment")
	}
	if len(uh.got) != 1 || string(uh.got[0]) != "κ" {
		t.Fatalf("unexpected reassembled=%q", uh.got)
	}
	if len(out) != 4 || out[0] != 0x81 || out[1] != 0x02 || !bytes.Equal(out[2:], []byte{0xce, 0xba}) {
		t.Fatalf("unexpected outgoing payload: %x", out)
	}
}

func TestProcessFrameFragmentedTextInvalidUTF8FailFast(t *testing.T) {
	pool.Setup(17)

	cli := &WsClient{Upgraded: true, Cid: 1}
	uh := &recordUserHandler{}

	f1 := buildClientFrame(0x1, false, true, []byte{0xf4})
	f2 := buildClientFrame(0x0, false, true, []byte{0x90})

	if out, closed := processFrame(cli, f1, uh); closed || len(out) > 0 {
		t.Fatalf("unexpected close on fragment 1")
	}
	out, closed := processFrame(cli, f2, uh)
	if !closed {
		t.Fatalf("expected close on invalid second fragment")
	}
	if len(out) != 4 || out[0] != 0x88 || out[1] != 0x02 {
		t.Fatalf("expected close frame with code, got=%x", out)
	}
	code := int(out[2])<<8 | int(out[3])
	if code != 1007 {
		t.Fatalf("expected invalid payload 1007, got=%d", code)
	}
}

func TestProcessFrameRejectContinuationWithoutStart(t *testing.T) {
	pool.Setup(17)

	cli := &WsClient{Upgraded: true, Cid: 1}
	uh := &recordUserHandler{}

	f := buildClientFrame(0x0, true, true, []byte("bad"))
	out, closed := processFrame(cli, f, uh)
	if !closed {
		t.Fatalf("expected close")
	}
	if len(out) == 0 || out[0] != 0x88 {
		t.Fatalf("expected close frame")
	}
}

func TestParseFramePartial(t *testing.T) {
	pool.Setup(17)

	frame := buildClientFrame(0x1, true, true, bytes.Repeat([]byte("a"), 200))
	n, expect := parseFrame(frame[:3])
	if n != 0 || expect <= len(frame[:3]) {
		t.Fatalf("expected partial parse n=0 expect>len, got n=%d expect=%d", n, expect)
	}
}

func TestProcessFrameCloseInvalidCodeProtocolError(t *testing.T) {
	pool.Setup(17)

	cli := &WsClient{Upgraded: true, Cid: 1}
	uh := &recordUserHandler{}

	payload := []byte{0x03, 0xEC} // 1004 invalid to send on wire
	frame := buildClientFrame(0x8, true, true, payload)
	out, closed := processFrame(cli, frame, uh)
	if !closed {
		t.Fatalf("expected close")
	}
	if len(out) != 4 || out[0] != 0x88 || out[1] != 0x02 {
		t.Fatalf("expected close frame with status code, got=%x", out)
	}
	code := int(out[2])<<8 | int(out[3])
	if code != 1002 {
		t.Fatalf("expected protocol error 1002, got=%d", code)
	}
}

func TestParseFrameAllow16MBByDefault(t *testing.T) {
	pool.Setup(17)

	payload := bytes.Repeat([]byte("x"), 16*1024*1024)
	frame := buildClientFrame(0x2, true, true, payload)
	n, expect := parseFrame(frame)
	if n != expect || n != len(frame) {
		t.Fatalf("expected full frame parse for 16MB payload, got n=%d expect=%d len=%d", n, expect, len(frame))
	}
}
