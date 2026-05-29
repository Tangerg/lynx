package chat_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
)

// TestToolMiddleware_RecursiveLoop checks the headline behavior:
//  1. model returns a tool-call response,
//  2. middleware executes the tool,
//  3. middleware re-prompts the model with the tool result,
//  4. the second model invocation returns a regular reply,
//  5. middleware returns that reply to the caller.
func TestToolMiddleware_RecursiveLoop(t *testing.T) {
	model := newFakeChatModel(t)

	calls := 0
	model.respond = func(req *chat.Request) (*chat.Response, error) {
		calls++
		if calls == 1 {
			return responseWithToolCall(t, "echo", `{"x":1}`), nil
		}
		return responseWithText("final answer"), nil
	}

	echoTool := mustNewCallable(t, "echo", false, func(_ context.Context, args string) (string, error) {
		return "echoed:" + args, nil
	})

	callMW, _ := chat.NewToolMiddleware()
	req, _ := chat.NewClientRequest(model)
	req.
		WithMiddlewares(callMW).
		WithMessages(chat.NewUserMessage("seed")).
		WithTools(echoTool)

	resp, err := req.Call().Response(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("model invoked %d times, want 2", calls)
	}
	if resp.Result.AssistantMessage.JoinedText() != "final answer" {
		t.Fatalf("Text = %q, want final answer", resp.Result.AssistantMessage.JoinedText())
	}
}

// TestToolMiddleware_DirectReturn confirms the short-circuit path:
// when every called tool is ReturnDirect the middleware skips the
// follow-up LLM call and returns the tool result wrapped as a response.
func TestToolMiddleware_DirectReturn(t *testing.T) {
	model := newFakeChatModel(t)

	calls := 0
	model.respond = func(req *chat.Request) (*chat.Response, error) {
		calls++
		return responseWithToolCall(t, "notify", `{}`), nil
	}

	notify := mustNewCallable(t, "notify", true, func(context.Context, string) (string, error) {
		return "sent", nil
	})

	callMW, _ := chat.NewToolMiddleware()
	req, _ := chat.NewClientRequest(model)
	req.
		WithMiddlewares(callMW).
		WithMessages(chat.NewUserMessage("seed")).
		WithTools(notify)

	resp, err := req.Call().Response(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("model invoked %d times, want 1 (direct return must short-circuit)", calls)
	}
	if resp.Result.Metadata.FinishReason != chat.FinishReasonToolCalls {
		t.Fatalf("FinishReason = %q", resp.Result.Metadata.FinishReason)
	}
}

// TestToolMiddleware_StreamEmitsToolMessageBetweenTurns checks the
// streaming-path invariant from MESSAGE_PARTS_DESIGN §8.4: the
// runtime-injected ToolMessage MUST be yielded to the external
// consumer as its own delta between assistant turns, so the
// downstream timeline matches the message history fed to the next
// model call.
func TestToolMiddleware_StreamEmitsToolMessageBetweenTurns(t *testing.T) {
	model := newFakeChatModel(t)

	streamCalls := 0
	model.streamRespond = func(req *chat.Request) []*chat.Response {
		streamCalls++
		if streamCalls == 1 {
			return []*chat.Response{responseWithToolCall(t, "echo", `{"x":1}`)}
		}
		return []*chat.Response{responseWithText("final answer")}
	}

	echoTool := mustNewCallable(t, "echo", false, func(_ context.Context, args string) (string, error) {
		return "echoed:" + args, nil
	})

	_, streamMW := chat.NewToolMiddleware()
	req, _ := chat.NewClientRequest(model)
	req.
		WithMiddlewares(streamMW).
		WithMessages(chat.NewUserMessage("seed")).
		WithTools(echoTool)

	var assistantChunks, toolChunks int
	for resp, err := range req.Stream().Response(context.Background()) {
		if err != nil {
			t.Fatal(err)
		}
		if resp == nil || resp.Result == nil {
			continue
		}
		switch {
		case resp.Result.AssistantMessage != nil:
			assistantChunks++
		case resp.Result.ToolMessage != nil:
			toolChunks++
			if len(resp.Result.ToolMessage.ToolReturns) != 1 {
				t.Errorf("tool message returns = %d, want 1", len(resp.Result.ToolMessage.ToolReturns))
			}
			if resp.Result.ToolMessage.ToolReturns[0].Result != "echoed:{\"x\":1}" {
				t.Errorf("tool result = %q", resp.Result.ToolMessage.ToolReturns[0].Result)
			}
		}
	}

	if streamCalls != 2 {
		t.Fatalf("model stream invoked %d times, want 2", streamCalls)
	}
	if toolChunks != 1 {
		t.Errorf("expected exactly 1 ToolMessage yield between turns, got %d", toolChunks)
	}
	if assistantChunks < 2 {
		t.Errorf("expected at least 2 assistant deltas, got %d", assistantChunks)
	}
}

// TestToolMiddleware_MaxIterationsCap verifies the loop aborts with a
// MaxToolIterationsError instead of recursing forever when the model keeps
// requesting tools and the tool result never satisfies it.
func TestToolMiddleware_MaxIterationsCap(t *testing.T) {
	model := newFakeChatModel(t)

	calls := 0
	// Always ask for the tool again — a loop that never terminates.
	model.respond = func(req *chat.Request) (*chat.Response, error) {
		calls++
		return responseWithToolCall(t, "echo", `{"x":1}`), nil
	}

	echoTool := mustNewCallable(t, "echo", false, func(_ context.Context, args string) (string, error) {
		return "echoed:" + args, nil
	})

	callMW, _ := chat.NewToolMiddleware(chat.ToolLoopConfig{MaxIterations: 3})
	req, _ := chat.NewClientRequest(model)
	req.
		WithMiddlewares(callMW).
		WithMessages(chat.NewUserMessage("seed")).
		WithTools(echoTool)

	_, err := req.Call().Response(context.Background())
	var capErr *chat.MaxToolIterationsError
	if !errors.As(err, &capErr) {
		t.Fatalf("expected MaxToolIterationsError, got %v", err)
	}
	if capErr.Limit != 3 {
		t.Fatalf("Limit = %d, want 3", capErr.Limit)
	}
	if calls != 3 {
		t.Fatalf("model invoked %d times, want exactly 3 (the cap)", calls)
	}
}

// TestToolMiddleware_UnknownToolThrowsByDefault confirms the default
// behavior is unchanged: a call to an unregistered tool aborts the request.
func TestToolMiddleware_UnknownToolThrowsByDefault(t *testing.T) {
	model := newFakeChatModel(t)
	model.respond = func(req *chat.Request) (*chat.Response, error) {
		return responseWithToolCall(t, "ghost", `{}`), nil
	}

	callMW, _ := chat.NewToolMiddleware()
	req, _ := chat.NewClientRequest(model)
	req.WithMiddlewares(callMW).WithMessages(chat.NewUserMessage("seed"))

	if _, err := req.Call().Response(context.Background()); err == nil {
		t.Fatal("expected error for unregistered tool, got nil")
	}
}

// TestToolMiddleware_UnknownToolFeedback verifies that with feedback enabled
// the loop hands the model an error result for the missing tool (so it can
// recover) and continues, rather than aborting.
func TestToolMiddleware_UnknownToolFeedback(t *testing.T) {
	model := newFakeChatModel(t)

	calls := 0
	model.respond = func(req *chat.Request) (*chat.Response, error) {
		calls++
		if calls == 1 {
			return responseWithToolCall(t, "ghost", `{}`), nil // not registered
		}
		return responseWithText("recovered"), nil
	}

	real := mustNewCallable(t, "real", false, func(context.Context, string) (string, error) {
		return "ok", nil
	})

	callMW, _ := chat.NewToolMiddleware(chat.ToolLoopConfig{FeedbackOnUnknownTool: true})
	req, _ := chat.NewClientRequest(model)
	req.WithMiddlewares(callMW).WithMessages(chat.NewUserMessage("seed")).WithTools(real)

	resp, err := req.Call().Response(context.Background())
	if err != nil {
		t.Fatalf("expected recovery, got error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("model invoked %d times, want 2 (error result fed back, then recovery)", calls)
	}
	if resp.Result.AssistantMessage.JoinedText() != "recovered" {
		t.Fatalf("text = %q, want recovered", resp.Result.AssistantMessage.JoinedText())
	}
}

// TestToolMiddleware_EmptyResponseFeedback verifies the one-shot nudge: an
// empty reply triggers a single re-prompt when the policy is enabled.
func TestToolMiddleware_EmptyResponseFeedback(t *testing.T) {
	model := newFakeChatModel(t)

	calls := 0
	model.respond = func(req *chat.Request) (*chat.Response, error) {
		calls++
		if calls == 1 {
			return responseWithText(""), nil // empty
		}
		return responseWithText("now answered"), nil
	}

	callMW, _ := chat.NewToolMiddleware(chat.ToolLoopConfig{FeedbackOnEmptyResponse: true})
	req, _ := chat.NewClientRequest(model)
	req.WithMiddlewares(callMW).WithMessages(chat.NewUserMessage("seed"))

	resp, err := req.Call().Response(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("model invoked %d times, want 2 (empty, then nudged)", calls)
	}
	if resp.Result.AssistantMessage.JoinedText() != "now answered" {
		t.Fatalf("text = %q, want 'now answered'", resp.Result.AssistantMessage.JoinedText())
	}
}

// TestToolMiddleware_EmptyResponseNudgeIsOneShot confirms the nudge fires at
// most once: a persistently empty model returns the empty reply rather than
// looping.
func TestToolMiddleware_EmptyResponseNudgeIsOneShot(t *testing.T) {
	model := newFakeChatModel(t)

	calls := 0
	model.respond = func(req *chat.Request) (*chat.Response, error) {
		calls++
		return responseWithText(""), nil // always empty
	}

	callMW, _ := chat.NewToolMiddleware(chat.ToolLoopConfig{FeedbackOnEmptyResponse: true})
	req, _ := chat.NewClientRequest(model)
	req.WithMiddlewares(callMW).WithMessages(chat.NewUserMessage("seed"))

	if _, err := req.Call().Response(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("model invoked %d times, want exactly 2 (original + one nudge)", calls)
	}
}

type artifactChart struct{ Bars int }

// TestArtifactTool_CallReturnsContentOnly confirms an ArtifactTool still
// satisfies the plain Tool contract (Call yields just the text).
func TestArtifactTool_CallReturnsContentOnly(t *testing.T) {
	at, err := chat.NewArtifactTool(
		chat.ToolDefinition{Name: "x", InputSchema: "{}"},
		chat.ToolMetadata{},
		func(context.Context, string) (chat.ToolResult, error) {
			return chat.ToolResult{Content: "txt", Artifact: artifactChart{Bars: 1}}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := at.Call(context.Background(), "{}")
	if err != nil || got != "txt" {
		t.Fatalf("Call = %q, %v; want \"txt\", nil", got, err)
	}
}

// TestToolMiddleware_ArtifactRidesToolMessage confirms a tool's artifact
// reaches the tool message (for non-LLM consumers) while the LLM-visible
// Result carries only the text.
func TestToolMiddleware_ArtifactRidesToolMessage(t *testing.T) {
	model := newFakeChatModel(t)

	streamCalls := 0
	model.streamRespond = func(req *chat.Request) []*chat.Response {
		streamCalls++
		if streamCalls == 1 {
			return []*chat.Response{responseWithToolCall(t, "render", `{}`)}
		}
		return []*chat.Response{responseWithText("done")}
	}

	renderTool, err := chat.NewArtifactTool(
		chat.ToolDefinition{Name: "render", InputSchema: "{}"},
		chat.ToolMetadata{},
		func(context.Context, string) (chat.ToolResult, error) {
			return chat.ToolResult{Content: "rendered a chart", Artifact: artifactChart{Bars: 3}}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, streamMW := chat.NewToolMiddleware()
	req, _ := chat.NewClientRequest(model)
	req.WithMiddlewares(streamMW).WithMessages(chat.NewUserMessage("seed")).WithTools(renderTool)

	var (
		gotResult   string
		gotArtifact any
	)
	for resp, err := range req.Stream().Response(context.Background()) {
		if err != nil {
			t.Fatal(err)
		}
		if resp == nil || resp.Result == nil || resp.Result.ToolMessage == nil {
			continue
		}
		ret := resp.Result.ToolMessage.ToolReturns[0]
		gotResult = ret.Result
		gotArtifact = ret.Artifact
	}

	if gotResult != "rendered a chart" {
		t.Fatalf("tool result text = %q, want \"rendered a chart\"", gotResult)
	}
	c, ok := gotArtifact.(artifactChart)
	if !ok || c.Bars != 3 {
		t.Fatalf("artifact = %#v, want artifactChart{Bars:3}", gotArtifact)
	}
}

// TestToolMiddleware_PassthroughWithoutToolCalls verifies the middleware
// is invisible when the LLM doesn't request any tools.
func TestToolMiddleware_PassthroughWithoutToolCalls(t *testing.T) {
	model := newFakeChatModel(t)
	model.respond = func(*chat.Request) (*chat.Response, error) {
		return responseWithText("plain reply"), nil
	}

	callMW, _ := chat.NewToolMiddleware()
	req, _ := chat.NewClientRequest(model)
	req.
		WithMiddlewares(callMW).
		WithMessages(chat.NewUserMessage("hi"))

	resp, err := req.Call().Response(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Result.AssistantMessage.JoinedText() != "plain reply" {
		t.Fatalf("Text = %q", resp.Result.AssistantMessage.JoinedText())
	}
}
