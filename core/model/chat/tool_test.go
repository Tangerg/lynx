package chat_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
)

func TestNewTool_RequiresNameAndSchema(t *testing.T) {
	_, err := chat.NewTool(chat.ToolDefinition{}, chat.ToolMetadata{}, nil)
	if err == nil {
		t.Fatal("missing name+schema must error")
	}

	_, err = chat.NewTool(chat.ToolDefinition{Name: "search"}, chat.ToolMetadata{}, nil)
	if err == nil {
		t.Fatal("missing schema must error")
	}
}

func TestNewTool_External_SatisfiesToolOnly(t *testing.T) {
	tool, err := chat.NewTool(
		chat.ToolDefinition{Name: "external", InputSchema: "{}"},
		chat.ToolMetadata{ReturnDirect: true},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tool.(chat.CallableTool); ok {
		t.Fatal("external tool must NOT satisfy CallableTool")
	}
}

func TestNewTool_Internal_SatisfiesCallable(t *testing.T) {
	tool, err := chat.NewTool(
		chat.ToolDefinition{Name: "echo", InputSchema: "{}"},
		chat.ToolMetadata{},
		func(ctx context.Context, args string) (string, error) { return args, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	callable, ok := tool.(chat.CallableTool)
	if !ok {
		t.Fatal("internal tool must satisfy CallableTool")
	}
	got, err := callable.Call(context.Background(), "hi")
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
	support.RegisterTools(a, b, nil) // nil silently dropped

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

	support.UnregisterTools("alpha")
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

	support.RegisterTools(a, b)
	if support.Registry().Size() != 1 {
		t.Fatalf("Size = %d, want 1 (duplicate names silently dropped)", support.Registry().Size())
	}
}

// TestToolSupport_InvokeToolCalls_InternalReturnsForLLM verifies the
// happy path: an inline tool runs and the result message is built so
// the runtime can re-prompt the LLM.
func TestToolSupport_InvokeToolCalls_InternalReturnsForLLM(t *testing.T) {
	support := chat.NewToolSupport()
	support.RegisterTools(mustNewCallable(t, "echo", false, func(_ context.Context, args string) (string, error) {
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
	if got := len(cont.Messages); got < 3 {
		t.Fatalf("continuation has %d messages, want at least 3 (orig + assistant + tool)", got)
	}
}

// TestToolSupport_InvokeToolCalls_ReturnDirectShortCircuits checks the
// other branch: a tool with ReturnDirect=true should not trigger an
// LLM follow-up.
func TestToolSupport_InvokeToolCalls_ReturnDirectShortCircuits(t *testing.T) {
	support := chat.NewToolSupport()
	support.RegisterTools(mustNewCallable(t, "direct", true, func(context.Context, string) (string, error) {
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

// TestToolSupport_InvokeToolCalls_ExternalForcesReturn verifies that an
// unknown-call-able tool (delegated/external) routes the call to the
// host instead of running it.
func TestToolSupport_InvokeToolCalls_ExternalForcesReturn(t *testing.T) {
	support := chat.NewToolSupport()
	// External tool — no exec function.
	tool, err := chat.NewTool(chat.ToolDefinition{Name: "external", InputSchema: "{}"}, chat.ToolMetadata{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	support.RegisterTools(tool)

	resp := responseWithToolCall(t, "external", "args")
	req := mustNewRequest(t)

	result, err := support.InvokeToolCalls(context.Background(), req, resp)
	if err != nil {
		t.Fatal(err)
	}
	if result.ShouldContinue() {
		t.Fatal("external tool must force return-direct")
	}
}

func TestToolSupport_ShouldInvokeToolCalls_UnknownToolErrors(t *testing.T) {
	support := chat.NewToolSupport()
	resp := responseWithToolCall(t, "missing", "")

	_, err := support.ShouldInvokeToolCalls(resp)
	if err == nil {
		t.Fatal("unknown tool must error")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("error %q should mention the offending tool name", err.Error())
	}
}

func TestToolSupport_InvokeToolCalls_ToolFailurePropagates(t *testing.T) {
	wantErr := errors.New("tool blew up")

	support := chat.NewToolSupport()
	support.RegisterTools(mustNewCallable(t, "fail", false, func(context.Context, string) (string, error) {
		return "", wantErr
	}))

	resp := responseWithToolCall(t, "fail", "")
	req := mustNewRequest(t)

	_, err := support.InvokeToolCalls(context.Background(), req, resp)
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want chain to include %v", err, wantErr)
	}
}

func TestToolSupport_ShouldReturnDirect_RequiresAllReturnDirect(t *testing.T) {
	support := chat.NewToolSupport()
	support.RegisterTools(
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
	support.RegisterTools(mustNewCallable(t, "a", true, nil))

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
		[]*chat.Result{{
			AssistantMessage: chat.NewAssistantMessage(chat.MessageParams{
				ToolCalls: []*chat.ToolCall{{ID: "call_1", Name: name, Arguments: args}},
			}),
			Metadata: &chat.ResultMetadata{FinishReason: chat.FinishReasonToolCalls},
		}},
		&chat.ResponseMetadata{},
	)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}
