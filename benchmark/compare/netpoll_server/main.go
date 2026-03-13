//go:build netpoll

package main

import (
	"context"
	"flag"
	"log"
	"strings"
	"sync"

	"github.com/bernardhu/connaxis/benchmark/compare/common"
	"github.com/cloudwego/netpoll"
)

type modeState struct {
	mode       string
	wsUpgraded bool
}

func main() {
	var (
		addr string
		mode string
	)
	flag.StringVar(&addr, "addr", ":5000", "listen address")
	flag.StringVar(&mode, "mode", "tcp", "tcp|http|ws")
	flag.Parse()

	if mode == "tls" || mode == "wss" {
		log.Fatal("netpoll server: tls/wss not supported in this harness")
	}

	ln, err := netpoll.CreateListener("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}

	var state sync.Map // key: netpoll.Connection, value: *modeState

	h := func(ctx context.Context, conn netpoll.Connection) error {
		itf, ok := state.Load(conn)
		var st *modeState
		if ok {
			st = itf.(*modeState)
		} else {
			st = &modeState{mode: mode}
			state.Store(conn, st)
			_ = conn.AddCloseCallback(func(c netpoll.Connection) error {
				state.Delete(c)
				return nil
			})
		}

		r := conn.Reader()
		w := conn.Writer()
		defer func() {
			_ = r.Release()
		}()

		switch st.mode {
		case "tcp":
			buf, _ := r.Peek(r.Len())
			if len(buf) == 0 {
				return nil
			}
			_ = r.Skip(len(buf))
			_, _ = w.WriteBinary(buf)
			return w.Flush()
		case "http":
			buf, _ := r.Peek(r.Len())
			if !common.ParseHTTPRequest(buf) {
				return nil
			}
			idx := strings.Index(string(buf), "\r\n\r\n")
			if idx >= 0 {
				_ = r.Skip(idx + 4)
			}
			_, _ = w.WriteBinary(common.HTTPResponse())
			return w.Flush()
		case "ws":
			buf, _ := r.Peek(r.Len())
			if !st.wsUpgraded {
				idx := strings.Index(string(buf), "\r\n\r\n")
				if idx < 0 {
					return nil
				}
				key := ""
				for _, line := range strings.Split(string(buf[:idx+4]), "\r\n") {
					if strings.HasPrefix(strings.ToLower(line), "sec-websocket-key:") {
						key = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
						break
					}
				}
				if key == "" {
					return netpoll.ErrConnClosed
				}
				st.wsUpgraded = true
				_ = r.Skip(idx + 4)
				_, _ = w.WriteBinary([]byte(common.WSHandshakeResponse(key)))
				return w.Flush()
			}
			payload, frameLen, ok := parseWSFrame(buf)
			if !ok {
				return nil
			}
			_ = r.Skip(frameLen)
			out := make([]byte, 0, len(payload)+2)
			out = append(out, 0x81, byte(len(payload)))
			out = append(out, payload...)
			_, _ = w.WriteBinary(out)
			return w.Flush()
		}
		return nil
	}

	eventLoop, err := netpoll.NewEventLoop(h)
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(eventLoop.Serve(ln))
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
