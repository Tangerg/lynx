package chat_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
)

func TestNewTool_RequiresNameSchemaAndExec(t *testing.T) {
	nop := func(context.Context, string) (string, error) { return "", nil }

	_, err := chat.NewTool(chat.ToolDefinition{}, chat.ToolMetadata{}, nop)
	if err == nil {
		t.Fatal("missing name must error")
	}

	_, err = chat.NewTool(chat.ToolDefinition{Name: "search"}, chat.ToolMetadata{}, nop)
	if err == nil {
		t.Fatal("missing schema must error")
	}

	_, err = chat.NewTool(chat.ToolDefinition{Name: "search", InputSchema: "{}"}, chat.ToolMetadata{}, nil)
	if err == nil {
		t.Fatal("nil execFunc must error")
	}
}

func TestNewTool_RunsExecFunc(t *testing.T) {
	tool, err := chat.NewTool(
		chat.ToolDefinition{Name: "echo", InputSchema: "{}"},
		chat.ToolMetadata{},
		func(_ context.Context, args string) (string, error) { return args, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := tool.Call(context.Background(), "hi")
	if err != nil {
		t.Fatal(err)
	}
	if got != "hi" {
		t.Fatalf("Call = %q, want hi", got)
	}
}

// TestToolRegistry_Lifecycle covers the full mutation surface in one
// run — register, find, exists, names, all, unregister, clear.
func TestToolRegistry_Lifecycle(t *testing.T) {
	support := chat.NewToolSupport()

	a := mustNewTool(t, "alpha")
	b := mustNewTool(t, "beta")
	support.Register(a, b, nil) // nil silently dropped

	if support.Registry().Size() != 2 {
		t.Fatalf("Size = %d, want 2", support.Registry().Size())
	}
	if !support.Registry().Exists("alpha") {
		t.Fatal("alpha must be registered")
	}
	if got, ok := support.Registry().Find("alpha"); !ok || got.Definition().Name != "alpha" {
		t.Fatalf("Find returned (%v,%v)", got, ok)
	}
	if names := support.Registry().Names(); len(names) != 2 {
		t.Fatalf("Names len = %d", len(names))
	}
	if all := support.Registry().All(); len(all) != 2 {
		t.Fatalf("All len = %d", len(all))
	}

	support.Unregister("alpha")
	if support.Registry().Exists("alpha") {
		t.Fatal("alpha should have been unregistered")
	}

	support.Registry().Clear()
	if support.Registry().Size() != 0 {
		t.Fatal("Clear did not empty the registry")
	}
}

func TestToolRegistry_Register_DuplicatesIgnored(t *testing.T) {
	support := chat.NewToolSupport()
	a := mustNewTool(t, "alpha")
	b := mustNewTool(t, "alpha") // same name

	support.Register(a, b)
	if support.Registry().Size() != 1 {
		t.Fatalf("Size = %d, want 1 (duplicate names silently dropped)", support.Registry().Size())
	}
}

// TestToolSupport_InvokeToolCalls_InternalReturnsForLLM verifies the
// happy path: an inline tool runs and the result message is built so
// the runtime can re-prompt the LLM.
func TestToolSupport_InvokeToolCalls_InternalReturnsForLLM(t *testing.T) {
	support := chat.NewToolSupport()
	support.Register(mustNewCallable(t, "echo", false, func(_ context.Context, args string) (string, error) {
		return "echoed:" + args, nil
	}))

	resp := responseWithToolCall(t, "echo", "args")
	req := mustNewRequest(t)

	can, err := support.ShouldInvokeToolCalls(resp)
	if err != nil || !can {
		t.Fatalf("ShouldInvokeToolCalls = (%v,%v)", can, err)
	}

	result, err := support.InvokeToolCalls(context.Background(), req, resp)
	if err != nil {
		t.Fatal(err)
	}
	if !result.ShouldContinue() {
		t.Fatal("internal tool with ReturnDirect=false should request continuation")
	}
	cont, err := result.BuildContinueRequest()
	if err != nil {
		t.Fatal(err)
	}
	// New contract: the continuation carries the turn's system header (none in
	// this request) + this round's assistant tool-call message + its tool
	// result — the atomic exchange the memory layer persists together. It does
	// NOT carry the prior conversation (the memory middleware owns stored
	// history and splices it back in). So here: [assistant(tool_calls), tool].
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
// other branch: a tool with ReturnDirect=true should not trigger an
// LLM follow-up.
func TestToolSupport_InvokeToolCalls_ReturnDirectShortCircuits(t *testing.T) {
	support := chat.NewToolSupport()
	support.Register(mustNewCallable(t, "direct", true, func(context.Context, string) (string, error) {
		return "ok", nil
	}))

	resp := responseWithToolCall(t, "direct", "")
	req := mustNewRequest(t)

	result, err := support.InvokeToolCalls(context.Background(), req, resp)
	if err != nil {
		t.Fatal(err)
	}
	if result.ShouldContinue() {
		t.Fatal("ReturnDirect=true must not request continuation")
	}
	final, err := result.BuildReturnResponse()
	if err != nil {
		t.Fatal(err)
	}
	if final == nil {
		t.Fatal("BuildReturnResponse returned nil")
	}
}

// continuationToolResult returns the concatenated tool-result text of the
// continuation request the invocation result would feed back to the model.
func continuationToolResult(t *testing.T, result *chat.ToolInvocationResult) string {
	t.Helper()
	cont, err := result.BuildContinueRequest()
	if err != nil {
		t.Fatalf("BuildContinueRequest: %v", err)
	}
	var b strings.Builder
	for _, m := range cont.Messages {
		if tm, ok := m.(*chat.ToolMessage); ok {
			for _, r := range tm.ToolReturns {
				b.WriteString(r.Result)
			}
		}
	}
	return b.String()
}

// TestToolSupport_InvokeToolCalls_UnknownToolFedBack pins the default: an
// unregistered tool is tolerated (not a hard error) and answered with an error
// result naming it, so the model can self-correct instead of the run aborting.
func TestToolSupport_InvokeToolCalls_UnknownToolFedBack(t *testing.T) {
	support := chat.NewToolSupport()
	resp := responseWithToolCall(t, "missing", "")

	can, err := support.ShouldInvokeToolCalls(resp)
	if err != nil || !can {
		t.Fatalf("unknown tool should be tolerated, got can=%v err=%v", can, err)
	}

	result, err := support.InvokeToolCalls(context.Background(), mustNewRequest(t), resp)
	if err != nil {
		t.Fatalf("unknown tool must not propagate as an error: %v", err)
	}
	if got := continuationToolResult(t, result); !strings.Contains(got, "missing") {
		t.Fatalf("fed-back result %q should mention the missing tool", got)
	}
}

// TestToolSupport_InvokeToolCalls_RecoverableFailureFedBack pins the default: a
// tool that returns an ordinary error fails RECOVERABLY — the error text is
// fed back as the tool result, the run does not abort.
func TestToolSupport_InvokeToolCalls_RecoverableFailureFedBack(t *testing.T) {
	support := chat.NewToolSupport()
	support.Register(mustNewCallable(t, "fail", false, func(context.Context, string) (string, error) {
		return "", errors.New("tool blew up")
	}))

	resp := responseWithToolCall(t, "fail", "")
	result, err := support.InvokeToolCalls(context.Background(), mustNewRequest(t), resp)
	if err != nil {
		t.Fatalf("a recoverable failure must be fed back, not propagated: %v", err)
	}
	if got := continuationToolResult(t, result); !strings.Contains(got, "tool blew up") {
		t.Fatalf("fed-back result %q should include the failure text", got)
	}
}

func TestToolSupport_ShouldReturnDirect_RequiresAllReturnDirect(t *testing.T) {
	support := chat.NewToolSupport()
	support.Register(
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
	if support.ShouldReturnDirect([]chat.Message{tm}) {
		t.Fatal("any non-return-direct tool must veto direct return")
	}
}

func TestToolSupport_ShouldReturnDirect_AllDirect(t *testing.T) {
	support := chat.NewToolSupport()
	support.Register(mustNewCallable(t, "a", true, nil))

	tm, err := chat.NewToolMessage([]*chat.ToolReturn{{ID: "1", Name: "a", Result: "ok"}})
	if err != nil {
		t.Fatal(err)
	}
	if !support.ShouldReturnDirect([]chat.Message{tm}) {
		t.Fatal("all return-direct tools must allow direct return")
	}
}

// --- helpers --------------------------------------------------------------

func mustNewCallable(t *testing.T, name string, returnDirect bool, fn func(context.Context, string) (string, error)) chat.Tool {
	t.Helper()
	if fn == nil {
		fn = func(context.Context, string) (string, error) { return "", nil }
	}
	tool, err := chat.NewTool(
		chat.ToolDefinition{Name: name, InputSchema: `{"type":"object"}`},
		chat.ToolMetadata{ReturnDirect: returnDirect},
		fn,
	)
	if err != nil {
		t.Fatalf("NewTool: %v", err)
	}
	return tool
}

func mustNewRequest(t *testing.T) *chat.Request {
	t.Helper()
	req, err := chat.NewRequest([]chat.Message{chat.NewUserMessage("hi")})
	if err != nil {
		t.Fatal(err)
	}
	opts, _ := chat.NewOptions("m")
	req.Options = opts
	return req
}

func responseWithToolCall(t *testing.T, name, args string) *chat.Response {
	t.Helper()
	resp, err := chat.NewResponse(
		&chat.Result{
			AssistantMessage: chat.NewAssistantMessage(chat.MessageParams{
				Parts: []chat.OutputPart{&chat.ToolCallPart{
					ID:        "call_1",
					Name:      name,
					Arguments: args,
				}},
			}),
			Metadata: &chat.ResultMetadata{FinishReason: chat.FinishReasonToolCalls},
		},
		&chat.ResponseMetadata{},
	)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// abortErr is a tool error that stops the loop the way a fatal/control-flow
// failure does — structurally (duck-typed), via ToolLoopAbort.
type abortErr struct{}

func (abortErr) Error() string       { return "abort: fatal" }
func (abortErr) ToolLoopAbort() bool { return true }

// TestToolSupport_InvokeToolCalls_AbortErrorPropagates pins the other side of
// the contract: a ToolLoopAbort error is NOT fed back — it propagates and
// stops the loop, so genuinely unrecoverable failures end the run.
func TestToolSupport_InvokeToolCalls_AbortErrorPropagates(t *testing.T) {
	support := chat.NewToolSupport()
	support.Register(mustNewCallable(t, "fatal", false, func(context.Context, string) (string, error) {
		return "", abortErr{}
	}))

	resp := responseWithToolCall(t, "fatal", "")
	if _, err := support.InvokeToolCalls(context.Background(), mustNewRequest(t), resp); err == nil {
		t.Fatal("a ToolLoopAbort error must propagate (abort the loop), got nil")
	}
}
