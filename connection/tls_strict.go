package connection

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/bernardhu/connaxis/internal/tls"
)

const (
	tlsRecordTypeAlert     = 21
	tlsRecordTypeHandshake = 22

	tlsHandshakeTypeClientHello = 1

	tlsAlertLevelFatal      = 2
	tlsAlertDecodeError     = 50
	tlsAlertProtocolVersion = 70
	tlsAlertMissingExt      = 109

	maxStrictClientHelloBytes = 1 << 20
)

type replayConn struct {
	net.Conn
	preRead []byte
}

func (c *replayConn) Read(p []byte) (int, error) {
	if len(c.preRead) > 0 {
		n := copy(p, c.preRead)
		c.preRead = c.preRead[n:]
		return n, nil
	}
	return c.Conn.Read(p)
}

type strictClientHelloInfo struct {
	legacyVersion     uint16
	offersTLS13       bool
	hasSupportedGroup bool
	hasKeyShare       bool
	hasPreSharedKey   bool
}

func parseUint16PairListExt(extData []byte, name string) error {
	if len(extData) < 2 {
		return fmt.Errorf("tls: malformed %s extension", name)
	}
	listLen := int(binary.BigEndian.Uint16(extData[:2]))
	if listLen != len(extData)-2 || listLen == 0 || listLen%2 != 0 {
		return fmt.Errorf("tls: malformed %s extension payload", name)
	}
	return nil
}

func shouldStrictTLS13Check(cfg *tls.Config) bool {
	if cfg == nil {
		return false
	}
	if cfg.MinVersion != tls.VersionTLS13 {
		return false
	}
	return cfg.MaxVersion == 0 || cfg.MaxVersion == tls.VersionTLS13
}

func maybeWrapStrictTLS13ServerConn(conn net.Conn, cfg *tls.Config) (net.Conn, error) {
	if !shouldStrictTLS13Check(cfg) {
		return conn, nil
	}

	recordBuf, helloMsg, recordVersion, err := readClientHelloRecords(conn, TlsHandshakeTimeout)
	if err != nil {
		sendFatalAlert(conn, recordVersion, tlsAlertDecodeError)
		return nil, err
	}

	helloInfo, err := parseClientHello(helloMsg)
	if err != nil {
		sendFatalAlert(conn, recordVersion, tlsAlertDecodeError)
		return nil, err
	}

	if helloInfo.legacyVersion <= 0x0300 {
		sendFatalAlert(conn, recordVersion, tlsAlertProtocolVersion)
		return nil, fmt.Errorf("tls: invalid ClientHello legacy_version: 0x%04x", helloInfo.legacyVersion)
	}

	if helloInfo.offersTLS13 {
		if helloInfo.hasSupportedGroup != helloInfo.hasKeyShare ||
			(!helloInfo.hasPreSharedKey && (!helloInfo.hasSupportedGroup || !helloInfo.hasKeyShare)) {
			sendFatalAlert(conn, recordVersion, tlsAlertMissingExt)
			return nil, errors.New("tls: client hello missing required tls1.3 key_share/supported_groups extensions")
		}
	}

	return &replayConn{
		Conn:    conn,
		preRead: recordBuf,
	}, nil
}

func readClientHelloRecords(conn net.Conn, timeout time.Duration) (recordBuf []byte, helloMsg []byte, recordVersion uint16, err error) {
	if timeout > 0 {
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
		defer func() {
			_ = conn.SetReadDeadline(time.Time{})
		}()
	}

	recordBuf = make([]byte, 0, 4096)
	handshakeBytes := make([]byte, 0, 4096)
	msgLen := -1

	for {
		hdr := make([]byte, 5)
		if _, err = io.ReadFull(conn, hdr); err != nil {
			return nil, nil, recordVersion, err
		}
		typ := hdr[0]
		vers := binary.BigEndian.Uint16(hdr[1:3])
		if recordVersion == 0 {
			recordVersion = vers
		}
		recLen := int(binary.BigEndian.Uint16(hdr[3:5]))
		if recLen <= 0 {
			return nil, nil, recordVersion, errors.New("tls: invalid tls record length in strict handshake precheck")
		}
		rec := make([]byte, recLen)
		if _, err = io.ReadFull(conn, rec); err != nil {
			return nil, nil, recordVersion, err
		}

		recordBuf = append(recordBuf, hdr...)
		recordBuf = append(recordBuf, rec...)
		if len(recordBuf) > maxStrictClientHelloBytes {
			return nil, nil, recordVersion, errors.New("tls: client hello precheck exceeded max buffered size")
		}

		if typ != tlsRecordTypeHandshake {
			return nil, nil, recordVersion, fmt.Errorf("tls: unexpected first record type %d in strict handshake precheck", typ)
		}

		handshakeBytes = append(handshakeBytes, rec...)
		if msgLen < 0 && len(handshakeBytes) >= 4 {
			if handshakeBytes[0] != tlsHandshakeTypeClientHello {
				return nil, nil, recordVersion, fmt.Errorf("tls: first handshake message is not client_hello (%d)", handshakeBytes[0])
			}
			msgLen = int(handshakeBytes[1])<<16 | int(handshakeBytes[2])<<8 | int(handshakeBytes[3])
			if msgLen <= 0 || msgLen > maxStrictClientHelloBytes {
				return nil, nil, recordVersion, errors.New("tls: invalid client_hello length in strict handshake precheck")
			}
		}

		if msgLen >= 0 && len(handshakeBytes) >= 4+msgLen {
			return recordBuf, handshakeBytes[:4+msgLen], recordVersion, nil
		}
	}
}

func parseClientHello(msg []byte) (strictClientHelloInfo, error) {
	var out strictClientHelloInfo
	if len(msg) < 4 || msg[0] != tlsHandshakeTypeClientHello {
		return out, errors.New("tls: malformed client_hello handshake message")
	}

	body := msg[4:]
	if len(body) < 2+32+1 {
		return out, errors.New("tls: short client_hello body")
	}

	off := 0
	out.legacyVersion = binary.BigEndian.Uint16(body[off : off+2])
	off += 2 + 32

	sessionIDLen := int(body[off])
	off++
	if off+sessionIDLen > len(body) {
		return out, errors.New("tls: malformed client_hello session id")
	}
	off += sessionIDLen

	if off+2 > len(body) {
		return out, errors.New("tls: malformed client_hello cipher suites length")
	}
	cipherSuitesLen := int(binary.BigEndian.Uint16(body[off : off+2]))
	off += 2
	if cipherSuitesLen <= 0 || cipherSuitesLen%2 != 0 || off+cipherSuitesLen > len(body) {
		return out, errors.New("tls: malformed client_hello cipher suites")
	}
	off += cipherSuitesLen

	if off+1 > len(body) {
		return out, errors.New("tls: malformed client_hello compression methods length")
	}
	compressionLen := int(body[off])
	off++
	if compressionLen <= 0 || off+compressionLen > len(body) {
		return out, errors.New("tls: malformed client_hello compression methods")
	}
	off += compressionLen

	if off == len(body) {
		return out, nil
	}
	if off+2 > len(body) {
		return out, errors.New("tls: malformed client_hello extensions length")
	}

	extTotalLen := int(binary.BigEndian.Uint16(body[off : off+2]))
	off += 2
	if off+extTotalLen != len(body) {
		return out, errors.New("tls: malformed client_hello extensions")
	}

	for off < len(body) {
		if off+4 > len(body) {
			return out, errors.New("tls: malformed client_hello extension header")
		}
		extType := binary.BigEndian.Uint16(body[off : off+2])
		extLen := int(binary.BigEndian.Uint16(body[off+2 : off+4]))
		off += 4
		if off+extLen > len(body) {
			return out, errors.New("tls: malformed client_hello extension body")
		}
		extData := body[off : off+extLen]
		off += extLen

		switch extType {
		case 43: // supported_versions
			if len(extData) < 1 {
				return out, errors.New("tls: malformed supported_versions extension")
			}
			versionsLen := int(extData[0])
			if versionsLen != len(extData)-1 || versionsLen%2 != 0 {
				return out, errors.New("tls: malformed supported_versions extension payload")
			}
			for i := 1; i+1 < len(extData); i += 2 {
				if binary.BigEndian.Uint16(extData[i:i+2]) == tls.VersionTLS13 {
					out.offersTLS13 = true
					break
				}
			}
		case 10: // supported_groups
			out.hasSupportedGroup = true
		case 13: // signature_algorithms
			if err := parseUint16PairListExt(extData, "signature_algorithms"); err != nil {
				return out, err
			}
		case 50: // signature_algorithms_cert
			if err := parseUint16PairListExt(extData, "signature_algorithms_cert"); err != nil {
				return out, err
			}
		case 51: // key_share
			if len(extData) < 2 {
				return out, errors.New("tls: malformed key_share extension")
			}
			keyShareLen := int(binary.BigEndian.Uint16(extData[:2]))
			if keyShareLen != len(extData)-2 {
				return out, errors.New("tls: malformed key_share extension payload")
			}
			// TLS 1.3 clients may intentionally send an empty key_share list to
			// trigger HelloRetryRequest (e.g. tlsfuzzer HRR/unknown-groups cases).
			// Presence of the extension is enough for our strict precheck.
			out.hasKeyShare = true
		case 41: // pre_shared_key
			out.hasPreSharedKey = true
		}
	}

	return out, nil
}

func sendFatalAlert(conn net.Conn, vers uint16, desc byte) {
	if vers < tls.VersionTLS10 || vers > tls.VersionTLS13 {
		vers = tls.VersionTLS12
	}
	record := []byte{
		tlsRecordTypeAlert,
		byte(vers >> 8), byte(vers),
		0, 2,
		tlsAlertLevelFatal,
		desc,
	}
	_, _ = conn.Write(record)
}
