package websocket

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bernardhu/connaxis"
	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/eventloop"
	gws "github.com/gorilla/websocket"
)

func requireTCPListen(t *testing.T) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("tcp listen not available: %v", err)
		return
	}
	_ = ln.Close()
}

func reserveTCPListenAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve tcp addr: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func writeSelfSignedCertFiles(t *testing.T) (certPath, keyPath string) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatalf("serial: %v", err)
	}

	now := time.Now()
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}

	dir := t.TempDir()
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return certPath, keyPath
}

type tlsEchoHandler struct {
	connected chan struct{}
	gotData   chan []byte
}

func (h *tlsEchoHandler) OnReady(eventloop.IServer) {}

func (h *tlsEchoHandler) OnClosed(connection.AppConn, error) {}

func (h *tlsEchoHandler) OnConnected(c connection.ProtoConn) {
	c.SetPktHandler(h)
	if h.connected != nil {
		select {
		case h.connected <- struct{}{}:
		default:
		}
	}
}

func (h *tlsEchoHandler) Stat(bool) {}

func (h *tlsEchoHandler) ParsePacket(c connection.ProtoConn, in *[]byte) (int, int) {
	if len(*in) == 0 {
		return 0, 0
	}
	return len(*in), len(*in)
}

func (h *tlsEchoHandler) OnData(c connection.ProtoConn, in *[]byte) ([]byte, bool) {
	if h.gotData != nil && len(*in) > 0 {
		got := append([]byte(nil), (*in)...)
		select {
		case h.gotData <- got:
		default:
		}
	}
	return *in, false
}

func TestConnaxisTLSEcho(t *testing.T) {
	requireTCPListen(t)
	certPath, keyPath := writeSelfSignedCertFiles(t)

	cfg := connaxis.GetDefaultConfig()
	cfg.Ncpu = 1
	cfg.SslMode = "tls"
	cfg.SslPem = certPath
	cfg.SslKey = keyPath
	cfg.ListenAddrs = []eventloop.IEVEndpoint{
		&connaxis.EVEndpoint{Net: "tcp", Address: reserveTCPListenAddr(t)},
	}

	h := &tlsEchoHandler{
		connected: make(chan struct{}, 8),
		gotData:   make(chan []byte, 8),
	}
	if err, srv := connaxis.ServeByConfig(h, cfg, false); err != nil {
		t.Fatalf("serve: %v", err)
	} else {
		defer srv.Stop()
		addr := srv.GetListenAddrs()[0].String()

		for _, tc := range []struct {
			name     string
			min, max uint16
		}{
			{name: "TLS12", min: tls.VersionTLS12, max: tls.VersionTLS12},
			{name: "TLS13", min: tls.VersionTLS13, max: tls.VersionTLS13},
		} {
			t.Run(tc.name, func(t *testing.T) {
				for {
					select {
					case <-h.connected:
					default:
						goto drainedConnected
					}
				}
			drainedConnected:
				for {
					select {
					case <-h.gotData:
					default:
						goto drainedData
					}
				}
			drainedData:
				conn, err := tls.Dial("tcp", addr, &tls.Config{
					InsecureSkipVerify: true,
					MinVersion:         tc.min,
					MaxVersion:         tc.max,
				})
				if err != nil {
					t.Fatalf("dial: %v", err)
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))

				select {
				case <-h.connected:
				case <-time.After(2 * time.Second):
					t.Fatalf("server did not finish accept/OnConnected in time")
				}

				payload := []byte("ping")
				if _, err := conn.Write(payload); err != nil {
					t.Fatalf("write: %v", err)
				}

				select {
				case <-h.gotData:
				case <-time.After(2 * time.Second):
					t.Fatalf("server did not observe any plaintext data")
				}

				buf := make([]byte, len(payload))
				if _, err := io.ReadFull(conn, buf); err != nil {
					t.Fatalf("read: %v", err)
				}
				if !bytes.Equal(buf, payload) {
					t.Fatalf("unexpected echo got=%q want=%q", buf, payload)
				}
			})
		}
	}
}

func TestConnaxisTLSSessionResumptionAcrossReusePortListeners(t *testing.T) {
	requireTCPListen(t)
	certPath, keyPath := writeSelfSignedCertFiles(t)

	cfg := connaxis.GetDefaultConfig()
	cfg.Ncpu = 4
	cfg.SslMode = "tls"
	cfg.SslPem = certPath
	cfg.SslKey = keyPath
	cfg.ListenAddrs = []eventloop.IEVEndpoint{
		&connaxis.EVEndpoint{Net: "tcp", Address: reserveTCPListenAddr(t)},
	}

	h := &tlsEchoHandler{
		connected: make(chan struct{}, 16),
		gotData:   make(chan []byte, 16),
	}
	err, srv := connaxis.ServeByConfig(h, cfg, false)
	if err != nil {
		t.Fatalf("serve: %v", err)
	}
	defer srv.Stop()

	addr := srv.GetListenAddrs()[0].String()
	clientCfg := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         "localhost",
		MinVersion:         tls.VersionTLS12,
		MaxVersion:         tls.VersionTLS12,
		ClientSessionCache: tls.NewLRUClientSessionCache(16),
	}

	for i := 0; i < 6; i++ {
		conn, err := tls.Dial("tcp", addr, clientCfg)
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		didResume := conn.ConnectionState().DidResume
		_ = conn.Close()

		if i == 0 {
			if didResume {
				t.Fatalf("first dial unexpectedly resumed")
			}
			continue
		}
		if !didResume {
			t.Fatalf("dial %d did not resume TLS session", i)
		}
	}
}

func TestConnaxisTLSSessionResumptionAcrossIndependentServersWithSharedSeed(t *testing.T) {
	requireTCPListen(t)
	certPath, keyPath := writeSelfSignedCertFiles(t)

	newTLSConfig := func(addr string) *connaxis.EvConfig {
		cfg := connaxis.GetDefaultConfig()
		cfg.Ncpu = 1
		cfg.SslMode = "tls"
		cfg.SslPem = certPath
		cfg.SslKey = keyPath
		cfg.TlsMinVersion = tls.VersionTLS12
		cfg.TlsMaxVersion = tls.VersionTLS12
		cfg.TlsSessionTicketSeed = "cluster-shared-ticket-seed"
		cfg.TlsSessionTicketContext = "ws-tls-integration"
		cfg.ListenAddrs = []eventloop.IEVEndpoint{
			&connaxis.EVEndpoint{Net: "tcp", Address: addr},
		}
		return cfg
	}

	h1 := &tlsEchoHandler{connected: make(chan struct{}, 8), gotData: make(chan []byte, 8)}
	err, srv1 := connaxis.ServeByConfig(h1, newTLSConfig(reserveTCPListenAddr(t)), false)
	if err != nil {
		t.Fatalf("serve server1: %v", err)
	}
	defer srv1.Stop()

	h2 := &tlsEchoHandler{connected: make(chan struct{}, 8), gotData: make(chan []byte, 8)}
	err, srv2 := connaxis.ServeByConfig(h2, newTLSConfig(reserveTCPListenAddr(t)), false)
	if err != nil {
		t.Fatalf("serve server2: %v", err)
	}
	defer srv2.Stop()

	clientCfg := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         "localhost",
		MinVersion:         tls.VersionTLS12,
		MaxVersion:         tls.VersionTLS12,
		ClientSessionCache: tls.NewLRUClientSessionCache(16),
	}

	conn, err := tls.Dial("tcp", srv1.GetListenAddrs()[0].String(), clientCfg)
	if err != nil {
		t.Fatalf("dial server1: %v", err)
	}
	if conn.ConnectionState().DidResume {
		_ = conn.Close()
		t.Fatalf("first dial unexpectedly resumed")
	}
	_ = conn.Close()

	conn, err = tls.Dial("tcp", srv2.GetListenAddrs()[0].String(), clientCfg)
	if err != nil {
		t.Fatalf("dial server2: %v", err)
	}
	defer conn.Close()
	if !conn.ConnectionState().DidResume {
		t.Fatalf("second dial did not resume TLS session across independent servers")
	}
}

type wsUserEchoHandler struct{}

func (h *wsUserEchoHandler) OnUserData(ctx *WsCtx) []byte {
	return ctx.Data
}

type wsServerHandler struct {
	WsHandler
}

func (h *wsServerHandler) OnReady(eventloop.IServer) {}

func TestConnaxisWSS(t *testing.T) {
	requireTCPListen(t)
	certPath, keyPath := writeSelfSignedCertFiles(t)

	cfg := connaxis.GetDefaultConfig()
	cfg.Ncpu = 1
	cfg.SslMode = "tls"
	cfg.SslPem = certPath
	cfg.SslKey = keyPath
	cfg.ListenAddrs = []eventloop.IEVEndpoint{
		&connaxis.EVEndpoint{Net: "tcp", Address: reserveTCPListenAddr(t)},
	}

	h := &wsServerHandler{}
	h.SetUserDataHandler(&wsUserEchoHandler{})

	if err, srv := connaxis.ServeByConfig(h, cfg, false); err != nil {
		t.Fatalf("serve: %v", err)
	} else {
		defer srv.Stop()

		addr := srv.GetListenAddrs()[0].String()
		u := url.URL{Scheme: "wss", Host: addr, Path: "/"}
		dialer := gws.Dialer{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		c, resp, err := dialer.Dial(u.String(), nil)
		if resp != nil && resp.Body != nil {
			defer resp.Body.Close()
		}
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer c.Close()

		if c.WriteMessage(gws.TextMessage, []byte("hello")) != nil {
			t.Fatalf("write msg: %v", err)
		}
		_, msg, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("read msg: %v", err)
		}
		if string(msg) != "hello" {
			t.Fatalf("unexpected echo: %q", msg)
		}
	}
}
