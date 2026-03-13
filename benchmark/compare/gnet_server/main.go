package main

import (
	"flag"
	"log"
	"strings"

	"github.com/bernardhu/connaxis/benchmark/compare/common"
	"github.com/panjf2000/gnet/v2"
)

type modeState struct {
	mode       string
	wsUpgraded bool
}

type handler struct {
	gnet.BuiltinEventEngine
	mode string
}

func (h *handler) OnBoot(eng gnet.Engine) (action gnet.Action) {
	log.Printf("ready: gnet server started")
	return gnet.None
}

func (h *handler) OnOpen(c gnet.Conn) (out []byte, action gnet.Action) {
	c.SetContext(&modeState{mode: h.mode})
	return nil, gnet.None
}

func (h *handler) OnClose(c gnet.Conn, err error) (action gnet.Action) {
	_ = err
	return gnet.None
}

func (h *handler) OnTraffic(c gnet.Conn) (action gnet.Action) {
	st := c.Context().(*modeState)
	buf, _ := c.Peek(-1)

	switch st.mode {
	case "tcp", "tls":
		if len(buf) == 0 {
			return gnet.None
		}
		_, _ = c.Discard(len(buf))
		_, _ = c.Write(buf)
		return gnet.None
	case "http":
		if !common.ParseHTTPRequest(buf) {
			return gnet.None
		}
		_, _ = c.Discard(len(buf))
		_, _ = c.Write(common.HTTPResponse())
		return gnet.None
	case "ws", "wss":
		if !st.wsUpgraded {
			head := string(buf)
			key := ""
			for _, line := range strings.Split(head, "\r\n") {
				if strings.HasPrefix(strings.ToLower(line), "sec-websocket-key:") {
					key = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
					break
				}
			}
			if key == "" {
				return gnet.Close
			}
			st.wsUpgraded = true
			_, _ = c.Discard(len(buf))
			_, _ = c.Write([]byte(common.WSHandshakeResponse(key)))
			return gnet.None
		}
		payload, frameLen, ok := parseWSFrame(buf)
		if !ok {
			return gnet.None
		}
		_, _ = c.Discard(frameLen)
		out := make([]byte, 0, len(payload)+2)
		out = append(out, 0x81, byte(len(payload)))
		out = append(out, payload...)
		_, _ = c.Write(out)
		return gnet.None
	}
	return gnet.None
}

func parseWSFrame(buf []byte) ([]byte, int, bool) {
	if len(buf) < 2 {
		return nil, 0, false
	}
	ln := int(buf[1] & 0x7f)
	off := 2
	if ln == 126 {
		if len(buf) < 4 {
			return nil, 0, false
		}
		ln = int(buf[2])<<8 | int(buf[3])
		off = 4
	} else if ln == 127 {
		return nil, 0, false
	}
	masked := buf[1]&0x80 != 0
	if masked {
		if len(buf) < off+4 {
			return nil, 0, false
		}
		maskKey := buf[off : off+4]
		off += 4
		if len(buf) < off+ln {
			return nil, 0, false
		}
		payload := make([]byte, ln)
		copy(payload, buf[off:off+ln])
		for i := 0; i < ln; i++ {
			payload[i] ^= maskKey[i%4]
		}
		return payload, off + ln, true
	}
	if len(buf) < off+ln {
		return nil, 0, false
	}
	return buf[off : off+ln], off + ln, true
}

func main() {
	var (
		addr  string
		mode  string
		loops int
	)
	flag.StringVar(&addr, "addr", "tcp://:5000", "listen address")
	flag.StringVar(&mode, "mode", "tcp", "tcp|http|ws|tls|wss")
	flag.IntVar(&loops, "loops", -1, "event loops")
	flag.Parse()

	h := &handler{mode: mode}
	var opts []gnet.Option
	if loops > 0 {
		opts = append(opts, gnet.WithMulticore(true), gnet.WithNumEventLoop(loops))
	}
	if mode == "tls" || mode == "wss" {
		log.Fatal("gnet server: tls/wss not supported in this harness")
	}

	log.Fatal(gnet.Run(h, addr, opts...))
}
