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

// TestDirectOwnsOnlyItsMarker pins what Direct decides and what merely passes
// through it. The decorator owns the return-direct marker; every other
// capability belongs to the tool it wraps and stays reachable through the
// wrapping chain — including a host's own, which no longer has to be composed
// around this decorator.
func TestDirectOwnsOnlyItsMarker(t *testing.T) {
	wrapped := toolloop.Direct(directCapabilityTool{})

	direct, ok := toolloop.Capability[interface{ ReturnsDirect() bool }](wrapped)
	if !ok || !direct.ReturnsDirect() {
		t.Fatal("Direct() did not mark the tool return-direct")
	}
	concurrent, ok := toolloop.Capability[toolloop.ConcurrentTool](wrapped)
	if !ok {
		t.Fatal("the wrapped tool's scheduling capability is unreachable")
	}
	if key, allowed := concurrent.ConcurrencyKey(`{}`); key != "receipt.txt" || !allowed {
		t.Fatalf("ConcurrencyKey() = %q, %v; want receipt.txt, true", key, allowed)
	}
	host, ok := toolloop.Capability[interface{ HostMetadata() string }](wrapped)
	if !ok || host.HostMetadata() != "host-owned" {
		t.Fatal("a host capability of the wrapped tool is unreachable through the chain")
	}
	// Reachable through the chain is not the same as promoted onto the wrapper:
	// the decorator still declares only what it implements.
	if _, promoted := wrapped.(interface{ HostMetadata() string }); promoted {
		t.Fatal("Direct() promoted a host capability onto itself")
	}
	if output, err := wrapped.Call(t.Context(), `{}`); output != "written" || err != nil {
		t.Fatalf("Call() = %q, %v; want written, nil", output, err)
	}
}
