package connaxis

import (
	"bytes"
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
