package connaxis

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"time"

	"github.com/bernardhu/connaxis/internal/tls"
	"github.com/bernardhu/connaxis/wrapper"
)

const (
	defaultTLSSessionTicketRotationSec = 24 * 60 * 60
	tlsSessionTicketHistoryKeys        = 7
	tlsSessionTicketRotationCheckEvery = time.Second
)

var tlsSessionTicketKeyOffsets = []int64{0, 1, -1, -2, -3, -4, -5, -6, -7}

func hasManagedTLSSessionTicketKeys(cfg *EvConfig) bool {
	return cfg != nil && !cfg.TlsSessionTicketsDisabled && cfg.TlsSessionTicketSeed != ""
}

func tlsSessionTicketWindowStartSec(now time.Time) int64 {
	rotationSec := int64(defaultTLSSessionTicketRotationSec)
	return now.Unix() / rotationSec * rotationSec
}

func deriveTLSSessionTicketKey(seed, context string, sec int64) [32]byte {
	mac := hmac.New(sha256.New, []byte(seed))
	_, _ = mac.Write([]byte("connaxis/tls-session-ticket/v1"))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(context))
	var meta [8]byte
	binary.BigEndian.PutUint64(meta[:], uint64(sec))
	_, _ = mac.Write(meta[:])

	var key [32]byte
	copy(key[:], mac.Sum(nil))
	return key
}

func buildTLSSessionTicketKeys(seed, context string, currentSec int64) [][32]byte {
	rotationSec := int64(defaultTLSSessionTicketRotationSec)
	keys := make([][32]byte, 0, 2+tlsSessionTicketHistoryKeys)
	for _, offset := range tlsSessionTicketKeyOffsets {
		keys = append(keys, deriveTLSSessionTicketKey(seed, context, currentSec+offset*rotationSec))
	}
	return keys
}

func applyManagedTLSSessionTicketKeys(tlsCfg *tls.Config, cfg *EvConfig, now time.Time) (int64, bool) {
	if tlsCfg == nil || !hasManagedTLSSessionTicketKeys(cfg) {
		return 0, false
	}

	currentSec := tlsSessionTicketWindowStartSec(now)
	tlsCfg.SetSessionTicketKeys(buildTLSSessionTicketKeys(
		cfg.TlsSessionTicketSeed,
		cfg.TlsSessionTicketContext,
		currentSec,
	))
	return currentSec, true
}

func (s *Server) startTLSSessionTicketRotation(tlsCfg *tls.Config) {
	if s == nil || tlsCfg == nil || !hasManagedTLSSessionTicketKeys(s.cfg) {
		return
	}

	rotationSec := int64(defaultTLSSessionTicketRotationSec)
	context := s.cfg.TlsSessionTicketContext
	currentSec, ok := applyManagedTLSSessionTicketKeys(tlsCfg, s.cfg, time.Now())
	if !ok {
		return
	}
	wrapper.Infof("tls session ticket resumption enabled: rotationSec=%d context=%q keys=%d", rotationSec, context, len(tlsSessionTicketKeyOffsets))

	go func() {
		ticker := time.NewTicker(tlsSessionTicketRotationCheckEvery)
		defer ticker.Stop()
		for {
			select {
			case <-s.done:
				return
			case now := <-ticker.C:
				if now.Unix()-currentSec < rotationSec {
					continue
				}
				currentSec = tlsSessionTicketWindowStartSec(now)
			}

			tlsCfg.SetSessionTicketKeys(buildTLSSessionTicketKeys(
				s.cfg.TlsSessionTicketSeed,
				s.cfg.TlsSessionTicketContext,
				currentSec,
			))
			wrapper.Debugf("tls session ticket keys rotated: sec=%d context=%q", currentSec, context)
		}
	}()
}
