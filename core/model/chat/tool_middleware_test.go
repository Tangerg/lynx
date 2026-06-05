package chat_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/memory"
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

// --- HITL interrupt + conversation-driven resume (R-model) -----------------

// interruptErr is a tool error that interrupts the loop for human input,
// the way agent/hitl.InterruptError does — structurally (duck-typed), with
// no import.
type interruptErr struct{}

func (interruptErr) Error() string           { return "interrupt: awaiting approval" }
func (interruptErr) ToolLoopInterrupt() bool { return true }

// twoToolCallResponse is a model reply requesting two tool calls in order.
func twoToolCallResponse(first, second string) *chat.Response {
	resp, _ := chat.NewResponse(
		&chat.Result{
			AssistantMessage: chat.NewAssistantMessage(chat.MessageParams{
				Parts: []chat.OutputPart{
					&chat.ToolCallPart{ID: "c1", Name: first, Arguments: "{}"},
					&chat.ToolCallPart{ID: "c2", Name: second, Arguments: "{}"},
				},
			}),
			Metadata: &chat.ResultMetadata{FinishReason: chat.FinishReasonToolCalls},
		},
		&chat.ResponseMetadata{},
	)
	return resp
}

// collectStream drains a stream, returning the accumulated conversation
// (assistant + tool messages) and the final text, plus any error.
func collectStream(seq func(func(*chat.Response, error) bool)) (msgs []chat.Message, finalText string, err error) {
	for resp, e := range seq {
		if e != nil {
			return msgs, finalText, e
		}
		if resp == nil || resp.Result == nil {
			continue
		}
		if am := resp.Result.AssistantMessage; am != nil {
			if txt := am.JoinedText(); txt != "" {
				finalText = txt
			}
		}
	}
	return msgs, finalText, nil
}

// TestToolMiddleware_InterruptThenResume is the headline R-model test for
// the interrupt/resume model: a gated tool interrupts mid-round; the loop
// returns a *ToolLoopInterrupted carrying the resumable conversation
// (assistant tool-call message + the result of the call that already ran).
// Feeding that conversation back resumes the turn — executing ONLY the
// still-pending (now-approved) call, never re-invoking the model for the
// completed round and never re-executing the tool that already ran.
func TestToolMiddleware_InterruptThenResume(t *testing.T) {
	model := newFakeChatModel(t)

	modelCalls := 0
	model.streamRespond = func(*chat.Request) []*chat.Response {
		modelCalls++
		if modelCalls == 1 {
			return []*chat.Response{twoToolCallResponse("free", "gated")} // round 1
		}
		return []*chat.Response{responseWithText("done")} // round 2 (resume only)
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

	_, streamMW := chat.NewToolMiddleware()

	// --- First run: free runs, gated interrupts. ---
	req1, _ := chat.NewClientRequest(model)
	req1.WithMiddlewares(streamMW).WithMessages(chat.NewUserMessage("seed")).WithTools(freeTool, gatedTool)

	var firstErr error
	for _, e := range req1.Stream().Response(context.Background()) {
		if e != nil {
			firstErr = e
			break
		}
	}
	var interrupted *chat.ToolLoopInterrupted
	if !errors.As(firstErr, &interrupted) {
		t.Fatalf("first run error = %v, want *chat.ToolLoopInterrupted", firstErr)
	}
	if !errors.As(firstErr, new(interruptErr)) {
		t.Fatal("interrupt cause should be reachable via errors.As")
	}
	if modelCalls != 1 || freeRuns != 1 || gatedRuns != 0 {
		t.Fatalf("after interrupt: model=%d free=%d gated=%d, want 1/1/0", modelCalls, freeRuns, gatedRuns)
	}
	// Conversation tail must be [assistant(free,gated), tool(free result)].
	conv := interrupted.Conversation
	if len(conv) < 2 {
		t.Fatalf("conversation too short: %d messages", len(conv))
	}
	if tm, ok := conv[len(conv)-1].(*chat.ToolMessage); !ok || len(tm.ToolReturns) != 1 || tm.ToolReturns[0].Name != "free" {
		t.Fatalf("conversation tail tool message = %+v, want one 'free' result", conv[len(conv)-1])
	}

	// --- Resume: approve, feed the saved conversation back. ---
	approved = true
	req2, _ := chat.NewClientRequest(model)
	req2.WithMiddlewares(streamMW).WithMessages(conv...).WithTools(freeTool, gatedTool)

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

// toolCallResponseID builds a tool-call response with an explicit call id,
// so a multi-round turn uses distinct ids (call_1, call_2, …) and the
// tool_call ↔ result correlation can be checked.
func toolCallResponseID(t *testing.T, id, name string) *chat.Response {
	t.Helper()
	resp, err := chat.NewResponse(&chat.Result{
		AssistantMessage: chat.NewAssistantMessage(chat.MessageParams{
			Parts: []chat.OutputPart{&chat.ToolCallPart{ID: id, Name: name, Arguments: "{}"}},
		}),
		Metadata: &chat.ResultMetadata{FinishReason: chat.FinishReasonToolCalls},
	}, &chat.ResponseMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func countToolMsgs(msgs []chat.Message) int {
	n := 0
	for _, m := range msgs {
		if m.Type() == chat.MessageTypeTool {
			n++
		}
	}
	return n
}

// assertValidToolHistory enforces the provider invariants on a stored
// conversation: every tool message immediately follows an assistant message
// with tool calls (the rule deepseek 400s on), every tool_call id has a
// matching tool return, and every tool return matches a tool_call id.
func assertValidToolHistory(t *testing.T, msgs []chat.Message) {
	t.Helper()
	calls := map[string]bool{}  // tool_call ids seen on assistant messages
	answered := map[string]bool{}
	for i, m := range msgs {
		switch msg := m.(type) {
		case *chat.AssistantMessage:
			for _, c := range msg.CollectToolCalls() {
				calls[c.ID] = true
			}
		case *chat.ToolMessage:
			prev, ok := msgs[i-1].(*chat.AssistantMessage)
			if i == 0 || !ok || !prev.HasToolCalls() {
				t.Fatalf("tool message at %d not preceded by assistant(tool_calls): history=%s", i, historyShape(msgs))
			}
			for _, ret := range msg.ToolReturns {
				answered[ret.ID] = true
				if !calls[ret.ID] {
					t.Fatalf("tool return id %q has no matching tool_call: history=%s", ret.ID, historyShape(msgs))
				}
			}
		}
	}
	for id := range calls {
		if !answered[id] {
			t.Fatalf("tool_call id %q has no tool response (provider rejects unanswered calls): history=%s", id, historyShape(msgs))
		}
	}
}

func historyShape(msgs []chat.Message) string {
	parts := make([]string, 0, len(msgs))
	for _, m := range msgs {
		switch msg := m.(type) {
		case *chat.AssistantMessage:
			if msg.HasToolCalls() {
				parts = append(parts, "assistant(tool_calls)")
			} else {
				parts = append(parts, "assistant(text)")
			}
		default:
			parts = append(parts, string(m.Type()))
		}
	}
	return strings.Join(parts, " → ")
}

// TestMemory_SequentialMultiRoundTurn_ValidHistory drives a turn that calls
// a DIFFERENT tool each round (alpha, then beta, then answers) through the
// real memory + tool middlewares, then checks the persisted conversation is
// a valid provider sequence. Guards the deep pitfall where the accumulator
// merges all rounds into one assistant + keeps only the last round's tool
// results, orphaning earlier tool_calls.
func TestMemory_SequentialMultiRoundTurn_ValidHistory(t *testing.T) {
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

	store := memory.NewInMemoryStore()
	memCallMW, memStreamMW, err := memory.NewMiddleware(store)
	if err != nil {
		t.Fatal(err)
	}
	_, toolStreamMW := chat.NewToolMiddleware()

	req, _ := chat.NewClientRequest(model)
	req.WithMiddlewares(memCallMW, memStreamMW, toolStreamMW).
		WithParams(map[string]any{memory.ConversationIDKey: "c1"}).
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
