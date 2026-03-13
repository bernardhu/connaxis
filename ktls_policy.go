package connaxis

import (
	"crypto/tls"
	"strings"

	"github.com/bernardhu/connaxis/connection"
	ktls "github.com/bernardhu/connaxis/internal/ktls"
	"github.com/bernardhu/connaxis/wrapper"
)

func applyKTLSPolicy(cfg *EvConfig) {
	policy := strings.ToLower(strings.TrimSpace(cfg.KTLSPolicy))
	switch policy {
	case "", "tls12-tx":
		connection.KTLSEnableRX = false
		cfg.TlsMinVersion = tls.VersionTLS12
		cfg.TlsMaxVersion = tls.VersionTLS12
		wrapper.Infof("ktls policy=tls12-tx (force tls1.2 + tx-only)")
	case "tls13-tx":
		connection.KTLSEnableRX = false
		cfg.TlsMinVersion = tls.VersionTLS13
		cfg.TlsMaxVersion = tls.VersionTLS13
		wrapper.Infof("ktls policy=tls13-tx (force tls1.3 + tx-only)")
	case "tls12-rxtx":
		connection.KTLSEnableRX = true
		cfg.TlsMinVersion = tls.VersionTLS12
		cfg.TlsMaxVersion = tls.VersionTLS12
		wrapper.Infof("ktls policy=tls12-rxtx (force tls1.2 + rx/tx)")
	case "tls13-rxtx":
		connection.KTLSEnableRX = true
		cfg.TlsMinVersion = tls.VersionTLS13
		cfg.TlsMaxVersion = tls.VersionTLS13
		wrapper.Infof("ktls policy=tls13-rxtx (force tls1.3 + rx/tx)")
	default:
		connection.KTLSEnableRX = false
		cfg.TlsMinVersion = tls.VersionTLS12
		cfg.TlsMaxVersion = tls.VersionTLS12
		wrapper.Warnf("unknown ktlsPolicy=%q, fallback to tls12-tx", cfg.KTLSPolicy)
	}

	ktls.KTLSPreferTLS12IfNoAESGCM = connection.KTLSPreferTLS12IfNoAESGCM
	ktls.KTLSForceTLS13AESGCM = connection.KTLSForceTLS13AESGCM
	ktls.KTLSEnableRX = connection.KTLSEnableRX
	ktls.KTLSDisableSessionTickets = connection.KTLSDisableSessionTickets
	ktls.KTLSClientPostHandshakeTimeout = connection.KTLSClientPostHandshakeTimeout
	ktls.KTLSClientPostHandshakeReadMaxBytes = connection.KTLSClientPostHandshakeReadMaxBytes
}
