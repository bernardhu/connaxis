package evhandler

import "testing"

func TestSniffProtoByFirstLine(t *testing.T) {
	tests := []struct {
		name   string
		in     []byte
		want   connMode
		expect int
	}{
		{
			name:   "empty",
			in:     nil,
			want:   connModeUnknown,
			expect: 1,
		},
		{
			name:   "partial http method prefix",
			in:     []byte("GE"),
			want:   connModeUnknown,
			expect: 3,
		},
		{
			name:   "http request line",
			in:     []byte("GET /ws HTTP/1.1\r\n"),
			want:   connModeHTTP,
			expect: 0,
		},
		{
			name:   "unknown method",
			in:     []byte("HELLO /x\r\n"),
			want:   connModeTCP,
			expect: 0,
		},
		{
			name:   "binary tcp payload",
			in:     []byte{0x01, 0x02, 0x03},
			want:   connModeTCP,
			expect: 0,
		},
		{
			name:   "long token no space",
			in:     []byte("ABCDEFGHIJKLMNOP"),
			want:   connModeTCP,
			expect: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMode, gotExpect := sniffProtoByFirstLine(tt.in)
			if gotMode != tt.want {
				t.Fatalf("mode mismatch, got=%v want=%v", gotMode, tt.want)
			}
			if gotExpect != tt.expect {
				t.Fatalf("expect mismatch, got=%d want=%d", gotExpect, tt.expect)
			}
		})
	}
}
