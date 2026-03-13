package common

// ParseWSFrame parses a single frame from buf. Returns payload, frameLen, ok.
func ParseWSFrame(buf []byte) ([]byte, int, bool) {
	if len(buf) < 2 {
		return nil, 0, false
	}
	ln := int(buf[1] & 0x7f)
	off := 2
	if ln == 126 {
		if len(buf) < 4 {
			return nil, 0, false
		}
		ln = int(buf[2])<<8 | int(buf[3])
		off = 4
	} else if ln == 127 {
		return nil, 0, false
	}
	masked := buf[1]&0x80 != 0
	if masked {
		if len(buf) < off+4 {
			return nil, 0, false
		}
		maskKey := buf[off : off+4]
		off += 4
		if len(buf) < off+ln {
			return nil, 0, false
		}
		payload := make([]byte, ln)
		copy(payload, buf[off:off+ln])
		for i := 0; i < ln; i++ {
			payload[i] ^= maskKey[i%4]
		}
		return payload, off + ln, true
	}
	if len(buf) < off+ln {
		return nil, 0, false
	}
	return buf[off : off+ln], off + ln, true
}
