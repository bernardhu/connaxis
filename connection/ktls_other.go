//go:build !linux

package connection

import (
	"context"
	"errors"

	"github.com/bernardhu/connaxis/internal/tls"
)

func newKTLSConnServer(ctx context.Context, fd int, cfg *tls.Config) (*ATLSConn, error) {
	return nil, errors.New("ktls supported on linux only")
}

func newKTLSConnClient(ctx context.Context, fd int, cfg *tls.Config) (*ATLSConn, error) {
	return nil, errors.New("ktls supported on linux only")
}

func (c *ATLSConn) finalizeServerKTLS() error {
	return nil
}
