package main

import (
	"crypto/tls"
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"
	"strings"

	"github.com/bernardhu/connaxis"
	"github.com/bernardhu/connaxis/benchmark/compare/common"
	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/eventloop"
)

type modeState struct {
	mode       string
	wsUpgraded bool
}

type handler struct {
	mode string
}

func (h *handler) OnReady(s eventloop.IServer) {
	log.Printf("ready: listen on %v (loops: %d)", s.GetListenAddrs(), s.GetWorkerNum())
}

func (h *handler) OnClosed(c connection.AppConn, err error) {
	_ = err
	c.SetContext(nil)
}

func (h *handler) OnConnected(c connection.ProtoConn) {
	c.SetPktHandler(h)
	c.SetContext(&modeState{mode: h.mode})
}

func (h *handler) ParsePacket(c connection.ProtoConn, in *[]byte) (int, int) {
	buf := *in
	if len(buf) == 0 {
		return 0, 0
	}
	return len(buf), len(buf)

}

func (h *handler) OnData(c connection.ProtoConn, in *[]byte) ([]byte, bool) {
	st := c.Context().(*modeState)
	buf := *in

	switch st.mode {
	case "tcp", "tls":
		return buf, false
	case "http":
		if !common.ParseHTTPRequest(buf) {
			return nil, false
		}
		return common.HTTPResponse(), false
	case "ws", "wss":
		if !st.wsUpgraded {
			// handshake
			head := string(buf)
			key := ""
			for _, line := range strings.Split(head, "\r\n") {
				if strings.HasPrefix(strings.ToLower(line), "sec-websocket-key:") {
					key = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
					break
				}
			}
			if key == "" {
				return nil, true
			}
			st.wsUpgraded = true
			resp := common.WSHandshakeResponse(key)
			return []byte(resp), false
		}
		// echo frame (assume text, no fragmentation)
		payload, _, ok := common.ParseWSFrame(buf)
		if !ok {
			return nil, false
		}
		out := make([]byte, 0, len(payload)+2)
		out = append(out, 0x81, byte(len(payload)))
		out = append(out, payload...)
		return out, false
	}
	return nil, false
}

func (h *handler) Stat(bool) {}

func main() {
	var (
		addr      string
		mode      string
		netw      string
		cert      string
		key       string
		tlsEngine string
		ktlsPolicy string
		tlsVersion string
		pprofAddr string
		reuse     bool
		workers   int
	)
	flag.StringVar(&addr, "addr", ":5000", "listen address")
	flag.StringVar(&mode, "mode", "tcp", "tcp|http|ws|tls|wss")
	flag.StringVar(&netw, "net", "tcp", "network: tcp")
	flag.StringVar(&cert, "cert", "../certs/cert.pem", "tls cert")
	flag.StringVar(&key, "key", "../certs/key.pem", "tls key")
	flag.StringVar(&tlsEngine, "tls-engine", "atls", "tls engine: atls|ktls")
	flag.StringVar(&ktlsPolicy, "ktls-policy", "", "kTLS policy: tls12-tx|tls12-rxtx|tls13-tx|tls13-rxtx")
	flag.StringVar(&tlsVersion, "tls-version", "", "tls version for tls/wss: 1.2|1.3")
	flag.StringVar(&pprofAddr, "pprof", "", "pprof listen address, e.g. :6060")
	flag.BoolVar(&reuse, "reuseport", true, "reuseport")
	flag.IntVar(&workers, "loops", -1, "worker loops")
	flag.Parse()

	if pprofAddr != "" {
		go func() {
			log.Printf("pprof: listening on %s", pprofAddr)
			if err := http.ListenAndServe(pprofAddr, nil); err != nil {
				log.Printf("pprof: server stopped: %v", err)
			}
		}()
	}

	cfg := connaxis.GetDefaultConfig()
	cfg.Ncpu = workers
	cfg.ListenAddrs = []eventloop.IEVEndpoint{
		&connaxis.EVEndpoint{Net: netw, Address: addr, Reuse: reuse},
	}
	if mode == "tls" || mode == "wss" {
		cfg.SslMode = "tls"
		cfg.SslPem = cert
		cfg.SslKey = key
		cfg.TlsEngine = strings.TrimSpace(strings.ToLower(tlsEngine))
		if strings.TrimSpace(ktlsPolicy) != "" {
			cfg.KTLSPolicy = strings.TrimSpace(strings.ToLower(ktlsPolicy))
		}
		switch strings.TrimSpace(strings.ToLower(tlsVersion)) {
		case "1.2", "tls1.2":
			cfg.TlsMinVersion = tls.VersionTLS12
			cfg.TlsMaxVersion = tls.VersionTLS12
		case "1.3", "tls1.3":
			cfg.TlsMinVersion = tls.VersionTLS13
			cfg.TlsMaxVersion = tls.VersionTLS13
		}
	}

	h := &handler{
		mode: mode,
	}
	if err, _ := connaxis.ServeByConfig(h, cfg, true); err != nil {
		log.Fatal(err)
	}
}
