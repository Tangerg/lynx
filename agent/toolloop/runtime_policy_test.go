package toolloop_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Tangerg/lynx/agent/toolloop"
	"github.com/Tangerg/lynx/core/chat"
)

type directCapabilityTool struct{}

func (directCapabilityTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: "write_receipt", InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (directCapabilityTool) Call(context.Context, string) (string, error) { return "written", nil }
func (directCapabilityTool) ConcurrencyKey(string) (string, bool)         { return "receipt.txt", true }
func (directCapabilityTool) HostMetadata() string                         { return "host-owned" }

func TestDirectOwnsOnlyToolLoopCapabilities(t *testing.T) {
	wrapped := toolloop.Direct(directCapabilityTool{})

	direct, ok := wrapped.(interface{ ReturnsDirect() bool })
	if !ok || !direct.ReturnsDirect() {
		t.Fatal("Direct() did not mark the tool return-direct")
	}
	concurrent, ok := wrapped.(toolloop.ConcurrentTool)
	if !ok {
		t.Fatal("Direct() dropped the scheduling capability")
	}
	if key, allowed := concurrent.ConcurrencyKey(`{}`); key != "receipt.txt" || !allowed {
		t.Fatalf("ConcurrencyKey() = %q, %v; want receipt.txt, true", key, allowed)
	}
	if _, ok := wrapped.(interface{ HostMetadata() string }); ok {
		t.Fatal("Direct() leaked a host-owned capability through the tool-loop decorator")
	}
	if output, err := wrapped.Call(t.Context(), `{}`); output != "written" || err != nil {
		t.Fatalf("Call() = %q, %v; want written, nil", output, err)
	}
}
