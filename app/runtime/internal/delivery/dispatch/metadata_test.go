package dispatch

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

func TestBindRequestMetaStripsMetaAndStoresContext(t *testing.T) {
	req := &transport.Request{
		ID:     transport.StringID("1"),
		Method: MethodRunsCancel,
		Params: json.RawMessage(`{
			"_meta": {
				"protocolVersion": "2026-06-07",
				"clientInfo": { "name": "cli", "version": "0.1.0" },
				"clientCapabilities": {
					"events": ["segment.started"],
					"features": {},
					"interruptTypes": ["approval"]
				}
			},
			"runId": "run_1"
		}`),
	}

	ctx, rpcErr := bindRequestMeta(context.Background(), req)
	if rpcErr != nil {
		t.Fatalf("bindRequestMeta error = %v", rpcErr)
	}

	meta, ok := protocol.RequestMetaFrom(ctx)
	if !ok {
		t.Fatalf("request metadata missing from context")
	}
	if meta.ProtocolVersion != "2026-06-07" {
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

func TestBindRequestMetaRejectsMalformedMeta(t *testing.T) {
	req := &transport.Request{
		ID:     transport.StringID("1"),
		Method: MethodRunsCancel,
		Params: json.RawMessage(`{"_meta":"bad","runId":"run_1"}`),
	}

	_, rpcErr := bindRequestMeta(context.Background(), req)
	if rpcErr == nil {
		t.Fatalf("expected invalid params error")
	}
	if rpcErr.Code != transport.CodeInvalidParams {
		t.Fatalf("code = %d, want %d", rpcErr.Code, transport.CodeInvalidParams)
	}
}

func TestBindRequestMetaRejectsNullMeta(t *testing.T) {
	req := &transport.Request{
		ID:     transport.StringID("1"),
		Method: MethodRunsCancel,
		Params: json.RawMessage(`{"_meta":null,"runId":"run_1"}`),
	}

	_, rpcErr := bindRequestMeta(context.Background(), req)
	if rpcErr == nil {
		t.Fatalf("expected invalid params error")
	}
	if rpcErr.Code != transport.CodeInvalidParams {
		t.Fatalf("code = %d, want %d", rpcErr.Code, transport.CodeInvalidParams)
	}
}

func TestBindRequestMetaRejectsUnsupportedProtocolVersion(t *testing.T) {
	req := &transport.Request{
		ID:     transport.StringID("1"),
		Method: MethodRunsCancel,
		Params: json.RawMessage(`{"_meta":{"protocolVersion":"1900-01-01"},"runId":"run_1"}`),
	}

	_, rpcErr := bindRequestMeta(context.Background(), req)
	if rpcErr == nil {
		t.Fatalf("expected invalid protocol version error")
	}
	if rpcErr.Code != protocol.CodeInvalidProtocolVersion {
		t.Fatalf("code = %d, want %d", rpcErr.Code, protocol.CodeInvalidProtocolVersion)
	}
}

func TestHandleDoesNotMutateCallerRequestWhenStrippingMeta(t *testing.T) {
	req := &transport.Request{
		ID:     transport.StringID("1"),
		Method: "unknown.method",
		Params: json.RawMessage(`{"_meta":{"protocolVersion":"2026-06-07"},"value":1}`),
	}
	original := string(req.Params)
	New(nil).Handle(context.Background(), req, "")
	if got := string(req.Params); got != original {
		t.Fatalf("Handle mutated caller params: got %s, want %s", got, original)
	}
}
