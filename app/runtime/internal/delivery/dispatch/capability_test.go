package dispatch

import (
	"encoding/json"
	"testing"
)

func TestDecodeFrame(t *testing.T) {
	tests := []struct {
		name    string
		params  json.RawMessage
		wantLen int
		wantErr bool
	}{
		{name: "absent"},
		{name: "object", params: json.RawMessage(`{"watch":true}`), wantLen: 1},
		{name: "null", params: json.RawMessage(`null`), wantErr: true},
		{name: "malformed", params: json.RawMessage(`{`), wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			frame, err := decodeFrame(test.params)
			if (err != nil) != test.wantErr {
				t.Fatalf("decodeFrame error = %v, want error %v", err, test.wantErr)
			}
			if len(frame) != test.wantLen {
				t.Fatalf("decodeFrame length = %d, want %d", len(frame), test.wantLen)
			}
		})
	}
}
