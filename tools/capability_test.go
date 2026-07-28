package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

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

func TestCapabilityRejectsWrappingCycles(t *testing.T) {
	first := &wrappingTool{}
	second := &wrappingTool{inner: first}
	first.inner = second

	_, ok, err := tools.Capability[capabilityMarker](first)
	if ok || !errors.Is(err, tools.ErrInvalidWrappingChain) {
		t.Fatalf("Capability() = _, %v, %v; want false, ErrInvalidWrappingChain", ok, err)
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
