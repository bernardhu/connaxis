package connaxis

import (
	"encoding/base64"
	"encoding/hex"
	"os"
	"testing"
)

func TestParseSessionTicketKeyLine(t *testing.T) {
	raw := []byte("0123456789abcdef0123456789abcdef")
	tests := []struct {
		name string
		line []byte
		ok   bool
	}{
		{name: "raw", line: raw, ok: true},
		{name: "hex", line: []byte(hex.EncodeToString(raw)), ok: true},
		{name: "base64", line: []byte(base64.StdEncoding.EncodeToString(raw)), ok: true},
		{name: "comment", line: []byte("# comment"), ok: false},
		{name: "empty", line: []byte("  "), ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, ok, err := parseSessionTicketKeyLine(tt.line)
			if err != nil {
				t.Fatalf("parseSessionTicketKeyLine() error = %v", err)
			}
			if ok != tt.ok {
				t.Fatalf("parseSessionTicketKeyLine() ok = %t, want %t", ok, tt.ok)
			}
			if ok && string(key[:]) != string(raw) {
				t.Fatalf("parseSessionTicketKeyLine() key = %q, want %q", key[:], raw)
			}
		})
	}
}

func TestParseSessionTicketKeyLineRejectsWrongLength(t *testing.T) {
	_, _, err := parseSessionTicketKeyLine([]byte("short"))
	if err == nil {
		t.Fatal("parseSessionTicketKeyLine() error = nil, want error")
	}
}

func TestLoadSessionTicketKeys(t *testing.T) {
	path := t.TempDir() + "/ticket.keys"
	content := []byte("# newest first\n0123456789abcdef0123456789abcdef\nabcdef0123456789abcdef0123456789\n")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	keys, err := loadSessionTicketKeys(path)
	if err != nil {
		t.Fatalf("loadSessionTicketKeys() error = %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("loadSessionTicketKeys() len = %d, want 2", len(keys))
	}
	if got := string(keys[0][:]); got != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("first key = %q", got)
	}
}
