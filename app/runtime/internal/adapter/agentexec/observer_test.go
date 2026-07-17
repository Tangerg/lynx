package agentexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/toolpolicy"
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
func (noopObserver) OnToolCallStart(string, string, string) {}
func (noopObserver) OnToolCallEnd(string, string, string, string, []string, error) {
}
func (noopObserver) OnMessageDelta(string)                         {}
func (noopObserver) OnReasoningDelta(string)                       {}
func (noopObserver) OnUsage(accounting.TokenUsage, float64, int64) {}

type blockingStartObserver struct {
	recordingObserver
	firstEntered chan struct{}
	releaseFirst chan struct{}
}

func (o *blockingStartObserver) OnToolCallStart(callID, toolName, arguments string) {
	if toolName == "first" {
		close(o.firstEntered)
		<-o.releaseFirst
	}
	o.recordingObserver.OnToolCallStart(callID, toolName, arguments)
}

// keyedTool implements the loop's optional ConcurrencyKey contract as a keyed,
// concurrent tool (the shape of a per-path file edit).
type keyedTool struct{}

func (keyedTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: "keyed", InputSchema: json.RawMessage(`{}`)}
}
func (keyedTool) Call(context.Context, string) (string, error) { return "", nil }
func (keyedTool) ReturnsDirect() bool                          { return true }
func (keyedTool) ConcurrencyKey(string) (string, bool)         { return "resource", true }

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
	observation := newToolObservation(noopObserver{})
	keyed := &observedTool{inner: keyedTool{}, observation: observation}
	direct, ok := tools.Tool(keyed).(interface{ ReturnsDirect() bool })
	if !ok {
		t.Fatal("observedTool must satisfy the return-direct marker when inner does")
	}
	if !direct.ReturnsDirect() {
		t.Fatal("ReturnsDirect marker was not forwarded")
	}

	plain := &observedTool{inner: plainTool{}, observation: observation}
	if plain.ReturnsDirect() {
		t.Fatal("plain tool must not become return-direct")
	}
}

func TestObservedToolForwardsConcurrencyKey(t *testing.T) {
	observation := newToolObservation(noopObserver{})
	keyed := &observedTool{inner: keyedTool{}, observation: observation}
	key, concurrent := keyed.ConcurrencyKey(`{}`)
	if key != "resource" || !concurrent {
		t.Fatalf("keyed concurrency = %q, %v", key, concurrent)
	}

	plain := &observedTool{inner: plainTool{}, observation: observation}
	key, concurrent = plain.ConcurrencyKey(`{}`)
	if key != "" || concurrent {
		t.Fatalf("plain concurrency = %q, %v", key, concurrent)
	}
}

func TestObservedToolReportsOnlySuccessfulMutatedPaths(t *testing.T) {
	observer := new(recordingObserver)
	tool := &observedTool{inner: mutatingTool{}, observation: newToolObservation(observer)}
	if _, err := tool.Call(t.Context(), `{}`); err != nil {
		t.Fatalf("Call: %v", err)
	}
	ends := observer.ends()
	if len(ends) != 1 || !slices.Equal(ends[0].mutatedPaths, []string{"a.go", "b.go"}) {
		t.Fatalf("mutated paths = %+v, want [a.go b.go]", ends)
	}

	observer = new(recordingObserver)
	callErr := errors.New("write failed")
	tool = &observedTool{inner: mutatingTool{err: callErr}, observation: newToolObservation(observer)}
	if _, err := tool.Call(t.Context(), `{}`); !errors.Is(err, callErr) {
		t.Fatalf("Call error = %v, want %v", err, callErr)
	}
	ends = observer.ends()
	if len(ends) != 1 || len(ends[0].mutatedPaths) != 0 {
		t.Fatalf("failed call mutated paths = %+v, want none", ends)
	}
}

func TestObservedToolPreservesMutationPathsThroughPolicyWrappers(t *testing.T) {
	policy, err := toolpolicy.Once(mutatingTool{})
	if err != nil {
		t.Fatalf("Once: %v", err)
	}
	observer := new(recordingObserver)
	wrapped := &observedTool{inner: policy, observation: newToolObservation(observer)}

	reporter, ok := tools.Tool(wrapped).(tools.FileMutationReporter)
	if !ok {
		t.Fatal("observed tool dropped the file-mutation capability")
	}
	paths, err := reporter.MutationPaths(`{}`)
	if err != nil || !slices.Equal(paths, []string{"b.go", "", "a.go", "b.go"}) {
		t.Fatalf("MutationPaths() = %v, %v", paths, err)
	}
	if _, err := wrapped.Call(toolpolicy.WithScope(t.Context()), `{}`); err != nil {
		t.Fatalf("Call: %v", err)
	}
	ends := observer.ends()
	if len(ends) != 1 || !slices.Equal(ends[0].mutatedPaths, []string{"a.go", "b.go"}) {
		t.Fatalf("observed mutation paths = %+v, want [a.go b.go]", ends)
	}
}

func TestToolObservationPublishesPreparedStartsInModelOrder(t *testing.T) {
	observer := new(recordingObserver)
	observation := newToolObservation(observer)
	observation.begin("process-1", 2, chat.ToolCall{ID: "call-1", Name: "first", Arguments: `{}`})
	observation.begin("process-1", 2, chat.ToolCall{ID: "call-2", Name: "second", Arguments: `{}`})

	observation.mu.Lock()
	first := observation.model["call-1"]
	second := observation.model["call-2"]
	observation.mu.Unlock()

	secondDone := make(chan struct{})
	go func() {
		observation.prepare(second, `{"effective":2}`)
		close(secondDone)
	}()
	waitForPreparedCall(t, observation, second)
	if starts := observer.starts(); len(starts) != 0 {
		t.Fatalf("starts before first call was prepared = %+v", starts)
	}

	observation.prepare(first, `{"effective":1}`)
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("second preparation remained blocked after first was published")
	}
	starts := observer.starts()
	if got := []string{starts[0].toolName, starts[1].toolName}; !slices.Equal(got, []string{"first", "second"}) {
		t.Fatalf("start order = %v, want [first second]", got)
	}
	if starts[0].arguments != `{"effective":1}` || starts[1].arguments != `{"effective":2}` {
		t.Fatalf("effective start arguments = %+v", starts)
	}
	if starts[0].callID == starts[1].callID {
		t.Fatalf("concurrent calls share ID %q", starts[0].callID)
	}
}

func TestToolObservationSerializesClaimedStartBatches(t *testing.T) {
	observer := &blockingStartObserver{
		firstEntered: make(chan struct{}),
		releaseFirst: make(chan struct{}),
	}
	observation := newToolObservation(observer)
	for index, name := range []string{"first", "second", "third"} {
		observation.begin("process-1", 1, chat.ToolCall{
			ID: fmt.Sprintf("call-%d", index+1), Name: name, Arguments: `{}`,
		})
	}
	observation.mu.Lock()
	first := observation.model["call-1"]
	second := observation.model["call-2"]
	third := observation.model["call-3"]
	observation.mu.Unlock()

	secondDone := make(chan struct{})
	go func() {
		observation.prepare(second, `{}`)
		close(secondDone)
	}()
	waitForPreparedCall(t, observation, second)

	firstDone := make(chan struct{})
	go func() {
		observation.prepare(first, `{}`)
		close(firstDone)
	}()
	select {
	case <-observer.firstEntered:
	case <-time.After(time.Second):
		t.Fatal("first start callback was not entered")
	}

	thirdDone := make(chan struct{})
	go func() {
		observation.prepare(third, `{}`)
		close(thirdDone)
	}()
	waitForPreparedCall(t, observation, third)
	if starts := observer.starts(); len(starts) != 0 {
		t.Fatalf("later batch overtook blocked first start: %+v", starts)
	}

	close(observer.releaseFirst)
	for _, done := range []<-chan struct{}{firstDone, secondDone, thirdDone} {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("ordered start publisher did not release all callers")
		}
	}
	starts := observer.starts()
	if got := []string{starts[0].toolName, starts[1].toolName, starts[2].toolName}; !slices.Equal(got, []string{"first", "second", "third"}) {
		t.Fatalf("serialized start order = %v", got)
	}
}

func TestModelToolCallIDIncludesProcessAndRoundOwnership(t *testing.T) {
	base := modelToolCallID("process-1", 1, "call-1")
	for _, other := range []string{
		modelToolCallID("process-2", 1, "call-1"),
		modelToolCallID("process-1", 2, "call-1"),
		modelToolCallID("process-1", 1, "call-2"),
	} {
		if other == base {
			t.Fatalf("model tool call identity collision: %q", base)
		}
	}
	if resumed := modelToolCallID("process-1", 1, "call-1"); resumed != base {
		t.Fatalf("resumed model call ID = %q, want stable %q", resumed, base)
	}
}

func TestToolObservationClosesUnknownCallsButIgnoresRestoredSettledResults(t *testing.T) {
	observer := new(recordingObserver)
	observation := newToolObservation(observer)
	result := chat.ToolResult{ID: "missing-1", Name: "missing", Result: "not available", IsError: true}

	observation.result("process-1", 1, result)
	if len(observer.starts()) != 0 || len(observer.ends()) != 0 {
		t.Fatal("result without a boundary was emitted; restored settled results must not duplicate lifecycle")
	}

	observation.begin("process-1", 1, chat.ToolCall{ID: "missing-1", Name: "missing", Arguments: `{}`})
	observation.result("process-1", 1, result)
	starts, ends := observer.starts(), observer.ends()
	if len(starts) != 1 || len(ends) != 1 || starts[0].callID != ends[0].callID {
		t.Fatalf("unknown tool lifecycle = %+v / %+v, want one paired start/end", starts, ends)
	}
	if ends[0].err == nil || ends[0].arguments != `{}` {
		t.Fatalf("unknown tool completion = %+v", ends[0])
	}
}

func waitForPreparedCall(t *testing.T, observation *toolObservation, call *observedModelCall) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		observation.mu.Lock()
		prepared := call.prepared
		observation.mu.Unlock()
		if prepared {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("tool call did not reach start-ordering barrier")
		}
		runtime.Gosched()
	}
}
