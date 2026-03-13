package websocket

import (
	"unsafe"
)

func stringToBytes(str string) []byte {
	return unsafe.Slice(unsafe.StringData(str), len(str))
}

var charLower = byte('a' - 'A')

func canonicalizeBytes(k []byte) {
	upper := true
	for i, c := range k {
		if upper && 'a' <= c && c <= 'z' {
			k[i] = k[i] - charLower
		} else if !upper && 'A' <= c && c <= 'Z' {
			k[i] = k[i] + charLower
		}
		upper = c == '-'
	}
}
