package dispatch

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
	"github.com/Tangerg/lynx/app/runtime/protocol"
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

// TestDecodeReportsFieldLevelConstraintViolations pins the two facts a request
// constraint has to deliver: the failure is invalid_params, and it names the
// offending params keys in ProblemData.errors (API.md §8.3) rather than only in
// a prose detail the client would have to parse.
func TestDecodeReportsFieldLevelConstraintViolations(t *testing.T) {
	t.Parallel()

	msg := &transport.Request{Method: "sessions.update", Params: json.RawMessage(`{"sessionId":"","expectedRevision":0}`)}
	_, bad := decode[protocol.UpdateSessionRequest](msg)
	if bad == nil {
		t.Fatal("decode accepted a request with an empty id and a zero revision")
	}
	if bad.Code != codeInvalidParams {
		t.Fatalf("code = %d, want %d", bad.Code, codeInvalidParams)
	}
	var problem protocol.ProblemData
	if err := json.Unmarshal(bad.Data, &problem); err != nil {
		t.Fatalf("decode problem data: %v", err)
	}
	if problem.Type != "invalid_params" {
		t.Fatalf("type = %q, want invalid_params", problem.Type)
	}
	fields := make([]string, 0, len(problem.Errors))
	for _, f := range problem.Errors {
		if f.Detail == "" {
			t.Errorf("field %q carries no detail", f.Field)
		}
		fields = append(fields, f.Field)
	}
	if !slices.Equal(fields, []string{"sessionId", "expectedRevision"}) {
		t.Fatalf("errors fields = %v, want [sessionId expectedRevision]", fields)
	}
}

func TestDecodeRequiredAndOptionalArrayConstraints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		params     string
		decode     func(*transport.Request) *transport.Error
		wantField  string
		wantDetail string
	}{
		{
			name:   "required array omitted",
			params: `{"runId":"run_1","expectedSegmentId":"seg_1"}`,
			decode: func(msg *transport.Request) *transport.Error {
				_, bad := decode[protocol.SteerRunRequest](msg)
				return bad
			},
			wantField:  "input",
			wantDetail: "is required",
		},
		{
			name:   "required array empty",
			params: `{"runId":"run_1","expectedSegmentId":"seg_1","input":[]}`,
			decode: func(msg *transport.Request) *transport.Error {
				_, bad := decode[protocol.SteerRunRequest](msg)
				return bad
			},
			wantField:  "input",
			wantDetail: "must not be empty",
		},
		{
			name:   "optional array omitted",
			params: `{"runId":"run_1","responses":[]}`,
			decode: func(msg *transport.Request) *transport.Error {
				_, bad := decode[protocol.ResumeRunRequest](msg)
				return bad
			},
		},
		{
			name:   "optional array empty",
			params: `{"runId":"run_1","responses":[],"input":[]}`,
			decode: func(msg *transport.Request) *transport.Error {
				_, bad := decode[protocol.ResumeRunRequest](msg)
				return bad
			},
			wantField:  "input",
			wantDetail: "must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			bad := tt.decode(&transport.Request{Params: json.RawMessage(tt.params)})
			if tt.wantField == "" {
				if bad != nil {
					t.Fatalf("decode rejected valid params: %+v", bad)
				}
				return
			}
			if bad == nil {
				t.Fatal("decode accepted invalid params")
			}
			var problem protocol.ProblemData
			if err := json.Unmarshal(bad.Data, &problem); err != nil {
				t.Fatalf("decode problem data: %v", err)
			}
			if !slices.Equal(problem.Errors, []protocol.FieldError{{
				Field: tt.wantField, Detail: tt.wantDetail,
			}}) {
				t.Fatalf("errors = %+v, want %s: %s", problem.Errors, tt.wantField, tt.wantDetail)
			}
		})
	}
}

// TestDecodeAcceptsRequestsWithoutConstraints keeps the constraint hook from
// becoming a gate on every method: a request type that declares nothing must
// decode unchanged.
func TestDecodeAcceptsRequestsWithoutConstraints(t *testing.T) {
	t.Parallel()

	msg := &transport.Request{Method: "runs.list", Params: json.RawMessage(`{"sessionId":"ses_1"}`)}
	in, bad := decode[protocol.ListRunsRequest](msg)
	if bad != nil {
		t.Fatalf("decode rejected an unconstrained request: %+v", bad)
	}
	if in.SessionID != "ses_1" {
		t.Fatalf("sessionId = %q, want ses_1", in.SessionID)
	}
}
