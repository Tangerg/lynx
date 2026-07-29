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
		Method: "runs.cancel",
		Params: json.RawMessage(`{
			"_meta": {
				"protocolVersion": "2026-07-19",
				"clientInfo": { "name": "cli", "version": "0.1.0" },
				"clientCapabilities": {
					"features": {},
					"interruptTypes": ["approval"],
					"excludedEphemeralEvents": ["item.delta"]
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
	if meta.ProtocolVersion != "2026-07-19" {
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
		Method: "runs.cancel",
		Params: json.RawMessage(`{"_meta":"bad","runId":"run_1"}`),
	}

	_, rpcErr := bindRequestMeta(context.Background(), req)
	if rpcErr == nil {
		t.Fatalf("expected invalid params error")
	}
	if rpcErr.Code != protocol.CodeInvalidParams {
		t.Fatalf("code = %d, want %d", rpcErr.Code, protocol.CodeInvalidParams)
	}
}

func TestBindRequestMetaRejectsNullMeta(t *testing.T) {
	req := &transport.Request{
		ID:     transport.StringID("1"),
		Method: "runs.cancel",
		Params: json.RawMessage(`{"_meta":null,"runId":"run_1"}`),
	}

	_, rpcErr := bindRequestMeta(context.Background(), req)
	if rpcErr == nil {
		t.Fatalf("expected invalid params error")
	}
	if rpcErr.Code != protocol.CodeInvalidParams {
		t.Fatalf("code = %d, want %d", rpcErr.Code, protocol.CodeInvalidParams)
	}
}

func TestBindRequestMetaRejectsUnsupportedProtocolVersion(t *testing.T) {
	req := &transport.Request{
		ID:     transport.StringID("1"),
		Method: "runs.cancel",
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
		Params: json.RawMessage(`{"_meta":{"protocolVersion":"2026-07-19"},"value":1}`),
	}
	original := string(req.Params)
	New(nil).Handle(context.Background(), req)
	if got := string(req.Params); got != original {
		t.Fatalf("Handle mutated caller params: got %s, want %s", got, original)
	}
}

// TestBindRequestMetaRefusesADurableExclusion pins §8.1: a client may suppress an
// ephemeral preview, never an authoritative event. Ignoring the illegal entry
// would leave the client believing it had opted out of a frame it will keep
// receiving — and a runtime that honored it would break the §5.2 guarantee that
// discarding every ephemeral event still converges.
func TestBindRequestMetaRefusesADurableExclusion(t *testing.T) {
	req := &transport.Request{
		ID:     transport.StringID("1"),
		Method: "runs.cancel",
		Params: json.RawMessage(`{
			"_meta": {
				"clientCapabilities": { "excludedEphemeralEvents": ["item.completed"] }
			},
			"runId": "run_1"
		}`),
	}

	_, rpcErr := bindRequestMeta(context.Background(), req)
	if rpcErr == nil {
		t.Fatal("bindRequestMeta accepted an exclusion of a durable event")
	}
	if rpcErr.Code != protocol.CodeInvalidParams {
		t.Fatalf("error code = %d, want invalid_params (%d)", rpcErr.Code, protocol.CodeInvalidParams)
	}
}
