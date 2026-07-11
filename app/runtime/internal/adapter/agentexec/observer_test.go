package agentexec

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/core/model/chat"
)

// noopObserver satisfies toolObserver. ConcurrencyKey forwarding never touches
// it, so every method is a no-op.
type noopObserver struct{}

func (noopObserver) ApproveToolCall(context.Context, string, string, string) ToolApprovalVerdict {
	return ToolApprovalVerdict{}
}
func (noopObserver) OnToolCallStart(string, string, string)        {}
func (noopObserver) OnToolCallEnd(string, string, string, error)   {}
func (noopObserver) OnMessageDelta(string)                         {}
func (noopObserver) OnReasoningDelta(string)                       {}
func (noopObserver) OnUsage(accounting.TokenUsage, float64, int64) {}

// keyedTool implements the loop's optional ConcurrencyKey contract as a keyed,
// concurrent tool (the shape of a per-path file edit).
type keyedTool struct{ key string }

func (keyedTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: "keyed", InputSchema: "{}"}
}
func (keyedTool) Call(context.Context, string) (string, error) { return "", nil }
func (k keyedTool) ConcurrencyKey(string) (string, bool)       { return k.key, true }
func (keyedTool) ReturnsDirect() bool                          { return true }

// plainTool does NOT implement ConcurrencyKey — it must stay exclusive.
type plainTool struct{}

func (plainTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: "plain", InputSchema: "{}"}
}
func (plainTool) Call(context.Context, string) (string, error) { return "", nil }

// TestObservedToolForwardsConcurrencyKey pins that the tool-observer decorator,
// which wraps EVERY resolved tool, preserves the inner tool's concurrency
// declaration. Dropping it forces every tool exclusive (serial) and silently
// defeats parallel tool execution (concurrent `task` sub-agents, distinct-file
// edits). A tool that declares nothing stays exclusive.
func TestObservedToolForwardsConcurrencyKey(t *testing.T) {
	keyed := &observedTool{inner: keyedTool{key: "/tmp/a.go"}, observer: noopObserver{}}
	// The loop discovers concurrency via this structural interface (its
	// ConcurrentTool contract) — assert the wrapper satisfies it AND forwards.
	c, ok := chat.Tool(keyed).(interface {
		ConcurrencyKey(string) (string, bool)
	})
	if !ok {
		t.Fatal("observedTool must satisfy the loop's ConcurrencyKey contract")
	}
	if key, conc := c.ConcurrencyKey("{}"); key != "/tmp/a.go" || !conc {
		t.Fatalf("forwarded ConcurrencyKey = (%q, %v), want (\"/tmp/a.go\", true)", key, conc)
	}

	plain := &observedTool{inner: plainTool{}, observer: noopObserver{}}
	if key, conc := plain.ConcurrencyKey("{}"); key != "" || conc {
		t.Fatalf("plain tool ConcurrencyKey = (%q, %v), want (\"\", false) — exclusive", key, conc)
	}
}

func TestObservedToolForwardsReturnsDirect(t *testing.T) {
	keyed := &observedTool{inner: keyedTool{key: "/tmp/a.go"}, observer: noopObserver{}}
	direct, ok := chat.Tool(keyed).(interface{ ReturnsDirect() bool })
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
