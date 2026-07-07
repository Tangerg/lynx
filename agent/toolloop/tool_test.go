package toolloop

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	chatconversation "github.com/Tangerg/lynx/core/model/chat/conversation"
	"github.com/Tangerg/lynx/core/model/chat/history"
	historymw "github.com/Tangerg/lynx/core/model/chat/middleware/history"
)

// --- tool-loop middleware --------------------------------------------------

// TestToolLoop_RecursiveLoop checks the headline behavior:
//  1. model returns a tool-call response,
//  2. middleware executes the tool,
//  3. middleware re-prompts the model with the tool result,
//  4. the second model invocation returns a regular reply,
//  5. middleware returns that reply to the caller.
func TestToolLoop_RecursiveLoop(t *testing.T) {
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

	callMW, _ := NewMiddleware()
	req, _ := chat.NewClientRequest(model)
	req.
		WithCallMiddlewares(callMW).
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

// TestToolLoop_DirectReturn confirms the short-circuit path:
// when every called tool is return-direct the middleware skips the
// follow-up LLM call and returns the tool result wrapped as a response.
func TestToolLoop_DirectReturn(t *testing.T) {
	model := newFakeChatModel(t)

	calls := 0
	model.respond = func(req *chat.Request) (*chat.Response, error) {
		calls++
		return responseWithToolCall(t, "notify", `{}`), nil
	}

	notify := mustNewCallable(t, "notify", true, func(context.Context, string) (string, error) {
		return "sent", nil
	})

	callMW, _ := NewMiddleware()
	req, _ := chat.NewClientRequest(model)
	req.
		WithCallMiddlewares(callMW).
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

// TestToolLoop_StreamEmitsToolMessageBetweenTurns checks the
// streaming-path invariant from MESSAGE_PARTS_DESIGN §8.4: the
// runtime-injected ToolMessage MUST be yielded to the external
// consumer as its own delta between assistant turns, so the
// downstream timeline matches the message history fed to the next
// model call.
func TestToolLoop_StreamEmitsToolMessageBetweenTurns(t *testing.T) {
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

	_, streamMW := NewMiddleware()
	req, _ := chat.NewClientRequest(model)
	req.
		WithStreamMiddlewares(streamMW).
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

// TestToolLoop_MaxIterationsCap verifies the loop aborts with a
// MaxIterationsError instead of recursing forever when the model keeps
// requesting tools and the tool result never satisfies it.
func TestToolLoop_MaxIterationsCap(t *testing.T) {
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

	callMW, _ := NewMiddleware(Config{MaxIterations: 3})
	req, _ := chat.NewClientRequest(model)
	req.
		WithCallMiddlewares(callMW).
		WithMessages(chat.NewUserMessage("seed")).
		WithTools(echoTool)

	_, err := req.Call().Response(context.Background())
	capErr, ok := errors.AsType[*MaxIterationsError](err)
	if !ok {
		t.Fatalf("expected MaxIterationsError, got %v", err)
	}
	if capErr.Limit != 3 {
		t.Fatalf("Limit = %d, want 3", capErr.Limit)
	}
	if calls != 3 {
		t.Fatalf("model invoked %d times, want exactly 3 (the cap)", calls)
	}
}

// TestToolLoop_UnknownToolFeedback verifies that by default the loop
// hands the model an error result for the missing tool (so it can
// recover) and continues, rather than aborting.
func TestToolLoop_UnknownToolFeedback(t *testing.T) {
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

	// Unknown-tool recovery is the unconditional default now — no config knob.
	callMW, _ := NewMiddleware()
	req, _ := chat.NewClientRequest(model)
	req.WithCallMiddlewares(callMW).WithMessages(chat.NewUserMessage("seed")).WithTools(real)

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

// TestToolLoop_EmptyResponseFeedback verifies the one-shot nudge: an
// empty reply triggers a single re-prompt when the policy is enabled.
func TestToolLoop_EmptyResponseFeedback(t *testing.T) {
	model := newFakeChatModel(t)

	calls := 0
	model.respond = func(req *chat.Request) (*chat.Response, error) {
		calls++
		if calls == 1 {
			return responseWithText(""), nil // empty
		}
		return responseWithText("now answered"), nil
	}

	callMW, _ := NewMiddleware(Config{FeedbackOnEmptyResponse: true})
	req, _ := chat.NewClientRequest(model)
	req.WithCallMiddlewares(callMW).WithMessages(chat.NewUserMessage("seed"))

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

// TestToolLoop_EmptyResponseNudgeIsOneShot confirms the nudge fires at
// most once: a persistently empty model returns the empty reply rather than
// looping.
func TestToolLoop_EmptyResponseNudgeIsOneShot(t *testing.T) {
	model := newFakeChatModel(t)

	calls := 0
	model.respond = func(req *chat.Request) (*chat.Response, error) {
		calls++
		return responseWithText(""), nil // always empty
	}

	callMW, _ := NewMiddleware(Config{FeedbackOnEmptyResponse: true})
	req, _ := chat.NewClientRequest(model)
	req.WithCallMiddlewares(callMW).WithMessages(chat.NewUserMessage("seed"))

	if _, err := req.Call().Response(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("model invoked %d times, want exactly 2 (original + one nudge)", calls)
	}
}

// --- HITL interrupt: yield resumable tail, resume AT the pending call ------

// TestToolLoop_InterruptThenResume is the headline R-model test: a gated
// tool halts a round mid-way; the loop yields a FinishReasonInterrupt response
// carrying the resumable tail (this round's assistant tool-call message + the
// result of the call that already ran) and propagates the tool's Halt
// cause. Feeding that tail back resumes the turn — executing ONLY the
// still-pending (now-approved) call, NEVER re-invoking the model for the
// completed round and NEVER re-running the call that already ran.
func TestToolLoop_InterruptThenResume(t *testing.T) {
	model := newFakeChatModel(t)
	modelCalls := 0
	model.streamRespond = func(*chat.Request) []*chat.Response {
		modelCalls++
		if modelCalls == 1 {
			return []*chat.Response{twoToolCallResponse("free", "gated")} // round 1
		}
		return []*chat.Response{responseWithText("done")} // round 2 (resume synthesis)
	}

	var freeRuns, gatedRuns int
	approved := false
	freeTool := mustNewCallable(t, "free", false, func(context.Context, string) (string, error) {
		freeRuns++
		return "free-ok", nil
	})
	gatedTool := mustNewCallable(t, "gated", false, func(context.Context, string) (string, error) {
		if !approved {
			return "", interruptErr{}
		}
		gatedRuns++
		return "gated-ok", nil
	})

	_, streamMW := NewMiddleware()

	// --- First run: free runs, gated halts. The loop yields a
	//     FinishReasonInterrupt response (the tail) then the interruptErr. ---
	req1, _ := chat.NewClientRequest(model)
	req1.WithStreamMiddlewares(streamMW).WithMessages(chat.NewUserMessage("seed")).WithTools(freeTool, gatedTool)

	var (
		tail     []chat.Message
		firstErr error
	)
	for resp, e := range req1.Stream().Response(context.Background()) {
		if e != nil {
			firstErr = e
			break
		}
		if resp != nil && resp.Result != nil && resp.Result.Metadata != nil &&
			resp.Result.Metadata.FinishReason == FinishReasonInterrupt {
			tail = append(tail, resp.Result.AssistantMessage)
			if resp.Result.ToolMessage != nil {
				tail = append(tail, resp.Result.ToolMessage)
			}
		}
	}
	if !errors.As(firstErr, new(interruptErr)) {
		t.Fatalf("first run error = %v, want the tool's interruptErr", firstErr)
	}
	if modelCalls != 1 || freeRuns != 1 || gatedRuns != 0 {
		t.Fatalf("after interrupt: model=%d free=%d gated=%d, want 1/1/0", modelCalls, freeRuns, gatedRuns)
	}
	// Tail must be [assistant(free,gated), tool(free result)].
	if len(tail) != 2 {
		t.Fatalf("interrupt tail = %d messages, want 2 (assistant + partial tool)", len(tail))
	}
	if tm, ok := tail[1].(*chat.ToolMessage); !ok || len(tm.ToolReturns) != 1 || tm.ToolReturns[0].Name != "free" {
		t.Fatalf("tail tool message = %+v, want one 'free' result", tail[1])
	}

	// --- Resume: approve, feed the tail back. Only 'gated' runs; the model is
	//     NOT re-invoked for round 1. ---
	approved = true
	req2, _ := chat.NewClientRequest(model)
	req2.WithStreamMiddlewares(streamMW).WithMessages(tail...).WithTools(freeTool, gatedTool)

	_, finalText, err := collectStream(req2.Stream().Response(context.Background()))
	if err != nil {
		t.Fatalf("resume run error: %v", err)
	}
	if modelCalls != 2 {
		t.Fatalf("total model calls = %d, want 2 (round 1 NOT re-invoked on resume)", modelCalls)
	}
	if freeRuns != 1 {
		t.Fatalf("free ran %d times total, want 1 (completed call NOT re-executed)", freeRuns)
	}
	if gatedRuns != 1 {
		t.Fatalf("gated ran %d times, want 1 (executed once, on resume)", gatedRuns)
	}
	if finalText != "done" {
		t.Fatalf("final text = %q, want \"done\"", finalText)
	}
}

// --- persisted-history validity across multi-round / HITL turns -----------

// TestHistory_SequentialMultiRoundTurn_ValidHistory drives a turn that calls
// a DIFFERENT tool each round (alpha, then beta, then answers) through the
// real history + tool middlewares, then checks the persisted conversation is
// a valid provider sequence. Guards the deep pitfall where the accumulator
// merges all rounds into one assistant + keeps only the last round's tool
// results, orphaning earlier tool_calls.
func TestHistory_SequentialMultiRoundTurn_ValidHistory(t *testing.T) {
	model := newFakeChatModel(t)
	model.streamRespond = func(req *chat.Request) []*chat.Response {
		switch countToolMsgs(req.Messages) {
		case 0:
			return []*chat.Response{toolCallResponseID(t, "call_1", "alpha")}
		case 1:
			return []*chat.Response{toolCallResponseID(t, "call_2", "beta")}
		default:
			return []*chat.Response{responseWithText("done")}
		}
	}
	alpha := mustNewCallable(t, "alpha", false, func(context.Context, string) (string, error) { return "a-ok", nil })
	beta := mustNewCallable(t, "beta", false, func(context.Context, string) (string, error) { return "b-ok", nil })

	store := history.NewInMemoryStore()
	historyCallMW, historyStreamMW, err := historymw.NewMiddleware(store)
	if err != nil {
		t.Fatal(err)
	}
	_, toolStreamMW := NewMiddleware()

	// Tool middleware is OUTERMOST, history INNERMOST (model-adjacent): the
	// tool loop drives the rounds and hands each round's new messages down
	// to history, which loads, splices, and persists. First in the
	// slice = outermost.
	req, _ := chat.NewClientRequest(model)
	req.WithCallMiddlewares(historyCallMW).
		WithStreamMiddlewares(toolStreamMW, historyStreamMW).
		WithParams(map[string]any{chatconversation.IDKey: "c1"}).
		WithSystemPrompt("sys").
		WithUserPrompt("go").
		WithTools(alpha, beta)

	for _, e := range req.Stream().Response(context.Background()) {
		if e != nil {
			t.Fatalf("stream error: %v", e)
		}
	}

	stored, _ := store.Read(context.Background(), "c1")
	assertValidToolHistory(t, stored)
}

// TestToolLoop_PassthroughWithoutToolCalls verifies the middleware
// is invisible when the LLM doesn't request any tools.
func TestToolLoop_PassthroughWithoutToolCalls(t *testing.T) {
	model := newFakeChatModel(t)
	model.respond = func(*chat.Request) (*chat.Response, error) {
		return responseWithText("plain reply"), nil
	}

	callMW, _ := NewMiddleware()
	req, _ := chat.NewClientRequest(model)
	req.
		WithCallMiddlewares(callMW).
		WithMessages(chat.NewUserMessage("hi"))

	resp, err := req.Call().Response(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Result.AssistantMessage.JoinedText() != "plain reply" {
		t.Fatalf("Text = %q", resp.Result.AssistantMessage.JoinedText())
	}
}
