package main

import (
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"strings"

	"github.com/bernardhu/connaxis"
	log "github.com/bernardhu/connaxis/benchmark/internal/examplelog"
	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/eventloop"
	tls "github.com/bernardhu/connaxis/internal/tls"
	"github.com/bernardhu/connaxis/websocket"
	"github.com/bernardhu/connaxis/wrapper"
)

type wsUserHandler struct{}

func (h *wsUserHandler) OnUserData(ctx *websocket.WsCtx) []byte {
	// Echo back exactly what we received, preserving opcode (text/binary).
	return ctx.Data
}

type handler struct {
	websocket.WsHandler
}

func (h *handler) OnReady(s eventloop.IServer) {
	log.Infof("ready: listen on %v (loops: %d) tls-engine: %s", s.GetListenAddrs(), s.GetWorkerNum(), tlsEngineName(connection.GetTLSEngine()))
}

func tlsEngineName(engine connection.TLSEngine) string {
	switch engine {
	case connection.TLSEngineKTLS:
		return "ktls"
	case connection.TLSEngineATLS:
		return "atls"
	default:
		return "unknown"
	}
}

func parseTLSVersion(raw string) (uint16, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "":
		return 0, nil
	case "tls1.2", "1.2", "tls12", "12":
		return tls.VersionTLS12, nil
	case "tls1.3", "1.3", "tls13", "13":
		return tls.VersionTLS13, nil
	default:
		return 0, fmt.Errorf("unsupported tls version %q (use tls1.2 or tls1.3)", raw)
	}
}

func main() {
	var (
		addr                  string
		netw                  string
		reuse                 bool
		useTLS                bool
		certPem               string
		keyPem                string
		logLevel              string
		tlsEngine             string
		tlsMinVersion         string
		tlsMaxVersion         string
		alpn                  string
		ocspStaple            string
		disableSessionTickets bool
		ktlsPolicy            string
		enablePlainHTTP       bool
		pprofAddr             string
	)
	flag.StringVar(&addr, "addr", ":30000", "listen address")
	flag.StringVar(&netw, "net", "tcp", "network: tcp")
	flag.BoolVar(&reuse, "reuseport", true, "reuseport")
	flag.BoolVar(&useTLS, "tls", false, "enable TLS (WSS)")
	flag.StringVar(&certPem, "cert", "benchmark/echoserver/cert.pem", "tls certificate file")
	flag.StringVar(&keyPem, "key", "benchmark/echoserver/key.pem", "tls private key file")
	flag.StringVar(&logLevel, "log-level", "error", "wrapper log level: error|info|debug")
	flag.StringVar(&tlsEngine, "tls-engine", "atls", "tls engine: atls|ktls")
	flag.StringVar(&tlsMinVersion, "tls-min-version", "", "minimum TLS version: tls1.2|tls1.3 (empty keeps default)")
	flag.StringVar(&tlsMaxVersion, "tls-max-version", "", "maximum TLS version: tls1.2|tls1.3 (empty keeps default)")
	flag.StringVar(&alpn, "alpn", "http/1.1", "comma-separated ALPN list; empty disables ALPN")
	flag.StringVar(&ocspStaple, "ocsp-staple", "", "optional OCSP staple file path (PEM or DER)")
	flag.BoolVar(&disableSessionTickets, "disable-session-tickets", true, "disable TLS session tickets")
	flag.StringVar(&ktlsPolicy, "ktls-policy", "", "kTLS policy: tls12-tx|tls12-rxtx|tls13-tx|tls13-rxtx")
	flag.BoolVar(&enablePlainHTTP, "plain-http", true, "respond minimal HTTP 200 for non-WebSocket GET requests")
	flag.StringVar(&pprofAddr, "pprof-addr", ":30002", "pprof listen address, empty disables pprof (e.g. :6060)")
	flag.Parse()

	//log
	logCfg := new(log.LogCfg)
	logCfg.LogLevel = logLevel
	logCfg.FormatTimeWithMs = true
	logCfg.FormatWithFileName = true
	log.Init(logCfg)
	defer log.Flush()

	wrapper.SetLogger(log.GetLogger())

	if strings.TrimSpace(pprofAddr) != "" {
		go func() {
			log.Infof("pprof: listen on %s", pprofAddr)
			if err := http.ListenAndServe(pprofAddr, nil); err != nil {
				log.Errorf("pprof listen failed: %v", err)
			}
		}()
	}

	cfg := connaxis.GetDefaultConfig()
	cfg.PollWait = 100
	cfg.TwInterval = 100
	cfg.IdleCheckInt = 0
	cfg.IdleLimit = 0
	cfg.PrintStat = false
	cfg.DialKeepAlive = true
	cfg.CliSendBufLimit = 64 * 1024 * 1024
	if useTLS {
		minVersion, err := parseTLSVersion(tlsMinVersion)
		if err != nil {
			log.Fatal(err)
		}
		maxVersion, err := parseTLSVersion(tlsMaxVersion)
		if err != nil {
			log.Fatal(err)
		}
		if minVersion != 0 && maxVersion != 0 && minVersion > maxVersion {
			log.Fatalf("invalid TLS version range: min=%s max=%s", tlsMinVersion, tlsMaxVersion)
		}

		cfg.SslMode = "tls"
		cfg.SslPem = certPem
		cfg.SslKey = keyPem
		cfg.SslOcspStaple = ocspStaple
		cfg.TlsEngine = tlsEngine
		cfg.TlsMinVersion = minVersion
		cfg.TlsMaxVersion = maxVersion
		if strings.TrimSpace(ktlsPolicy) != "" {
			cfg.KTLSPolicy = strings.TrimSpace(ktlsPolicy)
		}
		cfg.TlsSessionTicketsDisabled = disableSessionTickets
		if strings.TrimSpace(alpn) != "" {
			parts := strings.Split(alpn, ",")
			nextProtos := make([]string, 0, len(parts))
			for _, part := range parts {
				proto := strings.TrimSpace(part)
				if proto != "" {
					nextProtos = append(nextProtos, proto)
				}
			}
			cfg.TlsNextProtos = nextProtos
		}
	}
	cfg.ListenAddrs = append(cfg.ListenAddrs, &connaxis.EVEndpoint{
		Net:     netw,
		Address: addr,
		Reuse:   reuse,
	})

	h := &handler{}
	h.EnablePlainHTTP = enablePlainHTTP
	h.SetUserDataHandler(&wsUserHandler{})

	if err, _ := connaxis.ServeByConfig(h, cfg, true); err != nil {
		log.Fatal(err)
	}
}
