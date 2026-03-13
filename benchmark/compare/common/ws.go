package common

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func WSHandshakeResponse(key string) string {
	h := sha1.New()
	_, _ = h.Write([]byte(key))
	_, _ = h.Write([]byte(wsGUID))
	accept := base64.StdEncoding.EncodeToString(h.Sum(nil))
	return "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
}

// ReadWSHandshake reads until \r\n\r\n and extracts Sec-WebSocket-Key.
func ReadWSHandshake(r *bufio.Reader) (string, error) {
	var header strings.Builder
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		header.WriteString(line)
		if strings.HasSuffix(header.String(), "\r\n\r\n") {
			break
		}
	}
	for _, line := range strings.Split(header.String(), "\r\n") {
		if strings.HasPrefix(strings.ToLower(line), "sec-websocket-key:") {
			return strings.TrimSpace(strings.SplitN(line, ":", 2)[1]), nil
		}
	}
	return "", errors.New("missing sec-websocket-key")
}

// Minimal WS frame handling (single-frame, no fragmentation).
func ReadWSFrame(r io.Reader) ([]byte, byte, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, 0, err
	}
	fin := hdr[0] & 0x80
	opcode := hdr[0] & 0x0f
	if fin == 0 {
		return nil, 0, errors.New("fragmentation not supported")
	}
	mask := hdr[1] & 0x80
	ln := int(hdr[1] & 0x7f)
	if ln == 126 {
		var ext [2]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return nil, 0, err
		}
		ln = int(ext[0])<<8 | int(ext[1])
	} else if ln == 127 {
		return nil, 0, errors.New("payload too large")
	}
	var maskingKey [4]byte
	if mask != 0 {
		if _, err := io.ReadFull(r, maskingKey[:]); err != nil {
			return nil, 0, err
		}
	}
	payload := make([]byte, ln)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, 0, err
	}
	if mask != 0 {
		for i := 0; i < ln; i++ {
			payload[i] ^= maskingKey[i%4]
		}
	}
	return payload, opcode, nil
}

func WriteWSFrame(w io.Writer, payload []byte, opcode byte) error {
	if len(payload) > 125 {
		return errors.New("payload too large for minimal frame")
	}
	b0 := byte(0x80 | (opcode & 0x0f))
	b1 := byte(len(payload))
	_, err := w.Write([]byte{b0, b1})
	if err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}

func DialWS(addr string, tls bool, host string) (net.Conn, error) {
	if host == "" {
		host = addr
	}
	if tls {
		return nil, fmt.Errorf("tls ws dial not implemented here")
	}
	return net.Dial("tcp", addr)
}
