package tls

import (
	stdtls "crypto/tls"
	"net"
)

type Config = stdtls.Config
type Conn = stdtls.Conn
type Certificate = stdtls.Certificate
type ConnectionState = stdtls.ConnectionState
type ClientHelloInfo = stdtls.ClientHelloInfo
type ClientSessionCache = stdtls.ClientSessionCache
type CipherSuite = stdtls.CipherSuite

const (
	VersionTLS10 = stdtls.VersionTLS10
	VersionTLS11 = stdtls.VersionTLS11
	VersionTLS12 = stdtls.VersionTLS12
	VersionTLS13 = stdtls.VersionTLS13
)

const (
	TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256   = stdtls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
	TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384   = stdtls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
	TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256 = stdtls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
	TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384 = stdtls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
	TLS_RSA_WITH_AES_128_GCM_SHA256         = stdtls.TLS_RSA_WITH_AES_128_GCM_SHA256
	TLS_RSA_WITH_AES_256_GCM_SHA384         = stdtls.TLS_RSA_WITH_AES_256_GCM_SHA384
	TLS_AES_128_GCM_SHA256                  = stdtls.TLS_AES_128_GCM_SHA256
	TLS_AES_256_GCM_SHA384                  = stdtls.TLS_AES_256_GCM_SHA384
)

var (
	LoadX509KeyPair = stdtls.LoadX509KeyPair
	NewLRUClientSessionCache = stdtls.NewLRUClientSessionCache
	CipherSuiteName = stdtls.CipherSuiteName
	CipherSuites = stdtls.CipherSuites
	InsecureCipherSuites = stdtls.InsecureCipherSuites
)

func Server(conn net.Conn, config *Config) *stdtls.Conn {
	return stdtls.Server(conn, config)
}

func Client(conn net.Conn, config *Config) *stdtls.Conn {
	return stdtls.Client(conn, config)
}
