package tls

import (
	"crypto"
	"crypto/cipher"
	"sync"
	_ "unsafe"
)

type aead interface {
	cipher.AEAD
	explicitNonceLen() int
}

type cipherSuiteTLS13 struct {
	id     uint16
	keyLen int
	aead   func(key, fixedNonce []byte) aead
	hash   crypto.Hash
}

//go:linkname cipherSuitesTLS13 crypto/tls.cipherSuitesTLS13
var cipherSuitesTLS13 []*cipherSuiteTLS13

var tls13SuitesMu sync.Mutex

// SetTLS13CipherSuites restricts TLS1.3 cipher suites to the given list.
// Returns number of suites applied (0 => no change).
func SetTLS13CipherSuites(ids []uint16) int {
	if len(ids) == 0 {
		return 0
	}
	tls13SuitesMu.Lock()
	defer tls13SuitesMu.Unlock()

	byID := make(map[uint16]*cipherSuiteTLS13, len(cipherSuitesTLS13))
	for _, suite := range cipherSuitesTLS13 {
		byID[suite.id] = suite
	}

	out := make([]*cipherSuiteTLS13, 0, len(ids))
	for _, id := range ids {
		if suite, ok := byID[id]; ok {
			out = append(out, suite)
		}
	}
	if len(out) == 0 {
		return 0
	}
	cipherSuitesTLS13 = out
	return len(out)
}

// ForceTLS13AESGCM restricts TLS1.3 to AES-GCM suites only.
func ForceTLS13AESGCM() int {
	return SetTLS13CipherSuites([]uint16{
		TLS_AES_128_GCM_SHA256,
		TLS_AES_256_GCM_SHA384,
	})
}
