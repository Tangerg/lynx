package agentexec

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

// noopObserver satisfies toolObserver. ConcurrencyKey forwarding never touches
// it, so every method is a no-op.
type noopObserver struct{}

func (noopObserver) ApproveToolCall(context.Context, string, string, string) ToolApprovalVerdict {
	return ToolApprovalVerdict{}
}
func (noopObserver) OnToolCallStart(string, string, string)                {}
func (noopObserver) OnToolCallEnd(string, string, string, []string, error) {}
func (noopObserver) OnMessageDelta(string)                                 {}
func (noopObserver) OnReasoningDelta(string)                               {}
func (noopObserver) OnUsage(accounting.TokenUsage, float64, int64)         {}

// keyedTool implements the loop's optional ConcurrencyKey contract as a keyed,
// concurrent tool (the shape of a per-path file edit).
type keyedTool struct{}

func (keyedTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: "keyed", InputSchema: json.RawMessage(`{}`)}
}
func (keyedTool) Call(context.Context, string) (string, error) { return "", nil }
func (keyedTool) ReturnsDirect() bool                          { return true }

// plainTool does NOT implement ConcurrencyKey — it must stay exclusive.
type plainTool struct{}

func (plainTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: "plain", InputSchema: json.RawMessage(`{}`)}
}
func (plainTool) Call(context.Context, string) (string, error) { return "", nil }

type mutatingTool struct{ err error }

func (mutatingTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: "mutating", InputSchema: json.RawMessage(`{}`)}
}

func (t mutatingTool) Call(context.Context, string) (string, error) {
	return `{"ok":true}`, t.err
}

func (mutatingTool) MutationPaths(string) ([]string, error) {
	return []string{"b.go", "", "a.go", "b.go"}, nil
}

func TestObservedToolForwardsReturnsDirect(t *testing.T) {
	keyed := &observedTool{inner: keyedTool{}, observer: noopObserver{}}
	direct, ok := tools.Tool(keyed).(interface{ ReturnsDirect() bool })
	if !ok {
		t.Fatal("observedTool must satisfy the return-direct marker when inner does")
	}
	if !direct.ReturnsDirect() {
		t.Fatal("ReturnsDirect marker was not forwarded")
	}

	plain := &observedTool{inner: plainTool{}, observer: noopObserver{}}
	if plain.ReturnsDirect() {
		t.Fatal("plain tool must not become return-direct")
	}
}

func TestObservedToolReportsOnlySuccessfulMutatedPaths(t *testing.T) {
	observer := new(recordingObserver)
	tool := &observedTool{inner: mutatingTool{}, observer: observer}
	if _, err := tool.Call(t.Context(), `{}`); err != nil {
		t.Fatalf("Call: %v", err)
	}
	ends := observer.ends()
	if len(ends) != 1 || !slices.Equal(ends[0].mutatedPaths, []string{"a.go", "b.go"}) {
		t.Fatalf("mutated paths = %+v, want [a.go b.go]", ends)
	}

	observer = new(recordingObserver)
	callErr := errors.New("write failed")
	tool = &observedTool{inner: mutatingTool{err: callErr}, observer: observer}
	if _, err := tool.Call(t.Context(), `{}`); !errors.Is(err, callErr) {
		t.Fatalf("Call error = %v, want %v", err, callErr)
	}
	ends = observer.ends()
	if len(ends) != 1 || len(ends[0].mutatedPaths) != 0 {
		t.Fatalf("failed call mutated paths = %+v, want none", ends)
	}
}
