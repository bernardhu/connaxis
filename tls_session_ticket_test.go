package connaxis

import "testing"

func TestBuildTLSSessionTicketKeysOrder(t *testing.T) {
	const (
		seed        = "shared-cluster-seed"
		context     = "prod/example.com"
		rotationSec = int64(defaultTLSSessionTicketRotationSec)
		currentSec  = int64(100 * defaultTLSSessionTicketRotationSec)
	)

	keys := buildTLSSessionTicketKeys(seed, context, currentSec)
	if got, want := len(keys), len(tlsSessionTicketKeyOffsets); got != want {
		t.Fatalf("len(keys) = %d, want %d", got, want)
	}

	wantSecs := []int64{
		currentSec,
		currentSec + rotationSec,
		currentSec - rotationSec,
		currentSec - 2*rotationSec,
		currentSec - 3*rotationSec,
		currentSec - 4*rotationSec,
		currentSec - 5*rotationSec,
		currentSec - 6*rotationSec,
		currentSec - 7*rotationSec,
	}
	for i, wantSec := range wantSecs {
		wantKey := deriveTLSSessionTicketKey(seed, context, wantSec)
		if keys[i] != wantKey {
			t.Fatalf("keys[%d] derived from sec %d, want sec %d", i, secForDerivedKey(seed, context, keys[i], wantSecs), wantSec)
		}
	}
}

func TestHasManagedTLSSessionTicketKeys(t *testing.T) {
	if hasManagedTLSSessionTicketKeys(&EvConfig{}) {
		t.Fatalf("empty config unexpectedly enabled managed ticket keys")
	}
	if hasManagedTLSSessionTicketKeys(&EvConfig{TlsSessionTicketSeed: "seed", TlsSessionTicketsDisabled: true}) {
		t.Fatalf("disabled session tickets unexpectedly enabled managed ticket keys")
	}
	if !hasManagedTLSSessionTicketKeys(&EvConfig{TlsSessionTicketSeed: "seed"}) {
		t.Fatalf("shared seed did not enable managed ticket keys")
	}
}

func secForDerivedKey(seed, context string, key [32]byte, candidates []int64) int64 {
	for _, sec := range candidates {
		if deriveTLSSessionTicketKey(seed, context, sec) == key {
			return sec
		}
	}
	return -1
}
