package websocket

import (
	"bytes"
	"compress/flate"
	"errors"
	"io"
)

var (
	extPermessageDeflateToken      = []byte("permessage-deflate")
	extClientNoContextTakeover     = []byte("client_no_context_takeover")
	extServerNoContextTakeover     = []byte("server_no_context_takeover")
	extClientMaxWindowBits         = []byte("client_max_window_bits")
	extServerMaxWindowBits         = []byte("server_max_window_bits")
	extPermessageDeflateServerLine = []byte("Sec-WebSocket-Extensions: permessage-deflate; server_no_context_takeover; client_no_context_takeover\r\n")
	extPMDMessageTail              = []byte{0x00, 0x00, 0xff, 0xff}
	extPMDDecodeTail               = []byte{0x00, 0x00, 0xff, 0xff, 0x01, 0x00, 0x00, 0xff, 0xff}
	errPMDMessageTooLarge          = errors.New("websocket permessage-deflate message too large")
)

type pmdInflater struct {
	reader io.ReadCloser
	reset  flate.Resetter
	encBuf []byte
	tmpBuf []byte
	outBuf bytes.Buffer
}

func negotiatePermessageDeflate(headerVal []byte) bool {
	for len(headerVal) > 0 {
		offer := headerVal
		if idx := bytes.IndexByte(headerVal, ','); idx >= 0 {
			offer = headerVal[:idx]
			headerVal = headerVal[idx+1:]
		} else {
			headerVal = nil
		}

		offer = trim(offer)
		if len(offer) == 0 {
			continue
		}

		parts := bytes.Split(offer, []byte(";"))
		if len(parts) == 0 {
			continue
		}
		if !equalFoldASCII(trim(parts[0]), extPermessageDeflateToken) {
			continue
		}

		hasClientNoContextTakeover := false
		valid := true
		for _, raw := range parts[1:] {
			param := trim(raw)
			if len(param) == 0 {
				continue
			}

			name := param
			value := []byte(nil)
			hasValue := false
			if idx := bytes.IndexByte(param, '='); idx >= 0 {
				name = trim(param[:idx])
				value = trim(param[idx+1:])
				if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
					value = value[1 : len(value)-1]
				}
				hasValue = true
			}

			switch {
			case equalFoldASCII(name, extClientNoContextTakeover):
				if hasValue {
					valid = false
					break
				}
				hasClientNoContextTakeover = true
			case equalFoldASCII(name, extServerNoContextTakeover):
				if hasValue {
					valid = false
					break
				}
			case equalFoldASCII(name, extClientMaxWindowBits):
				if hasValue && !validPMDWindowBits(value) {
					valid = false
					break
				}
			case equalFoldASCII(name, extServerMaxWindowBits):
				if !hasValue || !validPMDWindowBits(value) {
					valid = false
					break
				}
			default:
				valid = false
			}

			if !valid {
				break
			}
		}

		if valid && hasClientNoContextTakeover {
			return true
		}
	}

	return false
}

func validPMDWindowBits(in []byte) bool {
	if len(in) == 0 {
		return false
	}
	value := 0
	for i := 0; i < len(in); i++ {
		ch := in[i]
		if ch < '0' || ch > '9' {
			return false
		}
		value = value*10 + int(ch-'0')
		if value > 15 {
			return false
		}
	}
	return value >= 8 && value <= 15
}

func (i *pmdInflater) close() {
	if i.reader != nil {
		_ = i.reader.Close()
		i.reader = nil
		i.reset = nil
	}
	i.encBuf = nil
	i.tmpBuf = nil
	i.outBuf.Reset()
}

func (i *pmdInflater) inflate(payload []byte, maxSize int) ([]byte, error) {
	need := len(payload) + len(extPMDDecodeTail)
	if cap(i.encBuf) < need {
		i.encBuf = make([]byte, need)
	}
	encoded := i.encBuf[:need]
	copy(encoded, payload)
	copy(encoded[len(payload):], extPMDDecodeTail)

	r := bytes.NewReader(encoded)
	if i.reader == nil {
		i.reader = flate.NewReader(r)
		if reset, ok := i.reader.(flate.Resetter); ok {
			i.reset = reset
		}
	} else if i.reset != nil {
		_ = i.reset.Reset(r, nil)
	} else {
		_ = i.reader.Close()
		i.reader = flate.NewReader(r)
		if reset, ok := i.reader.(flate.Resetter); ok {
			i.reset = reset
		}
	}

	if len(i.tmpBuf) == 0 {
		i.tmpBuf = make([]byte, 4096)
	}
	i.outBuf.Reset()

	total := 0
	for {
		n, err := i.reader.Read(i.tmpBuf)
		if n > 0 {
			total += n
			if maxSize > 0 && total > maxSize {
				return nil, errPMDMessageTooLarge
			}
			_, _ = i.outBuf.Write(i.tmpBuf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return i.outBuf.Bytes(), nil
}

func inflatePermessageDeflate(cli *WsClient, payload []byte, maxSize int) ([]byte, error) {
	return cli.inflater.inflate(payload, maxSize)
}
