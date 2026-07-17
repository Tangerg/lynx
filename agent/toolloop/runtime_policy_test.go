package toolloop_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/agent/toolloop"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

type directCapabilityTool struct{}

func (directCapabilityTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: "write_receipt", InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (directCapabilityTool) Call(context.Context, string) (string, error) { return "written", nil }
func (directCapabilityTool) ConcurrencyKey(string) (string, bool)         { return "receipt.txt", true }
func (directCapabilityTool) MutationPaths(string) ([]string, error) {
	return []string{"receipt.txt"}, nil
}

func TestDirectPreservesSchedulingAndMutationCapabilities(t *testing.T) {
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
	reporter, ok := wrapped.(tools.FileMutationReporter)
	if !ok {
		t.Fatal("Direct() dropped the file-mutation capability")
	}
	paths, err := reporter.MutationPaths(`{}`)
	if err != nil || !slices.Equal(paths, []string{"receipt.txt"}) {
		t.Fatalf("MutationPaths() = %v, %v; want [receipt.txt], nil", paths, err)
	}
	if output, err := wrapped.Call(t.Context(), `{}`); output != "written" || err != nil {
		t.Fatalf("Call() = %q, %v; want written, nil", output, err)
	}
}
