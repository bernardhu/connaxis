package common

import (
	"crypto/tls"
	"fmt"
	"os"
)

func LoadTLSConfig(certFile, keyFile string) (*tls.Config, error) {
	if certFile == "" || keyFile == "" {
		return nil, fmt.Errorf("tls cert/key required")
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

func MustFile(path string) string {
	if _, err := os.Stat(path); err != nil {
		panic(err)
	}
	return path
}
