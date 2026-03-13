package websocket

import (
	"encoding/binary"
	"errors"
	"unicode/utf8"

	"github.com/bernardhu/connaxis/pool"
	"github.com/bernardhu/connaxis/tuning"
	"github.com/bernardhu/connaxis/wrapper"
)

var wsCtxPool = pool.NewObjectPool(func() *WsCtx {
	return new(WsCtx)
})

const (
	MessageNone = iota
	MessageText
	MessageBinary
)

type Header struct {
	Fin    bool
	Rsv    byte
	OpCode byte
	Masked bool
	Mask   [4]byte
	Length int64
}

// 0 partial, -1 err
func parseFrame(in []byte) (int, int) {
	if len(in) < WSHeaderMinSize {
		return 0, WSHeaderMinSize
	}

	headerSize := calcHeaderSize(in[1])
	if len(in) < headerSize {
		return 0, headerSize
	}

	length := int64(in[1] & 0x7f)
	switch in[1] & 0x7f {
	case 126:
		length = int64(binary.BigEndian.Uint16(in[2:4]))
	case 127:
		length = int64(binary.BigEndian.Uint64(in[2:10]))
		if length < 0 {
			return -1, -1
		}
	}

	if tuning.WSMaxFramePayloadBytes > 0 && length > int64(tuning.WSMaxFramePayloadBytes) {
		return -1, -1
	}

	total := int(length) + headerSize
	if total > len(in) {
		//wrapper.Debugf("parseFrame total:%d header:%d payload:%d has len:%d", total, headerSize, int(length), len(in))
		return 0, total
	}
	return total, total
}

func calcHeaderSize(b byte) int {
	size := 2
	if b&0x80 != 0 { //mask
		size = size + 4
	}

	switch b & 0x7f {
	case 126:
		size = size + 2
	case 127:
		size = size + 8
	}

	return size
}

func scanUTF8Prefix(in []byte) (checked int, complete bool, ok bool) {
	for checked < len(in) {
		if in[checked] < utf8.RuneSelf {
			checked++
			continue
		}

		_, size := utf8.DecodeRune(in[checked:])
		if size == 1 {
			if utf8.FullRune(in[checked:]) {
				return checked, false, false
			}
			return checked, false, true
		}
		checked += size
	}

	return checked, true, true
}

func validateBufferedText(buf *clientBuf, final bool) bool {
	if buf == nil || buf.writeOffset == 0 {
		return true
	}

	segment := buf.buf[buf.checkOffset:buf.writeOffset]
	checked, complete, ok := scanUTF8Prefix(segment)
	buf.checkOffset += checked
	if !ok {
		return false
	}
	return complete || !final
}

func processFrame(cli *WsClient, in []byte, h UserDataHandler) (out []byte, close bool) {
	headerSize := calcHeaderSize(in[1])
	payload := in[headerSize:]
	fragmentedText := cli.firstop == MessageText && !cli.fragmentCompressed

	if tuning.WSRequireClientMask && (in[1]&0x80) == 0 {
		wrapper.Debugf("client frame is not masked id：%d", cli.Cid)
		return buildCloseWithCode(1002), true
	}

	if (in[1]&0x80) != 0 && len(payload) > 0 {
		mask := [4]byte{in[headerSize-4], in[headerSize-3], in[headerSize-2], in[headerSize-1]}
		//wrapper.Debugf("before cipher mask: %v payload:%v,", mask, payload)
		cipher(payload, mask, 0)
		//wrapper.Debugf("after cipher mask: %x payload:%x,", mask, payload)
	}

	fin := (in[0] & 0x80) != 0
	rsv1 := (in[0] & 0x40) != 0
	rsv23 := in[0] & 0x30
	opCode := in[0] & 0x0f
	//wrapper.Debugf("processFrame opcode:%d", opCode)
	if rsv23 != 0 {
		wrapper.Debugf("RSV2/RSV3 is set id：%d", cli.Cid)
		return buildCloseWithCode(1002), true
	}

	if opCode >= 3 { //control
		if rsv1 {
			wrapper.Debugf("control frame has RSV1 set id：%d", cli.Cid)
			return buildCloseWithCode(1002), true
		}
		if len(payload) > 125 || !fin { //control frame paylaod < 125 byte
			wrapper.Debugf("invalid control frame length or FIN id：%d", cli.Cid)
			return buildCloseWithCode(1002), true
		}

		switch opCode {
		case 0x8: //close
			return buildClose(payload), true
		case 0x9: //ping
			return buildPong(payload), false
		case 0xa: //pong
			return nil, false
		case 0x3, 0x4, 0x5, 0x6, 0x7, 0xb, 0xc, 0xd, 0xe, 0xf: //reserved control byte
			wrapper.Debugf("reserved control opcode id：%d", cli.Cid)
			return buildCloseWithCode(1002), true
		}
	}

	compressed := false
	if opCode == 0 {
		if rsv1 {
			wrapper.Debugf("continuation frame has RSV1 set id：%d", cli.Cid)
			return buildCloseWithCode(1002), true
		}
		compressed = cli.fragmentCompressed
	} else {
		if rsv1 {
			if !cli.perMessageDeflate || (opCode != MessageText && opCode != MessageBinary) {
				wrapper.Debugf("RSV1 set without negotiated PMD or invalid opcode id：%d", cli.Cid)
				return buildCloseWithCode(1002), true
			}
			compressed = true
		}
	}

	// fragment
	// 1. unfragment: opcode !=0 && fin= true
	// 2. fragment:
	// 2.1 first frame fin == false && opcode!=0
	// 2.2 con't frame fin == false && opcode = 0
	// 2.3 final frame fin == true && opcode = 0
	fragmentCheckPass := false
	if (cli.firstop == 0 && opCode != 0) || //case 1/2.1
		(cli.firstop != 0 && opCode == 0) { //case 2.2/2.3
		fragmentCheckPass = true
	}

	if !fragmentCheckPass {
		wrapper.Debugf("invalid fragmentation sequence id：%d", cli.Cid)
		return buildCloseWithCode(1002), true
	}

	if opCode != 0 && !fin {
		cli.firstop = opCode
		cli.fragmentCompressed = compressed
	}

	if cli.firstop != 0 {
		if cli.buf == nil {
			cli.buf = new(clientBuf)
		}

		if !fin {
			if tuning.WSMaxMessageBytes > 0 && cli.buf.size()+len(payload) > tuning.WSMaxMessageBytes {
				cli.resetMessageState()
				return buildCloseWithCode(1009), true // message too big
			}

			if !cli.buf.write(payload) {
				cli.resetMessageState()
				return buildCloseWithCode(1009), true // message too big
			}
			if cli.firstop == MessageText && !cli.fragmentCompressed && !validateBufferedText(cli.buf, false) {
				cli.resetMessageState()
				wrapper.Debugf("processFrame incremental utf-8 fail id：%d", cli.Cid)
				return buildCloseWithCode(1007), true
			}
			//wrapper.Debugf("cli.buf len:%d", cli.buf.writeOffset)
			return nil, false
		}

		if tuning.WSMaxMessageBytes > 0 && cli.buf.size()+len(payload) > tuning.WSMaxMessageBytes {
			cli.resetMessageState()
			return buildCloseWithCode(1009), true // message too big
		}
		if !cli.buf.write(payload) {
			cli.resetMessageState()
			return buildCloseWithCode(1009), true // message too big
		}
		if cli.firstop == MessageText && !cli.fragmentCompressed {
			if !validateBufferedText(cli.buf, true) {
				cli.resetMessageState()
				wrapper.Debugf("processFrame incremental utf-8 fail id：%d", cli.Cid)
				return buildCloseWithCode(1007), true
			}
		}
		payload = cli.buf.bytes()
		opCode = cli.firstop
		compressed = cli.fragmentCompressed
		cli.firstop = 0
		cli.fragmentCompressed = false
		//wrapper.Debugf("recv all cli.buf len:%d", len(payload))
	}

	if opCode != 0 && fin {
		cli.fragmentCompressed = false
	}

	if compressed {
		decoded, err := inflatePermessageDeflate(cli, payload, tuning.WSMaxMessageBytes)
		if err != nil {
			cli.resetMessageState()
			if errors.Is(err, errPMDMessageTooLarge) {
				return buildCloseWithCode(1009), true
			}
			wrapper.Debugf("permessage-deflate decode failed id：%d", cli.Cid)
			return buildCloseWithCode(1002), true
		}
		payload = decoded
	}

	if tuning.WSMaxMessageBytes > 0 && len(payload) > tuning.WSMaxMessageBytes {
		cli.resetMessageState()
		return buildCloseWithCode(1009), true // message too big
	}

	//check utf-8
	if opCode == MessageText && !fragmentedText && !utf8.Valid(payload) {
		cli.resetMessageState()
		wrapper.Debugf("processFrame check utf-8 fail, payload: %v", payload)
		return buildCloseWithCode(1007), true
	}

	out = processUserdata(cli.Cid, payload, opCode, h)
	cli.resetBuf()
	//wrapper.Debugf("processFrame data:%v... res:%v", payload, res)
	//wrapper.Debugf("processFrame datalen %d reslen:%d", len(payload), len(res))
	return out, false
}

func buildCloseWithCode(code int) []byte {
	var payload [2]byte
	payload[0] = byte(code >> 8)
	payload[1] = byte(code)
	return buildClose(payload[:])
}

func buildClose(in []byte) []byte {
	wrapper.Debugf("recv close payload %x", in)
	payload := in

	if len(payload) > 0 {
		if len(payload) < 2 { // at least 2 byte code
			return buildCloseWithCode(1002)
		} else {
			code := int(payload[0])<<8 | int(payload[1])
			if !isValidCloseCode(code) {
				wrapper.Debugf("invalid close code %d", code)
				return buildCloseWithCode(1002)
			} else if len(payload) > 2 && !utf8.Valid(payload[2:]) {
				payload = nil
			}
		}
	}

	return buildFrame(&Header{
		Fin:    true,
		Rsv:    0,
		OpCode: 0x8,
		Masked: false,
		Length: int64(len(payload)),
	}, payload)
}

func isValidCloseCode(code int) bool {
	switch {
	case code >= 3000 && code <= 4999:
		return true
	case code >= 1000 && code <= 1014 && code != 1004 && code != 1005 && code != 1006:
		return true
	default:
		return false
	}
}

func calcPktSize(h *Header) (int64, int64) {
	size := int64(2)
	if h.Masked {
		size = size + 4
	}

	if h.Length > 125 && h.Length <= 65535 {
		size = size + 2
	} else if h.Length > 65535 {
		size = size + 8
	}

	return size + h.Length, size
}

func writeHeader(h *Header, hsize int64, buf []byte) {
	b0 := h.OpCode & 0x0f
	if h.Fin {
		b0 |= 0x80
	}
	b0 |= (h.Rsv & 0x7) << 4
	buf[0] = b0

	if h.Length <= 125 {
		buf[1] = byte(h.Length)
	} else if h.Length <= 65535 {
		buf[1] = 126
		binary.BigEndian.PutUint16(buf[2:4], uint16(h.Length))
	} else {
		buf[1] = 127
		binary.BigEndian.PutUint64(buf[2:10], uint64(h.Length))
	}

	if h.Masked {
		buf[1] = buf[1] | 0x80
		buf[hsize-4] = h.Mask[0]
		buf[hsize-3] = h.Mask[1]
		buf[hsize-2] = h.Mask[2]
		buf[hsize-1] = h.Mask[3]
	}
}

func buildPong(in []byte) []byte { //send pong
	wrapper.Debugf("recv ping")
	return buildFrame(&Header{
		Fin:    true,
		Rsv:    0,
		OpCode: 0xa,
		Masked: false,
		Length: int64(len(in)),
	}, in)
}

func processUserdata(id uint64, in []byte, opCode byte, handler UserDataHandler) []byte {
	ctx := wsCtxPool.Get()
	ctx.Cid = id
	ctx.Data = in

	out := handler.OnUserData(ctx)
	ctx.Cid = 0
	ctx.Data = nil
	ctx.CallBack = 0
	wsCtxPool.Put(ctx)

	if out == nil || (opCode != MessageText && opCode != MessageBinary) {
		return nil
	}

	return buildFrame(&Header{
		Fin:    true,
		OpCode: opCode,
		Length: int64(len(out)),
	}, out)
}

func BuildWsPkt(pl []byte, code byte) []byte {
	h := &Header{
		Fin:    true,
		OpCode: code,
		Length: int64(len(pl)),
	}

	size, hsize := calcPktSize(h)
	buf := make([]byte, size)

	writeHeader(h, hsize, buf)
	copy(buf[hsize:], pl)
	return buf
}

func buildFrame(h *Header, pl []byte) []byte {
	size64, hsize64 := calcPktSize(h)
	if size64 <= 0 || hsize64 <= 0 {
		return nil
	}
	if size64 > int64(^uint(0)>>1) {
		return nil
	}

	size := int(size64)
	hsize := int(hsize64)
	buf := make([]byte, size)
	writeHeader(h, int64(hsize), buf)
	copy(buf[hsize:], pl)
	return buf
}
