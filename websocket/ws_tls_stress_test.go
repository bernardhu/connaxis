//go:build stress

package websocket

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bernardhu/connaxis"
	"github.com/bernardhu/connaxis/eventloop"
	gws "github.com/gorilla/websocket"
)

func stressEnvInt(t *testing.T, key string, def int) int {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		t.Fatalf("invalid %s=%q", key, v)
	}
	return n
}

func runConnaxisWSStress(t *testing.T, useTLS bool) {
	t.Helper()
	requireTCPListen(t)

	cfg := connaxis.GetDefaultConfig()
	cfg.Ncpu = stressEnvInt(t, "WS_STRESS_NCPU", 2)
	cfg.ListenAddrs = []eventloop.IEVEndpoint{
		&connaxis.EVEndpoint{Net: "tcp", Address: reserveTCPListenAddr(t)},
	}
	if useTLS {
		certPath, keyPath := writeSelfSignedCertFiles(t)
		cfg.SslMode = "tls"
		cfg.SslPem = certPath
		cfg.SslKey = keyPath
	}

	h := &wsServerHandler{}
	h.SetUserDataHandler(&wsUserEchoHandler{})

	serveErr, srv := connaxis.ServeByConfig(h, cfg, false)
	if serveErr != nil {
		t.Fatalf("serve: %v", serveErr)
	}
	defer srv.Stop()

	addr := srv.GetListenAddrs()[0].String()
	scheme := "ws"
	if useTLS {
		scheme = "wss"
	}
	u := url.URL{Scheme: scheme, Host: addr, Path: "/"}

	clients := stressEnvInt(t, "WS_STRESS_CLIENTS", 40)
	messagesPerClient := stressEnvInt(t, "WS_STRESS_MSGS", 80)
	ioTimeout := time.Duration(stressEnvInt(t, "WS_STRESS_IO_TIMEOUT_SEC", 5)) * time.Second
	totalTimeout := time.Duration(stressEnvInt(t, "WS_STRESS_TIMEOUT_SEC", 60)) * time.Second

	var (
		wg        sync.WaitGroup
		closeOnce sync.Once
		errOnce   sync.Once
		firstErr  error
		totalMsgs int64
	)
	stopCh := make(chan struct{})
	setErr := func(err error) {
		if err == nil {
			return
		}
		errOnce.Do(func() {
			firstErr = err
			closeOnce.Do(func() { close(stopCh) })
		})
	}

	for i := 0; i < clients; i++ {
		clientID := i
		wg.Add(1)
		go func() {
			defer wg.Done()

			select {
			case <-stopCh:
				return
			default:
			}

			dialer := gws.Dialer{HandshakeTimeout: ioTimeout}
			if useTLS {
				dialer.TLSClientConfig = &tls.Config{
					InsecureSkipVerify: true,
					MinVersion:         tls.VersionTLS12,
				}
			}

			c, resp, err := dialer.Dial(u.String(), nil)
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			if err != nil {
				setErr(fmt.Errorf("dial client=%d: %w", clientID, err))
				return
			}
			defer c.Close()

			for j := 0; j < messagesPerClient; j++ {
				select {
				case <-stopCh:
					return
				default:
				}

				payload := []byte(fmt.Sprintf("c%03d-m%03d", clientID, j))
				_ = c.SetWriteDeadline(time.Now().Add(ioTimeout))
				if err := c.WriteMessage(gws.TextMessage, payload); err != nil {
					setErr(fmt.Errorf("write client=%d msg=%d: %w", clientID, j, err))
					return
				}

				_ = c.SetReadDeadline(time.Now().Add(ioTimeout))
				msgType, msg, err := c.ReadMessage()
				if err != nil {
					setErr(fmt.Errorf("read client=%d msg=%d: %w", clientID, j, err))
					return
				}
				if msgType != gws.TextMessage {
					setErr(fmt.Errorf("unexpected message type client=%d msg=%d got=%d", clientID, j, msgType))
					return
				}
				if !bytes.Equal(msg, payload) {
					setErr(fmt.Errorf("payload mismatch client=%d msg=%d got=%q want=%q", clientID, j, msg, payload))
					return
				}
				atomic.AddInt64(&totalMsgs, 1)
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(totalTimeout):
		setErr(fmt.Errorf("stress run timed out after %s", totalTimeout))
		<-done
	}

	closeOnce.Do(func() { close(stopCh) })

	if firstErr != nil {
		t.Fatal(firstErr)
	}

	expected := int64(clients * messagesPerClient)
	got := atomic.LoadInt64(&totalMsgs)
	if got != expected {
		t.Fatalf("unexpected completed messages got=%d want=%d", got, expected)
	}
}

func TestConnaxisWSStressEcho(t *testing.T) {
	runConnaxisWSStress(t, false)
}

func TestConnaxisWSSStressEcho(t *testing.T) {
	runConnaxisWSStress(t, true)
}
