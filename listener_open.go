package connaxis

import (
	"fmt"
	"strings"

	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/eventloop"

	reuseport "github.com/kavu/go_reuseport"
)

func openListener(ep eventloop.IEVEndpoint, cfg *EvConfig) (*listener, error) {
	network := strings.ToLower(strings.TrimSpace(ep.Network()))
	if network == "" {
		network = "tcp"
	}
	if network != "tcp" {
		return nil, fmt.Errorf("unsupported network %q: only tcp is supported", ep.Network())
	}

	ln := &listener{
		ep:   ep,
		addr: ep.String(),
	}

	var err error
	if cfg != nil && cfg.SslPem != "" && cfg.SslKey != "" {
		if cfg.SslMode == "tls" {
			ln.tlsmode = connection.TYPE_CONN_TLS
			ln.config, err = buildTLSConfig(cfg)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("ssl mode %q is not supported", cfg.SslMode)
		}
	}

	ln.ln, err = reuseport.Listen(network, ep.String())
	if err != nil {
		return nil, err
	}

	ln.lnaddr = ln.ln.Addr()

	if err := ln.system(); err != nil {
		return nil, err
	}

	return ln, nil
}
