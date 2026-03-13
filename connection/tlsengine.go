package connection

import (
	"strings"
)

type TLSEngine int

const (
	TLSEngineATLS TLSEngine = iota
	TLSEngineKTLS
)

var tlsEngine = TLSEngineATLS

func SetTLSEngine(engine TLSEngine) {
	tlsEngine = engine
}

func GetTLSEngine() TLSEngine {
	return tlsEngine
}

// ResolveTLSEngine selects the effective TLS engine.
// requested values: "atls" (default) and "ktls".
func ResolveTLSEngine(requested string) TLSEngine {
	switch strings.ToLower(strings.TrimSpace(requested)) {
	case "", "atls":
		return TLSEngineATLS
	case "ktls":
		return TLSEngineKTLS
	default:
		return TLSEngineATLS
	}
}
