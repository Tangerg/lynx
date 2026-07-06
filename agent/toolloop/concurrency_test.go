package toolloop

import (
	"context"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"
)

// concTool is a stub tool that declares its concurrency via the optional
// [ConcurrentTool] capability. Its Call records the peak number of overlapping
// executions (via a shared counter) and holds briefly so concurrent calls
// actually overlap — letting a test tell parallel execution from serial.
type concTool struct {
	name       string
	concurrent bool
	key        string
	cur        *atomic.Int32
	max        *atomic.Int32
}

func (c *concTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: c.name, Description: "stub", InputSchema: "{}"}
}

// ConcurrencyKey is the [ConcurrentTool] opt-in: (key, concurrent).
func (c *concTool) ConcurrencyKey(string) (string, bool) { return c.key, c.concurrent }

func (c *concTool) Call(context.Context, string) (string, error) {
	n := c.cur.Add(1)
	for { // record the running max
		m := c.max.Load()
		if n <= m || c.max.CompareAndSwap(m, n) {
			break
		}
	}
	time.Sleep(30 * time.Millisecond) // hold so concurrent calls overlap
	c.cur.Add(-1)
	return c.name, nil
}

func concCalls(names ...string) []*chat.ToolCallPart {
	out := make([]*chat.ToolCallPart, len(names))
	for i, n := range names {
		out[i] = &chat.ToolCallPart{ID: n, Name: n, Arguments: "{}"}
	}
	return out
}

func resultNames(res *invocationResult) []string {
	out := make([]string, 0, len(res.toolMessage.ToolReturns))
	for _, r := range res.toolMessage.ToolReturns {
		out = append(out, r.Name)
	}
	return out
}

// TestInvoker_ParallelToolsRunConcurrently pins that several parallel
// (concurrent, no-conflict) calls in one round overlap, and that results are
// still returned in call order regardless of completion order.
func TestInvoker_ParallelToolsRunConcurrently(t *testing.T) {
	var cur, max atomic.Int32
	inv := newInvoker()
	for _, n := range []string{"a", "b", "c"} {
		inv.register(&concTool{name: n, concurrent: true, cur: &cur, max: &max})
	}

	res, err := inv.invokeToolCalls(context.Background(), concCalls("a", "b", "c"))
	if err != nil {
		t.Fatalf("invokeToolCalls: %v", err)
	}
	if got := max.Load(); got < 2 {
		t.Fatalf("peak concurrency = %d, want >= 2 (parallel tools must overlap)", got)
	}
	if got, want := resultNames(res), []string{"a", "b", "c"}; !slices.Equal(got, want) {
		t.Fatalf("result order = %v, want %v (call order preserved)", got, want)
	}
}

// TestInvoker_KeyedConflict pins the resource-conflict rule: two concurrent
// calls reporting the SAME key serialize; DISTINCT keys run in parallel.
func TestInvoker_KeyedConflict(t *testing.T) {
	var cur, max atomic.Int32
	inv := newInvoker()
	inv.register(&concTool{name: "e1", concurrent: true, key: "same", cur: &cur, max: &max})
	inv.register(&concTool{name: "e2", concurrent: true, key: "same", cur: &cur, max: &max})
	if _, err := inv.invokeToolCalls(context.Background(), concCalls("e1", "e2")); err != nil {
		t.Fatalf("same-key invoke: %v", err)
	}
	if got := max.Load(); got != 1 {
		t.Fatalf("same-key peak concurrency = %d, want 1 (same resource serializes)", got)
	}

	var cur2, max2 atomic.Int32
	inv2 := newInvoker()
	inv2.register(&concTool{name: "f1", concurrent: true, key: "k1", cur: &cur2, max: &max2})
	inv2.register(&concTool{name: "f2", concurrent: true, key: "k2", cur: &cur2, max: &max2})
	if _, err := inv2.invokeToolCalls(context.Background(), concCalls("f1", "f2")); err != nil {
		t.Fatalf("distinct-key invoke: %v", err)
	}
	if got := max2.Load(); got < 2 {
		t.Fatalf("distinct-key peak concurrency = %d, want >= 2 (distinct resources parallelize)", got)
	}
}

// panicTool is a parallel tool that always panics — it exercises the loop's
// panic containment. A tool runs arbitrary code, and in a parallel batch it
// runs in a goroutine the loop spawns, so an escaping panic would crash the
// whole process rather than fail one call.
type panicTool struct{ name string }

func (p *panicTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: p.name, Description: "stub", InputSchema: "{}"}
}
func (p *panicTool) ConcurrencyKey(string) (string, bool) { return "", true }
func (p *panicTool) Call(context.Context, string) (string, error) {
	panic("boom")
}

// TestInvoker_ToolPanicContained pins that a panic in a parallel-batch tool is
// contained at the tool boundary instead of escaping its goroutine and
// crashing the process: it comes back as a recoverable tool result (so the
// model can route around the broken tool), and a sibling call in the same
// batch still completes. Without containment this test would crash the binary.
func TestInvoker_ToolPanicContained(t *testing.T) {
	var cur, max atomic.Int32
	inv := newInvoker()
	inv.register(&panicTool{name: "boom"})
	inv.register(&concTool{name: "ok", concurrent: true, cur: &cur, max: &max})

	res, err := inv.invokeToolCalls(context.Background(), concCalls("boom", "ok"))
	if err != nil {
		t.Fatalf("invokeToolCalls returned error (a panic must be recoverable, not fatal): %v", err)
	}
	byName := make(map[string]string, len(res.toolMessage.ToolReturns))
	for _, r := range res.toolMessage.ToolReturns {
		byName[r.Name] = r.Result
	}
	if got := byName["boom"]; !strings.Contains(got, "panic") {
		t.Fatalf("panicking tool result = %q, want it to surface the panic", got)
	}
	if got, want := byName["ok"], "ok"; got != want {
		t.Fatalf("sibling result = %q, want %q (a sibling's panic must not drop it)", got, want)
	}
}

// TestInvoker_ExclusiveDefaultSerial pins the conservative default: a tool that
// reports concurrent=false (the behavior of a tool not implementing
// ConcurrentTool) runs strictly one at a time.
func TestInvoker_ExclusiveDefaultSerial(t *testing.T) {
	var cur, max atomic.Int32
	inv := newInvoker()
	for _, n := range []string{"x", "y"} {
		inv.register(&concTool{name: n, concurrent: false, cur: &cur, max: &max})
	}
	if _, err := inv.invokeToolCalls(context.Background(), concCalls("x", "y")); err != nil {
		t.Fatalf("invokeToolCalls: %v", err)
	}
	if got := max.Load(); got != 1 {
		t.Fatalf("exclusive (default) peak concurrency = %d, want 1 (serial)", got)
	}
}
