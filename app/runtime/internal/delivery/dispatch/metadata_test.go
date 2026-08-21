package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
	"github.com/Tangerg/lynx/app/runtime/protocol"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

func testID(value string) transport.ID {
	id, _ := jsonrpc.MakeID(value)
	return id
}

func TestExtractRequestMetaStripsTransportMember(t *testing.T) {
	req := &transport.Request{
		ID:     testID("1"),
		Method: "runs.cancel",
		Params: json.RawMessage(fmt.Sprintf(`{
			"_meta": {
				"protocolVersion": %q,
				"clientInfo": { "name": "cli", "version": "0.1.0" },
				"clientCapabilities": {
					"features": {},
					"interruptTypes": ["approval"],
					"excludedEphemeralEvents": ["item.delta"]
				}
			},
			"runId": "run_1"
		}`, protocol.ProtocolVersion)),
	}

	meta, rpcErr := extractRequestMeta(req)
	if rpcErr != nil {
		t.Fatalf("extractRequestMeta error = %v", rpcErr)
	}
	if meta.ProtocolVersion != protocol.ProtocolVersion {
		t.Fatalf("protocolVersion = %q", meta.ProtocolVersion)
	}
	if meta.ClientInfo == nil || meta.ClientInfo.Name != "cli" {
		t.Fatalf("clientInfo = %+v", meta.ClientInfo)
	}
	if meta.ClientCapabilities == nil || len(meta.ClientCapabilities.InterruptTypes) != 1 {
		t.Fatalf("clientCapabilities = %+v", meta.ClientCapabilities)
	}
	if string(req.Params) != `{"runId":"run_1"}` {
		t.Fatalf("stripped params = %s", string(req.Params))
	}
}

func TestExtractRequestMetaRejectsMalformedMeta(t *testing.T) {
	req := &transport.Request{
		ID:     testID("1"),
		Method: "runs.cancel",
		Params: json.RawMessage(`{"_meta":"bad","runId":"run_1"}`),
	}

	_, rpcErr := extractRequestMeta(req)
	if rpcErr == nil {
		t.Fatalf("expected invalid params error")
	}
	if rpcErr.Code != codeInvalidParams {
		t.Fatalf("code = %d, want %d", rpcErr.Code, codeInvalidParams)
	}
}

func TestExtractRequestMetaRejectsNullMeta(t *testing.T) {
	req := &transport.Request{
		ID:     testID("1"),
		Method: "runs.cancel",
		Params: json.RawMessage(`{"_meta":null,"runId":"run_1"}`),
	}

	_, rpcErr := extractRequestMeta(req)
	if rpcErr == nil {
		t.Fatalf("expected invalid params error")
	}
	if rpcErr.Code != codeInvalidParams {
		t.Fatalf("code = %d, want %d", rpcErr.Code, codeInvalidParams)
	}
}

// TestBindRequestMetaRejectsUnsupportedProtocolVersion pins the cutover's refusal
// half: this build serves ONE version, and everything else is turned away with the
// same typed answer rather than served a best effort.
//
// "2026-07-19" is the version this runtime served until the cutover, and it is the
// case that matters — a client that still ships it must be told so, not quietly
// handed current frames it will fold as if they were the old shape. A far-past date
// alone would not prove that the predecessor is rejected.
func TestDispatchRejectsUnsupportedProtocolVersion(t *testing.T) {
	for _, version := range []string{
		"2026-07-19", // the version served before the current protocol cutover
		"2027-01-01", // a client newer than this build
		"1900-01-01",
		"not-a-date",
	} {
		t.Run(version, func(t *testing.T) {
			req := &transport.Request{
				ID:     testID("1"),
				Method: "runs.cancel",
				Params: json.RawMessage(fmt.Sprintf(`{"_meta":{"protocolVersion":%q},"runId":"run_1"}`, version)),
			}

			rpcErr := dispatchMetadataFailure(t, req)
			if rpcErr == nil {
				t.Fatalf("Dispatch accepted protocolVersion %q", version)
			}
			if rpcErr.Code != codeInvalidProtocolVersion {
				t.Fatalf("code = %d, want %d", rpcErr.Code, codeInvalidProtocolVersion)
			}
			// The refusal has to name the exact version: a client that only learns "no" cannot
			// tell an unsupported version from a malformed request.
			var problem struct {
				Type   string `json:"type"`
				Detail string `json:"detail"`
			}
			if err := json.Unmarshal(rpcErr.Data, &problem); err != nil {
				t.Fatalf("decode problem data: %v", err)
			}
			if problem.Type != protocol.ErrInvalidProtocolVersion.Error() {
				t.Errorf("problem type = %q, want %q", problem.Type, protocol.ErrInvalidProtocolVersion.Error())
			}
			if !strings.Contains(problem.Detail, protocol.ProtocolVersion) {
				t.Errorf("detail %q does not say which version this build serves", problem.Detail)
			}
		})
	}
}

func TestDispatchDoesNotMutateCallerRequestWhenStrippingMeta(t *testing.T) {
	req := &transport.Request{
		ID:     testID("1"),
		Method: "unknown.method",
		Params: json.RawMessage(fmt.Sprintf(`{"_meta":{"protocolVersion":%q},"value":1}`, protocol.ProtocolVersion)),
	}
	original := string(req.Params)
	New(newOperationEndpoint(t, nil)).Dispatch(context.Background(), req)
	if got := string(req.Params); got != original {
		t.Fatalf("Dispatch mutated caller params: got %s, want %s", got, original)
	}
}

// TestBindRequestMetaRefusesANonSuppressibleEvent pins §8.1: a client may suppress an
// ephemeral preview, never an authoritative event. Ignoring the illegal entry
// would leave the client believing it had opted out of a frame it will keep
// receiving — and a runtime that honored it would break the §5.2 guarantee that
// discarding every ephemeral event still converges.
func TestDispatchRefusesANonSuppressibleEvent(t *testing.T) {
	req := &transport.Request{
		ID:     testID("1"),
		Method: "runs.cancel",
		Params: json.RawMessage(`{
			"_meta": {
				"clientCapabilities": { "excludedEphemeralEvents": ["item.completed"] }
			},
			"runId": "run_1"
		}`),
	}

	rpcErr := dispatchMetadataFailure(t, req)
	if rpcErr == nil {
		t.Fatal("Dispatch accepted an event outside the closed opt-out set")
	}
	if rpcErr.Code != codeInvalidParams {
		t.Fatalf("error code = %d, want invalid_params (%d)", rpcErr.Code, codeInvalidParams)
	}
}

func TestDispatchValidatesMetadataWireShape(t *testing.T) {
	for _, test := range []struct {
		name  string
		meta  string
		field string
	}{
		{
			name:  "unknown interrupt type",
			meta:  `{"clientCapabilities":{"interruptTypes":["telepathy"]}}`,
			field: "clientCapabilities.interruptTypes[0]",
		},
		{
			name:  "duplicate event exclusion",
			meta:  `{"clientCapabilities":{"excludedEphemeralEvents":["item.delta","item.delta"]}}`,
			field: "clientCapabilities.excludedEphemeralEvents",
		},
		{
			name:  "empty client identity",
			meta:  `{"clientInfo":{"name":"","version":"1.0.0"}}`,
			field: "clientInfo.name",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := &transport.Request{
				ID:     testID("1"),
				Method: "runs.cancel",
				Params: json.RawMessage(`{"_meta":` + test.meta + `,"runId":"run_1"}`),
			}

			rpcErr := dispatchMetadataFailure(t, req)
			if rpcErr == nil || rpcErr.Code != codeInvalidParams {
				t.Fatalf("error = %+v, want invalid_params", rpcErr)
			}
			var problem protocol.ProblemData
			if err := json.Unmarshal(rpcErr.Data, &problem); err != nil {
				t.Fatalf("decode problem: %v", err)
			}
			for _, field := range problem.Errors {
				if field.Field == test.field {
					return
				}
			}
			t.Fatalf("problem errors = %+v, want field %q", problem.Errors, test.field)
		})
	}
}

func TestExtractRequestMetaRejectsUnknownFields(t *testing.T) {
	req := &transport.Request{
		ID:     testID("1"),
		Method: "runs.cancel",
		Params: json.RawMessage(`{"_meta":{"capabilities":{}},"runId":"run_1"}`),
	}

	_, rpcErr := extractRequestMeta(req)
	if rpcErr == nil || rpcErr.Code != codeInvalidParams {
		t.Fatalf("error = %+v, want invalid_params", rpcErr)
	}
	var problem protocol.ProblemData
	if err := json.Unmarshal(rpcErr.Data, &problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if !strings.Contains(problem.Detail, `unknown field "capabilities"`) {
		t.Fatalf("detail = %q, want unknown metadata field", problem.Detail)
	}
}

func dispatchMetadataFailure(t *testing.T, request *transport.Request) *transport.Error {
	t.Helper()
	result := New(newOperationEndpoint(t, nil)).Dispatch(t.Context(), request)
	if result.Response == nil {
		return nil
	}
	rpcError, _ := errors.AsType[*transport.Error](result.Response.Error)
	return rpcError
}
