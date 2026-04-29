package connaxis

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"strings"

	"github.com/bernardhu/connaxis/internal/tls"
)

func buildTLSConfig(cfg *EvConfig) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(cfg.SslPem, cfg.SslKey)
	if err != nil {
		return nil, err
	}

	ocspStaplePath := strings.TrimSpace(cfg.SslOcspStaple)
	if ocspStaplePath != "" {
		ocspStaple, readErr := loadOCSPStaple(ocspStaplePath)
		if readErr != nil {
			return nil, readErr
		}
		cert.OCSPStaple = ocspStaple
	}

	tlsCfg := &tls.Config{
		Certificates:           []tls.Certificate{cert},
		SessionTicketsDisabled: cfg.TlsSessionTicketsDisabled,
	}
	if cfg.TlsSessionTicketKeyFile != "" {
		keys, err := loadSessionTicketKeys(cfg.TlsSessionTicketKeyFile)
		if err != nil {
			return nil, err
		}
		tlsCfg.SetSessionTicketKeys(keys)
	}
	if cfg.TlsMinVersion != 0 {
		tlsCfg.MinVersion = cfg.TlsMinVersion
	}
	if cfg.TlsMaxVersion != 0 {
		tlsCfg.MaxVersion = cfg.TlsMaxVersion
	}
	if len(cfg.TlsNextProtos) > 0 {
		tlsCfg.NextProtos = append([]string(nil), cfg.TlsNextProtos...)
	}
	return tlsCfg, nil
}

func loadSessionTicketKeys(path string) ([][32]byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tls session ticket key file %q: %w", path, err)
	}
	lines := bytes.Split(content, []byte{'\n'})
	keys := make([][32]byte, 0, len(lines))
	for idx, line := range lines {
		key, ok, err := parseSessionTicketKeyLine(line)
		if err != nil {
			return nil, fmt.Errorf("parse tls session ticket key file %q line %d: %w", path, idx+1, err)
		}
		if ok {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("tls session ticket key file %q has no keys", path)
	}
	return keys, nil
}

func parseSessionTicketKeyLine(line []byte) ([32]byte, bool, error) {
	var key [32]byte
	line = bytes.TrimSpace(line)
	if len(line) == 0 || bytes.HasPrefix(line, []byte("#")) {
		return key, false, nil
	}

	raw := make([]byte, 0, 32)
	if len(line) == 32 {
		raw = line
	} else if len(line) == 64 && isHex(line) {
		decoded, err := hex.DecodeString(string(line))
		if err != nil {
			return key, false, err
		}
		raw = decoded
	} else if decoded, err := base64.StdEncoding.DecodeString(string(line)); err == nil {
		raw = decoded
	} else {
		raw = line
	}
	if len(raw) != 32 {
		return key, false, fmt.Errorf("key length is %d bytes, want 32", len(raw))
	}
	copy(key[:], raw)
	return key, true, nil
}

func isHex(in []byte) bool {
	for _, b := range in {
		switch {
		case b >= '0' && b <= '9':
		case b >= 'a' && b <= 'f':
		case b >= 'A' && b <= 'F':
		default:
			return false
		}
	}
	return true
}

func loadOCSPStaple(path string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read ocsp staple file %q: %w", path, err)
	}
	if len(content) == 0 {
		return nil, fmt.Errorf("ocsp staple file %q is empty", path)
	}

	trimmed := bytes.TrimSpace(content)
	if bytes.HasPrefix(trimmed, []byte("-----BEGIN")) {
		blockData := trimmed
		for len(blockData) > 0 {
			block, rest := pem.Decode(blockData)
			if block == nil {
				break
			}
			if strings.EqualFold(block.Type, "OCSP RESPONSE") {
				if len(block.Bytes) == 0 {
					return nil, fmt.Errorf("ocsp staple file %q has empty PEM block", path)
				}
				return block.Bytes, nil
			}
			blockData = rest
		}
		return nil, fmt.Errorf("ocsp staple file %q does not contain an OCSP RESPONSE PEM block", path)
	}

	return content, nil
}
