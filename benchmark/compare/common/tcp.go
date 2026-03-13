package common

import "encoding/binary"

// Fixed length-prefixed frame: 4-byte big-endian length + payload.

func EncodeFrame(dst []byte, payload []byte) []byte {
	need := 4 + len(payload)
	if cap(dst) < need {
		dst = make([]byte, need)
	} else {
		dst = dst[:need]
	}
	binary.BigEndian.PutUint32(dst[:4], uint32(len(payload)))
	copy(dst[4:], payload)
	return dst
}

// ParseFrame returns (payload, ok). It expects full frame in buf.
func ParseFrame(buf []byte) ([]byte, bool) {
	if len(buf) < 4 {
		return nil, false
	}
	ln := int(binary.BigEndian.Uint32(buf[:4]))
	if ln < 0 || len(buf) < 4+ln {
		return nil, false
	}
	return buf[4 : 4+ln], true
}
