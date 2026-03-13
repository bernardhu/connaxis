//go:build linux

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"flag"
	"fmt"
	"math/big"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/internal"
	"github.com/bernardhu/connaxis/internal/tls"
	"github.com/bernardhu/connaxis/pool"
	"golang.org/x/sys/unix"
)

const handshakeTimeout = 10 * time.Second

func main() {
	var (
		benchEnabled = flag.Bool("bench", true, "run performance test")
		benchBytes   = flag.Int("bench-bytes", 64<<20, "throughput test total bytes")
		benchChunk   = flag.Int("bench-chunk", 32<<10, "throughput write chunk size")
		benchIters   = flag.Int("bench-iters", 10000, "latency test iterations")
		benchMsg     = flag.Int("bench-msg", 64, "latency test message size")
		payloadSize  = flag.Int("payload", 32*1024+7, "initial payload size")
	)
	flag.Parse()
	pool.Setup(16)
	fmt.Println("== kTLS full flow check ==")
	fmt.Printf("kernel: %s\n", kernelRelease())

	if _, err := os.Stat("/sys/module/tls"); err == nil {
		fmt.Println("module: /sys/module/tls present")
	} else {
		fmt.Printf("module: /sys/module/tls not present (%v)\n", err)
	}

	if ulp, err := os.ReadFile("/proc/sys/net/ipv4/tcp_available_ulp"); err == nil {
		s := strings.TrimSpace(string(ulp))
		fmt.Printf("tcp_available_ulp: %q\n", s)
	} else {
		fmt.Printf("tcp_available_ulp: unreadable (%v)\n", err)
	}

	if ok, err := internal.SystemSupportKTLS(); ok {
		fmt.Println("SystemSupportKTLS: yes")
	} else {
		if err == unix.ENOTCONN {
			fmt.Printf("SystemSupportKTLS: warn (%v)\n", err)
		} else {
			fmt.Printf("SystemSupportKTLS: no (%v)\n", err)
		}
	}

	srvCfg, cliCfg, err := buildTLSConfigs()
	if err != nil {
		fatalf("tls config: %v", err)
	}

	connection.SetTLSEngine(connection.TLSEngineKTLS)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fatalf("listen: %v", err)
	}
	defer ln.Close()

	payload := bytes.Repeat([]byte("a"), *payloadSize)
	reply := bytes.Repeat([]byte("b"), *payloadSize)

	benchCfg := benchConfig{
		enabled: *benchEnabled,
		bytes:   *benchBytes,
		chunk:   *benchChunk,
		iters:   *benchIters,
		msgSize: *benchMsg,
	}

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- runServer(ln, srvCfg, payload, reply, benchCfg)
	}()

	if err := runClient(ln.Addr().String(), cliCfg, payload, reply, benchCfg); err != nil {
		fatalf("client: %v", err)
	}
	if err := <-serverErr; err != nil {
		fatalf("server: %v", err)
	}

	fmt.Println("kTLS end-to-end: OK (ULP attach + crypto_info inject)")
}

type benchConfig struct {
	enabled bool
	bytes   int
	chunk   int
	iters   int
	msgSize int
}

func runServer(ln net.Listener, cfg *tls.Config, payload, reply []byte, bench benchConfig) error {
	conn, err := ln.Accept()
	if err != nil {
		return err
	}
	tcp, ok := conn.(*net.TCPConn)
	if !ok {
		_ = conn.Close()
		return fmt.Errorf("accept: not TCPConn")
	}

	fd, err := dupFD(tcp)
	_ = conn.Close()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), handshakeTimeout)
	defer cancel()

	c, err := connection.NewTLSConnServer(ctx, fd, cfg)
	if err != nil {
		return err
	}
	defer c.Close()

	if err := checkULP("server", c.Fd()); err != nil {
		return err
	}

	buf := make([]byte, len(payload))
	if err := readFull(c, buf); err != nil {
		return err
	}
	if !bytes.Equal(buf, payload) {
		return fmt.Errorf("payload mismatch")
	}

	if err := writeAll(c, reply); err != nil {
		return err
	}

	if bench.enabled {
		if err := serverThroughput(c, bench); err != nil {
			return err
		}
		if err := serverLatency(c, bench); err != nil {
			return err
		}
	}

	return nil
}

func runClient(addr string, cfg *tls.Config, payload, reply []byte, bench benchConfig) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	tcp, ok := conn.(*net.TCPConn)
	if !ok {
		_ = conn.Close()
		return fmt.Errorf("dial: not TCPConn")
	}

	fd, err := dupFD(tcp)
	_ = conn.Close()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), handshakeTimeout)
	defer cancel()

	c, err := connection.NewTLSConnClient(ctx, fd, cfg)
	if err != nil {
		return err
	}
	defer c.Close()

	if err := checkULP("client", c.Fd()); err != nil {
		return err
	}

	if err := writeAll(c, payload); err != nil {
		return err
	}

	buf := make([]byte, len(reply))
	if err := readFull(c, buf); err != nil {
		return err
	}
	if !bytes.Equal(buf, reply) {
		return fmt.Errorf("reply mismatch")
	}

	if bench.enabled {
		if err := clientThroughput(c, bench); err != nil {
			return err
		}
		if err := clientLatency(c, bench); err != nil {
			return err
		}
	}
	return nil
}

func buildTLSConfigs() (*tls.Config, *tls.Config, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber: serial,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}

	cert := tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
	}

	cipherSuites := []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}

	serverCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		MaxVersion:   tls.VersionTLS12,
		CipherSuites: cipherSuites,
	}

	clientCfg := &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
		MaxVersion:         tls.VersionTLS12,
		CipherSuites:       cipherSuites,
	}

	return serverCfg, clientCfg, nil
}

func checkULP(label string, fd int) error {
	ulp, err := unix.GetsockoptString(fd, unix.IPPROTO_TCP, unix.TCP_ULP)
	if err != nil {
		return fmt.Errorf("%s TCP_ULP: %v", label, err)
	}
	ulp = normalizeSockoptString(ulp)
	fmt.Printf("%s TCP_ULP: %q\n", label, ulp)
	if ulp != "tls" {
		return fmt.Errorf("%s TCP_ULP not tls", label)
	}
	return nil
}

func serverThroughput(c connection.EngineConn, bench benchConfig) error {
	if bench.bytes <= 0 || bench.chunk <= 0 {
		return nil
	}
	buf := make([]byte, bench.chunk)
	return readN(c, bench.bytes, buf)
}

func clientThroughput(c connection.EngineConn, bench benchConfig) error {
	if bench.bytes <= 0 || bench.chunk <= 0 {
		return nil
	}
	chunk := bytes.Repeat([]byte("t"), bench.chunk)
	start := time.Now()
	if err := writeN(c, bench.bytes, chunk); err != nil {
		return err
	}
	elapsed := time.Since(start)
	mbps := (float64(bench.bytes) / (1024 * 1024)) / elapsed.Seconds()
	fmt.Printf("throughput: %.2f MB/s (%d bytes in %v)\n", mbps, bench.bytes, elapsed)
	return nil
}

func serverLatency(c connection.EngineConn, bench benchConfig) error {
	if bench.iters <= 0 || bench.msgSize <= 0 {
		return nil
	}
	buf := make([]byte, bench.msgSize)
	for i := 0; i < bench.iters; i++ {
		if err := readFull(c, buf); err != nil {
			return err
		}
		if err := writeAll(c, buf); err != nil {
			return err
		}
	}
	return nil
}

func clientLatency(c connection.EngineConn, bench benchConfig) error {
	if bench.iters <= 0 || bench.msgSize <= 0 {
		return nil
	}
	msg := bytes.Repeat([]byte("l"), bench.msgSize)
	resp := make([]byte, bench.msgSize)
	lat := make([]time.Duration, 0, bench.iters)
	for i := 0; i < bench.iters; i++ {
		start := time.Now()
		if err := writeAll(c, msg); err != nil {
			return err
		}
		if err := readFull(c, resp); err != nil {
			return err
		}
		lat = append(lat, time.Since(start))
	}

	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	p50 := lat[len(lat)/2]
	p90 := lat[int(float64(len(lat))*0.90)]
	p99 := lat[int(float64(len(lat))*0.99)]
	var total time.Duration
	for _, v := range lat {
		total += v
	}
	avg := total / time.Duration(len(lat))
	fmt.Printf("latency: avg=%v p50=%v p90=%v p99=%v (iters=%d, msg=%d)\n", avg, p50, p90, p99, bench.iters, bench.msgSize)
	return nil
}

func writeAll(c connection.EngineConn, data []byte) error {
	return writeFully(c, data)
}

func writeN(c connection.EngineConn, total int, chunk []byte) error {
	remaining := total
	for remaining > 0 {
		size := len(chunk)
		if size > remaining {
			size = remaining
		}
		if err := writeFully(c, chunk[:size]); err != nil {
			return err
		}
		remaining -= size
	}
	return nil
}

func writeFully(c connection.EngineConn, data []byte) error {
	written := 0
	for written < len(data) {
		n, err := c.Write(data[written:])
		if err != nil {
			if err == unix.EAGAIN {
				time.Sleep(1 * time.Millisecond)
				continue
			}
			return err
		}
		if n > 0 {
			written += n
		}
		for c.PendingWrite() > 0 {
			nf, ferr := c.FlushN(len(data) - written)
			if ferr != nil {
				if ferr == unix.EAGAIN {
					time.Sleep(1 * time.Millisecond)
					continue
				}
				return ferr
			}
			if nf == 0 {
				return fmt.Errorf("flush stalled")
			}
		}
	}
	return nil
}

func readN(c connection.EngineConn, total int, buf []byte) error {
	remaining := total
	for remaining > 0 {
		toRead := len(buf)
		if toRead > remaining {
			toRead = remaining
		}
		n, err := c.Read(buf[:toRead])
		if err != nil {
			if err == unix.EAGAIN {
				time.Sleep(1 * time.Millisecond)
				continue
			}
			return err
		}
		if n == 0 {
			continue
		}
		remaining -= n
	}
	return nil
}

func readFull(c connection.EngineConn, dst []byte) error {
	off := 0
	for off < len(dst) {
		n, err := c.Read(dst[off:])
		if err != nil {
			if err == unix.EAGAIN {
				time.Sleep(1 * time.Millisecond)
				continue
			}
			return err
		}
		if n == 0 {
			continue
		}
		off += n
	}
	return nil
}

func dupFD(c *net.TCPConn) (int, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return 0, err
	}
	var (
		fd   int
		derr error
	)
	if err := raw.Control(func(s uintptr) {
		fd, derr = unix.Dup(int(s))
	}); err != nil {
		return 0, err
	}
	if derr != nil {
		return 0, derr
	}
	return fd, nil
}

func kernelRelease() string {
	var u unix.Utsname
	if err := unix.Uname(&u); err != nil {
		return "unknown"
	}
	return charsToString(u.Release[:])
}

func charsToString[T ~byte | ~int8](ca []T) string {
	n := 0
	for n < len(ca) && ca[n] != 0 {
		n++
	}
	buf := make([]byte, n)
	for i := 0; i < n; i++ {
		buf[i] = byte(ca[i])
	}
	return string(buf)
}

func normalizeSockoptString(s string) string {
	if idx := strings.IndexByte(s, 0); idx >= 0 {
		return s[:idx]
	}
	return s
}

func fatalf(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
	os.Exit(2)
}
