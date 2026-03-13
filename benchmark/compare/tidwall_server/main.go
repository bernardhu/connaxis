package main

import (
	"flag"
	"log"
	"strings"

	"github.com/bernardhu/connaxis/benchmark/compare/common"
	"github.com/tidwall/evio"
)

type modeState struct {
	mode       string
	wsUpgraded bool
}

func main() {
	var (
		addr  string
		mode  string
		reuse bool
		loops int
	)
	flag.StringVar(&addr, "addr", ":5000", "listen address")
	flag.StringVar(&mode, "mode", "tcp", "tcp|http|ws")
	flag.BoolVar(&reuse, "reuseport", false, "reuseport")
	flag.IntVar(&loops, "loops", -1, "event loops")
	flag.Parse()

	if mode == "tls" || mode == "wss" {
		log.Fatal("tidwall/evio server: tls/wss not supported in this harness")
	}

	events := evio.Events{}
	events.NumLoops = loops
	events.LoadBalance = evio.RoundRobin
	events.Opened = func(c evio.Conn) (out []byte, opts evio.Options, action evio.Action) {
		c.SetContext(&modeState{mode: mode})
		return nil, evio.Options{ReuseInputBuffer: true}, evio.None
	}
	events.Closed = func(c evio.Conn, err error) (action evio.Action) {
		_ = err
		return evio.None
	}
	events.Data = func(c evio.Conn, in []byte) (out []byte, action evio.Action) {
		st := c.Context().(*modeState)
		switch st.mode {
		case "tcp":
			if len(in) == 0 {
				return nil, evio.None
			}
			return in, evio.None
		case "http":
			if !common.ParseHTTPRequest(in) {
				return nil, evio.None
			}
			return common.HTTPResponse(), evio.None
		case "ws":
			if !st.wsUpgraded {
				head := string(in)
				key := ""
				for _, line := range strings.Split(head, "\r\n") {
					if strings.HasPrefix(strings.ToLower(line), "sec-websocket-key:") {
						key = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
						break
					}
				}
				if key == "" {
					return nil, evio.Close
				}
				st.wsUpgraded = true
				return []byte(common.WSHandshakeResponse(key)), evio.None
			}
			payload, _, ok := common.ParseWSFrame(in)
			if !ok {
				return nil, evio.None
			}
			out := make([]byte, 0, len(payload)+2)
			out = append(out, 0x81, byte(len(payload)))
			out = append(out, payload...)
			return out, evio.None
		}
		return nil, evio.None
	}

	addrStr := "tcp://" + addr
	if reuse {
		addrStr = addrStr + "?reuseport=true"
	}
	log.Fatal(evio.Serve(events, addrStr))
}
