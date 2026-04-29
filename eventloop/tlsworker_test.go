package eventloop

import (
	"errors"
	"testing"
)

func TestTLSHandshakeErrorReason(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{errors.New("connection reset by peer"), "reset"},
		{errors.New("remote error: tls: unknown certificate"), "unknown_certificate"},
		{errors.New("tls: client offered only unsupported versions: [301]"), "unsupported_version"},
		{errors.New("tls: first record does not look like a TLS handshake"), "not_tls"},
		{errors.New("use of closed network connection"), "closed"},
		{errors.New("something else"), "other"},
		{nil, "unknown"},
	}

	for _, tt := range tests {
		if got := tlsHandshakeErrorReason(tt.err); got != tt.want {
			t.Fatalf("tlsHandshakeErrorReason(%v) = %q, want %q", tt.err, got, tt.want)
		}
	}
}
