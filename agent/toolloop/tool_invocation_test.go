package toolloop

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
)

// TestToolRegistry_Lifecycle covers the mutation surface: register
// (nil-tolerant), find, names.
func TestToolRegistry_Lifecycle(t *testing.T) {
	inv := newInvoker()

	a := mustNewTool(t, "alpha")
	b := mustNewTool(t, "beta")
	inv.register(a, b, nil) // nil silently dropped

	if got, ok := inv.registry.find("alpha"); !ok || got.Definition().Name != "alpha" {
		t.Fatalf("Find returned (%v,%v)", got, ok)
	}
	if names := inv.registry.names(); len(names) != 2 {
		t.Fatalf("Names len = %d", len(names))
	}
}

func TestToolRegistry_Register_DuplicatesIgnored(t *testing.T) {
	inv := newInvoker()
	a := mustNewTool(t, "alpha")
	b := mustNewTool(t, "alpha") // same name

	inv.register(a, b)
	if names := inv.registry.names(); len(names) != 1 {
		t.Fatalf("names = %v, want 1 entry (duplicate names silently dropped)", names)
	}
}

// TestToolSupport_InvokeToolCalls_InternalReturnsForLLM verifies the
// happy path: an inline tool runs and the result message is built so
// the runtime can re-prompt the LLM.
func TestToolSupport_InvokeToolCalls_InternalReturnsForLLM(t *testing.T) {
	inv := newInvoker()
	inv.register(mustNewCallable(t, "echo", false, func(_ context.Context, args string) (string, error) {
		return "echoed:" + args, nil
	}))

	resp := responseWithToolCall(t, "echo", "args")
	req := mustNewRequest(t)

	if !inv.canInvokeToolCalls(resp) {
		t.Fatal("shouldInvokeToolCalls = false, want true")
	}

	result, err := inv.invoke(context.Background(), req, resp)
	if err != nil {
		t.Fatal(err)
	}
	if !result.shouldContinue() {
		t.Fatal("regular tool should request continuation")
	}
	cont, err := result.buildContinueRequest()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(cont.Messages); got != 2 {
		t.Fatalf("continuation has %d messages, want 2 (assistant tool-call + tool result)", got)
	}
	if am, ok := cont.Messages[0].(*chat.AssistantMessage); !ok || !am.HasToolCalls() {
		t.Fatalf("continuation[0] = %T, want *chat.AssistantMessage with tool calls", cont.Messages[0])
	}
	if _, ok := cont.Messages[1].(*chat.ToolMessage); !ok {
		t.Fatalf("continuation[1] is %T, want *chat.ToolMessage", cont.Messages[1])
	}
}

// TestToolSupport_InvokeToolCalls_ReturnDirectShortCircuits checks the
// other branch: a return-direct tool should not trigger an
// LLM follow-up.
func TestToolSupport_InvokeToolCalls_ReturnDirectShortCircuits(t *testing.T) {
	inv := newInvoker()
	inv.register(mustNewCallable(t, "direct", true, func(context.Context, string) (string, error) {
		return "ok", nil
	}))

	resp := responseWithToolCall(t, "direct", "")
	req := mustNewRequest(t)

	result, err := inv.invoke(context.Background(), req, resp)
	if err != nil {
		t.Fatal(err)
	}
	if result.shouldContinue() {
		t.Fatal("return-direct tool must not request continuation")
	}
	final, err := result.buildReturnResponse()
	if err != nil {
		t.Fatal(err)
	}
	if final == nil {
		t.Fatal("BuildReturnResponse returned nil")
	}
}

// TestToolSupport_InvokeToolCalls_UnknownToolFedBack pins the default: an
// unregistered tool is tolerated (not a hard error) and answered with an error
// result naming it, so the model can self-correct instead of the run aborting.
func TestToolSupport_InvokeToolCalls_UnknownToolFedBack(t *testing.T) {
	inv := newInvoker()
	resp := responseWithToolCall(t, "missing", "")

	if !inv.canInvokeToolCalls(resp) {
		t.Fatal("unknown tool should be tolerated, got shouldInvokeToolCalls = false")
	}

	result, err := inv.invoke(context.Background(), mustNewRequest(t), resp)
	if err != nil {
		t.Fatalf("unknown tool must not propagate as an error: %v", err)
	}
	if got := continuationToolResult(t, result); !strings.Contains(got, "missing") {
		t.Fatalf("fed-back result %q should mention the missing tool", got)
	}
}

// TestToolSupport_InvokeToolCalls_RecoverableFailureFedBack pins the default: a
// tool that returns an ordinary error fails recoverably; the error text is
// fed back as the tool result, the run does not abort.
func TestToolSupport_InvokeToolCalls_RecoverableFailureFedBack(t *testing.T) {
	inv := newInvoker()
	inv.register(mustNewCallable(t, "fail", false, func(context.Context, string) (string, error) {
		return "", errors.New("tool blew up")
	}))

	resp := responseWithToolCall(t, "fail", "")
	result, err := inv.invoke(context.Background(), mustNewRequest(t), resp)
	if err != nil {
		t.Fatalf("a recoverable failure must be fed back, not propagated: %v", err)
	}
	if got := continuationToolResult(t, result); !strings.Contains(got, "tool blew up") {
		t.Fatalf("fed-back result %q should include the failure text", got)
	}
}

func TestToolSupport_ShouldReturnDirect_RequiresAllReturnDirect(t *testing.T) {
	inv := newInvoker()
	inv.register(
		mustNewCallable(t, "a", true, nil),
		mustNewCallable(t, "b", false, nil),
	)

	tm, err := chat.NewToolMessage([]*chat.ToolReturn{
		{ID: "1", Name: "a", Result: "ok"},
		{ID: "2", Name: "b", Result: "ok"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if inv.shouldReturnDirect([]chat.Message{tm}) {
		t.Fatal("any non-return-direct tool must veto direct return")
	}
}

func TestToolSupport_ShouldReturnDirect_AllDirect(t *testing.T) {
	inv := newInvoker()
	inv.register(mustNewCallable(t, "a", true, nil))

	tm, err := chat.NewToolMessage([]*chat.ToolReturn{{ID: "1", Name: "a", Result: "ok"}})
	if err != nil {
		t.Fatal(err)
	}
	if !inv.shouldReturnDirect([]chat.Message{tm}) {
		t.Fatal("all return-direct tools must allow direct return")
	}
}

// TestToolSupport_InvokeToolCalls_AbortErrorPropagates pins the other side of
// the contract: a ToolLoopAbort error is not fed back. It propagates and stops
// the loop, so genuinely unrecoverable failures end the run.
func TestToolSupport_InvokeToolCalls_AbortErrorPropagates(t *testing.T) {
	inv := newInvoker()
	inv.register(mustNewCallable(t, "fatal", false, func(context.Context, string) (string, error) {
		return "", abortErr{}
	}))

	resp := responseWithToolCall(t, "fatal", "")
	if _, err := inv.invoke(context.Background(), mustNewRequest(t), resp); err == nil {
		t.Fatal("a ToolLoopAbort error must propagate (abort the loop), got nil")
	}
}
