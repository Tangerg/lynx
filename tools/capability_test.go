package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

type capabilityMarker interface {
	Marker() string
}

type markedTool struct{}

func (markedTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: "marked", InputSchema: json.RawMessage(`{}`)}
}

func (markedTool) Call(context.Context, string) (string, error) { return "", nil }
func (markedTool) Marker() string                               { return "inner" }

type wrappingTool struct{ inner tools.Tool }

func (w *wrappingTool) Definition() chat.ToolDefinition { return w.inner.Definition() }
func (w *wrappingTool) Call(ctx context.Context, input string) (string, error) {
	return w.inner.Call(ctx, input)
}
func (w *wrappingTool) Unwrap() tools.Tool { return w.inner }

func TestCapabilityTraversesWrappingTools(t *testing.T) {
	tool := &wrappingTool{inner: &wrappingTool{inner: markedTool{}}}
	marker, ok, err := tools.Capability[capabilityMarker](tool)
	if err != nil || !ok || marker.Marker() != "inner" {
		t.Fatalf("Capability() = %v, %v, %v; want inner, true, nil", marker, ok, err)
	}
}

// TestCapabilityRejectsWrappingCycles guards the traversal's termination, so it
// runs the lookup off the test goroutine: if the bound regresses, Capability
// never returns, and asserting inline would hang the package until Go's global
// test timeout instead of naming the broken invariant.
func TestCapabilityRejectsWrappingCycles(t *testing.T) {
	for name, chainOf := range map[string]func() tools.Tool{
		"two wrappers naming each other": func() tools.Tool {
			first := &wrappingTool{}
			second := &wrappingTool{inner: first}
			first.inner = second
			return first
		},
		"a wrapper naming itself": func() tools.Tool {
			self := &wrappingTool{}
			self.inner = self
			return self
		},
	} {
		t.Run(name, func(t *testing.T) {
			type outcome struct {
				found bool
				err   error
			}
			done := make(chan outcome, 1)
			go func() {
				_, found, err := tools.Capability[capabilityMarker](chainOf())
				done <- outcome{found: found, err: err}
			}()

			select {
			case got := <-done:
				if got.found || !errors.Is(got.err, tools.ErrInvalidWrappingChain) {
					t.Fatalf("Capability() = _, %v, %v; want false, ErrInvalidWrappingChain", got.found, got.err)
				}
			// A correct lookup is a bounded run of type assertions with no I/O, so
			// it returns in microseconds however loaded the machine is. This only
			// has to be long enough to never fire on a healthy build.
			case <-time.After(5 * time.Second):
				t.Fatal("Capability did not return on a wrapping chain that never ends")
			}
		})
	}
}

type panickingWrapper struct{ markedTool }

func (*panickingWrapper) Unwrap() tools.Tool { panic("broken unwrap") }

func TestCapabilityReturnsUnwrapPanics(t *testing.T) {
	_, ok, err := tools.Capability[interface{ Missing() }](new(panickingWrapper))
	if ok || !errors.Is(err, tools.ErrInvalidWrappingChain) {
		t.Fatalf("Capability() = _, %v, %v; want false, ErrInvalidWrappingChain", ok, err)
	}
}
