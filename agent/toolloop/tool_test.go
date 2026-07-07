package toolloop

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	chatconversation "github.com/Tangerg/lynx/core/model/chat/conversation"
	"github.com/Tangerg/lynx/core/model/chat/history"
	historymw "github.com/Tangerg/lynx/core/model/chat/middleware/history"
)

// --- registry lifecycle ----------------------------------------------------

// TestToolRegistry_Lifecycle covers the mutation surface — register
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
	// New contract: the continuation carries the turn's system header (none in
	// this request) + this round's assistant tool-call message + its tool
	// result — the atomic exchange the history layer persists together. It does
	// NOT carry the prior conversation (the history middleware owns stored
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
// tool that returns an ordinary error fails RECOVERABLY — the error text is
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
// the contract: a ToolLoopAbort error is NOT fed back — it propagates and
// stops the loop, so genuinely unrecoverable failures end the run.
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
