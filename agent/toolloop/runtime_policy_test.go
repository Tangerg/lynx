package toolloop_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

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

// wrappingLoop is a decorator whose chain never reaches an innermost tool.
type wrappingLoop struct {
	name string
	next tools.Tool
}

func (w *wrappingLoop) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: w.name, InputSchema: json.RawMessage(`{}`)}
}
func (w *wrappingLoop) Call(context.Context, string) (string, error) { return "", nil }
func (w *wrappingLoop) Unwrap() tools.Tool                           { return w.next }

// TestCapabilityRefusesAChainThatDoesNotEnd pins the terminating condition of the
// wrapping walk. Two decorators naming each other used to spin forever, which
// hangs whatever asked — tool advertisement, scheduling, a host's own capability
// query — with no error and no progress.
func TestCapabilityRefusesAChainThatDoesNotEnd(t *testing.T) {
	for name, chainOf := range map[string]func() tools.Tool{
		"two decorators wrapping each other": func() tools.Tool {
			outer := &wrappingLoop{name: "outer"}
			inner := &wrappingLoop{name: "inner", next: outer}
			outer.next = inner
			return outer
		},
		"a decorator wrapping itself": func() tools.Tool {
			self := &wrappingLoop{name: "self"}
			self.next = self
			return self
		},
	} {
		t.Run(name, func(t *testing.T) {
			tool := chainOf()
			done := make(chan any, 1)
			go func() {
				defer func() { done <- recover() }()
				toolloop.Capability[toolloop.ConcurrentTool](tool)
			}()
			select {
			case recovered := <-done:
				if recovered == nil {
					t.Fatal("a chain that does not end resolved quietly instead of reporting itself")
				}
				if !strings.Contains(fmt.Sprint(recovered), "Unwrap chain does not end") {
					t.Fatalf("panic = %v, want it to name the unterminated chain", recovered)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Capability did not terminate on a cyclic wrapping chain")
			}
		})
	}
}
