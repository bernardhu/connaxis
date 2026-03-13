package connaxis

import (
	"crypto/tls"
	"testing"

	"github.com/bernardhu/connaxis/connection"
)

func TestApplyKTLSPolicy(t *testing.T) {
	tests := []struct {
		name    string
		policy  string
		wantRX  bool
		wantMin uint16
		wantMax uint16
	}{
		{name: "default tls12 tx", policy: "", wantRX: false, wantMin: tls.VersionTLS12, wantMax: tls.VersionTLS12},
		{name: "tls12 tx", policy: "tls12-tx", wantRX: false, wantMin: tls.VersionTLS12, wantMax: tls.VersionTLS12},
		{name: "tls13 tx", policy: "tls13-tx", wantRX: false, wantMin: tls.VersionTLS13, wantMax: tls.VersionTLS13},
		{name: "tls12 rxtx", policy: "tls12-rxtx", wantRX: true, wantMin: tls.VersionTLS12, wantMax: tls.VersionTLS12},
		{name: "tls13 rxtx", policy: "tls13-rxtx", wantRX: true, wantMin: tls.VersionTLS13, wantMax: tls.VersionTLS13},
		{name: "unknown fallback", policy: "wat", wantRX: false, wantMin: tls.VersionTLS12, wantMax: tls.VersionTLS12},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prevRX := connection.KTLSEnableRX
			defer func() { connection.KTLSEnableRX = prevRX }()

			cfg := GetDefaultConfig()
			cfg.KTLSPolicy = tc.policy
			cfg.TlsMinVersion = 0
			cfg.TlsMaxVersion = 0

			applyKTLSPolicy(cfg)

			if got := connection.KTLSEnableRX; got != tc.wantRX {
				t.Fatalf("KTLSEnableRX=%v want %v", got, tc.wantRX)
			}
			if cfg.TlsMinVersion != tc.wantMin {
				t.Fatalf("TlsMinVersion=%#x want %#x", cfg.TlsMinVersion, tc.wantMin)
			}
			if cfg.TlsMaxVersion != tc.wantMax {
				t.Fatalf("TlsMaxVersion=%#x want %#x", cfg.TlsMaxVersion, tc.wantMax)
			}
		})
	}
}
