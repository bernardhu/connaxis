package main

import (
	"bufio"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bernardhu/connaxis/benchmark/compare/common"
)

type histogram struct {
	buckets []int64
	step    time.Duration
}

func newHistogram(step time.Duration, n int) *histogram {
	return &histogram{buckets: make([]int64, n), step: step}
}

func (h *histogram) observe(d time.Duration) {
	idx := int(d / h.step)
	if idx >= len(h.buckets) {
		idx = len(h.buckets) - 1
	}
	atomic.AddInt64(&h.buckets[idx], 1)
}

func (h *histogram) percentile(p float64) time.Duration {
	total := int64(0)
	for i := range h.buckets {
		total += atomic.LoadInt64(&h.buckets[i])
	}
	if total == 0 {
		return 0
	}
	goal := int64(float64(total) * p)
	running := int64(0)
	for i := range h.buckets {
		running += atomic.LoadInt64(&h.buckets[i])
		if running >= goal {
			return time.Duration(i) * h.step
		}
	}
	return time.Duration(len(h.buckets)-1) * h.step
}

func main() {
	var (
		addr      string
		mode      string
		conns     int
		payload   int
		duration  time.Duration
		useTLS    bool
		readDelay time.Duration
		tlsVersion string
	)
	flag.StringVar(&addr, "addr", "127.0.0.1:5000", "server address")
	flag.StringVar(&mode, "mode", "tcp", "tcp|http|ws|tls|wss")
	flag.IntVar(&conns, "c", 100, "concurrent connections")
	flag.IntVar(&payload, "payload", 64, "payload size")
	flag.DurationVar(&duration, "d", 30*time.Second, "test duration")
	flag.BoolVar(&useTLS, "tls", false, "use TLS (overrides mode for tcp/ws)")
	flag.DurationVar(&readDelay, "read-delay", 0, "delay before reading response")
	flag.StringVar(&tlsVersion, "tls-version", "", "tls version for tls/wss: 1.2|1.3")
	flag.Parse()

	if mode == "tls" || mode == "wss" {
		useTLS = true
	}

	payloadBuf := make([]byte, payload)
	_, _ = rand.Read(payloadBuf)

	var total int64
	h := newHistogram(100*time.Microsecond, 2000) // 200ms max

	start := time.Now()
	stop := start.Add(duration)

	wg := sync.WaitGroup{}
	wg.Add(conns)

	for i := 0; i < conns; i++ {
		go func() {
			defer wg.Done()
			switch mode {
			case "tcp", "tls":
				tcpLoop(addr, payloadBuf, useTLS, tlsVersion, readDelay, stop, &total, h)
			case "http":
				httpLoop(addr, readDelay, stop, &total, h)
			case "ws", "wss":
				wsLoop(addr, payloadBuf, useTLS, tlsVersion, readDelay, stop, &total, h)
			default:
				return
			}
		}()
	}

	wg.Wait()

	dur := time.Since(start)
	fmt.Printf("duration=%s total=%d qps=%.2f p50=%s p95=%s p99=%s\n",
		dur,
		total,
		float64(total)/dur.Seconds(),
		h.percentile(0.50),
		h.percentile(0.95),
		h.percentile(0.99),
	)
}

func dial(addr string, tlsEnabled bool, tlsVersion string) (net.Conn, error) {
	if !tlsEnabled {
		return net.Dial("tcp", addr)
	}
	cfg := &tls.Config{InsecureSkipVerify: true}
	switch tlsVersion {
	case "1.2", "tls1.2":
		cfg.MinVersion = tls.VersionTLS12
		cfg.MaxVersion = tls.VersionTLS12
	case "1.3", "tls1.3":
		cfg.MinVersion = tls.VersionTLS13
		cfg.MaxVersion = tls.VersionTLS13
	}
	return tls.Dial("tcp", addr, cfg)
}

func tcpLoop(addr string, payload []byte, tlsEnabled bool, tlsVersion string, readDelay time.Duration, stop time.Time, total *int64, h *histogram) {
	conn, err := dial(addr, tlsEnabled, tlsVersion)
	if err != nil {
		return
	}
	defer conn.Close()
	if err := conn.SetDeadline(stop); err != nil {
		return
	}

	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)

	for time.Now().Before(stop) {
		start := time.Now()
		if _, err := w.Write(payload); err != nil {
			return
		}
		if err := w.Flush(); err != nil {
			return
		}

		if readDelay > 0 {
			time.Sleep(readDelay)
		}

		// read response
		buf := make([]byte, len(payload))
		if _, err := io.ReadFull(r, buf); err != nil {
			return
		}

		atomic.AddInt64(total, 1)
		h.observe(time.Since(start))
	}
}

func httpLoop(addr string, readDelay time.Duration, stop time.Time, total *int64, h *histogram) {
	conn, err := dial(addr, false, "")
	if err != nil {
		return
	}
	defer conn.Close()
	if err := conn.SetDeadline(stop); err != nil {
		return
	}

	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	req := "GET / HTTP/1.1\r\nHost: test\r\nConnection: keep-alive\r\n\r\n"

	for time.Now().Before(stop) {
		start := time.Now()
		if _, err := w.WriteString(req); err != nil {
			return
		}
		if err := w.Flush(); err != nil {
			return
		}
		if readDelay > 0 {
			time.Sleep(readDelay)
		}
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		if line == "" {
			return
		}
		// consume headers
		for {
			l, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if l == "\r\n" {
				break
			}
		}
		// body 2 bytes
		if _, err := io.ReadFull(r, make([]byte, 2)); err != nil {
			return
		}
		atomic.AddInt64(total, 1)
		h.observe(time.Since(start))
	}
}

func wsLoop(addr string, payload []byte, tlsEnabled bool, tlsVersion string, readDelay time.Duration, stop time.Time, total *int64, h *histogram) {
	conn, err := dial(addr, tlsEnabled, tlsVersion)
	if err != nil {
		return
	}
	defer conn.Close()
	if err := conn.SetDeadline(stop); err != nil {
		return
	}

	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	key := make([]byte, 16)
	_, _ = rand.Read(key)
	secKey := base64.StdEncoding.EncodeToString(key)

	req := "GET / HTTP/1.1\r\n" +
		"Host: test\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: " + secKey + "\r\n\r\n"

	if _, err := w.WriteString(req); err != nil {
		return
	}
	if err := w.Flush(); err != nil {
		return
	}
	// read handshake response
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		if line == "\r\n" {
			break
		}
	}

	for time.Now().Before(stop) {
		start := time.Now()
		if err := writeClientFrame(w, payload); err != nil {
			return
		}
		if err := w.Flush(); err != nil {
			return
		}

		if readDelay > 0 {
			time.Sleep(readDelay)
		}

		if _, _, err := common.ReadWSFrame(r); err != nil {
			return
		}
		atomic.AddInt64(total, 1)
		h.observe(time.Since(start))
	}
}

func writeClientFrame(w io.Writer, payload []byte) error {
	if len(payload) > 125 {
		payload = payload[:125]
	}
	maskKey := make([]byte, 4)
	_, _ = rand.Read(maskKey)
	b0 := byte(0x81) // fin + text
	b1 := byte(0x80 | len(payload))
	if _, err := w.Write([]byte{b0, b1}); err != nil {
		return err
	}
	if _, err := w.Write(maskKey); err != nil {
		return err
	}
	masked := make([]byte, len(payload))
	for i := 0; i < len(payload); i++ {
		masked[i] = payload[i] ^ maskKey[i%4]
	}
	_, err := w.Write(masked)
	return err
}
