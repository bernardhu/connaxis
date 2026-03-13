package connection

import (
	"time"
)

// KTLSPreferTLS12IfNoAESGCM forces TLS1.2 when client doesn't advertise TLS1.3 AES-GCM.
var KTLSPreferTLS12IfNoAESGCM = true

// KTLSForceTLS13AESGCM restricts TLS1.3 cipher suites to AES-GCM.
var KTLSForceTLS13AESGCM = true

// KTLSEnableRX controls whether kTLS RX is enabled.
var KTLSEnableRX = false

// KTLSDisableSessionTickets disables TLS session tickets when kTLS is enabled.
var KTLSDisableSessionTickets = false

// KTLSClientPostHandshakeTimeout is the max time to wait for TLS1.3 tickets.
var KTLSClientPostHandshakeTimeout = 10 * time.Millisecond

// KTLSClientPostHandshakeReadMaxBytes caps buffered app data during ticket drain.
var KTLSClientPostHandshakeReadMaxBytes = 64 * 1024
