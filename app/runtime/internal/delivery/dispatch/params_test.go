package dispatch

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func TestDecodeParamsRejectsDriftedRequests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		raw    string
		detail string
	}{
		{name: "null", raw: `null`, detail: "must be an object"},
		{name: "unknown field", raw: `{"sessionId":"ses_1","input":[],"context":[]}`, detail: `unknown field "context"`},
		{name: "wrong type", raw: `{"sessionId":1,"input":[]}`, detail: "cannot unmarshal number"},
		{name: "multiple values", raw: `{"sessionId":"ses_1"} {}`, detail: "exactly one JSON object"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got protocol.StartRunRequest
			err := decodeParams(json.RawMessage(tt.raw), &got)
			if err == nil || !strings.Contains(err.Error(), tt.detail) {
				t.Fatalf("decodeParams() error = %v, want detail %q", err, tt.detail)
			}
		})
	}
}

func TestDecodeParamsAcceptsEmptyAndKnownFields(t *testing.T) {
	t.Parallel()

	var empty protocol.PageQuery
	if err := decodeParams(nil, &empty); err != nil {
		t.Fatalf("decode empty params: %v", err)
	}

	var start protocol.StartRunRequest
	if err := decodeParams(json.RawMessage(`{"sessionId":"ses_1","input":[{"type":"text","text":"hello"}]}`), &start); err != nil {
		t.Fatalf("decode known params: %v", err)
	}
	if start.SessionID != "ses_1" || len(start.Input) != 1 {
		t.Fatalf("decoded request = %+v", start)
	}
}
