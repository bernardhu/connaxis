//go:build !linux

package internal

import "errors"

var ErrKTLSUnsupported = errors.New("ktls unsupported")

func SystemSupportKTLS() (bool, error) {
	return false, ErrKTLSUnsupported
}
