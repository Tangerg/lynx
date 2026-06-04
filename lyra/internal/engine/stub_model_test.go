package engine

import (
	"context"
	"iter"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
)

// delegatingStubModel exercises the `task` delegation tool. A turn whose
// user message mentions "delegate" calls task once, then (after the tool
// result) returns a final answer. A turn that doesn't (the sub-agent's
// own turn, whose prompt is "do the subtask") returns text directly — so
// the delegation resolves in one level with no recursion.
type delegatingStubModel struct{ defaults *chat.Options }

func newDelegatingStubModel() *delegatingStubModel {
	opts, _ := chat.NewOptions("stub-delegating")
	return &delegatingStubModel{defaults: opts}
}

func (m *delegatingStubModel) DefaultOptions() chat.Options { return *m.defaults }
func (m *delegatingStubModel) Metadata() chat.ModelMetadata {
	return chat.ModelMetadata{Provider: "stub"}
}

func (m *delegatingStubModel) Call(_ context.Context, req *chat.Request) (*chat.Response, error) {
	switch {
	case hasToolMessage(req.Messages):
		return responseWithText("main: subtask done")
	case mentionsDelegate(req.Messages):
		return responseWithToolCall("task", `{"prompt":"do the subtask"}`)
	default:
		return responseWithText("subtask: result")
	}
}

func (m *delegatingStubModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chat.Response, error) bool) { yield(resp, err) }
}

// cwdDelegatingStubModel is delegatingStubModel's cwd-aware cousin: the main
// turn delegates via `task`, and the sub-agent (instead of replying text)
// asks bash to create a marker file with a RELATIVE path. The marker lands in
// whatever working directory the sub-agent's tools run in — so a test can
// assert the sub-agent inherited the turn's Cwd by checking where the file
// appeared.
type cwdDelegatingStubModel struct{ defaults *chat.Options }

func newCwdDelegatingStubModel() *cwdDelegatingStubModel {
	opts, _ := chat.NewOptions("stub-cwd-delegating")
	return &cwdDelegatingStubModel{defaults: opts}
}

func (m *cwdDelegatingStubModel) DefaultOptions() chat.Options { return *m.defaults }
func (m *cwdDelegatingStubModel) Metadata() chat.ModelMetadata {
	return chat.ModelMetadata{Provider: "stub"}
}

func (m *cwdDelegatingStubModel) Call(_ context.Context, req *chat.Request) (*chat.Response, error) {
	switch {
	case hasToolMessage(req.Messages):
		// Round 2 — distinguish the main turn (delegated) from the
		// sub-agent turn (ran bash) by the user message.
		if mentionsDelegate(req.Messages) {
			return responseWithText("main: subtask done")
		}
		return responseWithText("subtask done")
	case mentionsDelegate(req.Messages):
		return responseWithToolCall("task", `{"prompt":"create the marker"}`)
	default:
		// Sub-agent's first round: write a marker via a relative path so
		// where it lands reflects the inherited working directory.
		return responseWithToolCall("bash", `{"command":"touch subtask_was_here.txt"}`)
	}
}

func (m *cwdDelegatingStubModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chat.Response, error) bool) { yield(resp, err) }
}

func mentionsDelegate(msgs []chat.Message) bool {
	for _, msg := range msgs {
		if u, ok := msg.(*chat.UserMessage); ok && strings.Contains(u.Text, "delegate") {
			return true
		}
	}
	return false
}

// stubModel is a deterministic chat.Model used by engine + chat-service
// tests to drive the agent loop without a real LLM.
//
// State machine:
//
//   - Round 1 (no tool message in history): emit a single tool call
//     against ToolName with ToolArgs.
//   - Round 2 (tool message present): emit the configured FinalText.
//
// Two-round behavior is enough to exercise the ToolMiddleware tool
// loop end-to-end (tool dispatch → result feedback → final reply)
// while staying fully offline.
type stubModel struct {
	ToolName  string // tool the stub asks for in round 1
	ToolArgs  string // arguments JSON for that tool
	FinalText string // assistant text in round 2

	defaults *chat.Options
}

// newStubModel returns a stub configured to call `toolName` with
// `toolArgs` on round 1 and produce `finalText` on round 2. The
// stub's model id is "stub-model" — never speaks to a real endpoint.
func newStubModel(toolName, toolArgs, finalText string) *stubModel {
	opts, _ := chat.NewOptions("stub-model")
	return &stubModel{
		ToolName:  toolName,
		ToolArgs:  toolArgs,
		FinalText: finalText,
		defaults:  opts,
	}
}

func (m *stubModel) DefaultOptions() chat.Options { return *m.defaults }
func (m *stubModel) Metadata() chat.ModelMetadata { return chat.ModelMetadata{Provider: "stub"} }

func (m *stubModel) Call(_ context.Context, req *chat.Request) (*chat.Response, error) {
	if hasToolMessage(req.Messages) {
		return responseWithText(m.FinalText)
	}
	return responseWithToolCall(m.ToolName, m.ToolArgs)
}

func (m *stubModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chat.Response, error) bool) { yield(resp, err) }
}

// streamingStubModel ignores tool calls entirely and yields the
// configured Chunks one at a time so streaming-path tests can
// assert that each chunk lands on OnMessageDelta independently.
type streamingStubModel struct {
	Chunks   []string
	defaults *chat.Options
}

func newStreamingStubModel(chunks ...string) *streamingStubModel {
	opts, _ := chat.NewOptions("stub-model-streaming")
	return &streamingStubModel{Chunks: chunks, defaults: opts}
}

func (m *streamingStubModel) DefaultOptions() chat.Options { return *m.defaults }
func (m *streamingStubModel) Metadata() chat.ModelMetadata {
	return chat.ModelMetadata{Provider: "stub"}
}

// Call concatenates the chunks into one response — used when a non-
// stream caller asks for the full reply (the engine doesn't, but
// chat.Model requires both methods).
func (m *streamingStubModel) Call(_ context.Context, _ *chat.Request) (*chat.Response, error) {
	all := ""
	for _, c := range m.Chunks {
		all += c
	}
	return responseWithText(all)
}

func (m *streamingStubModel) Stream(_ context.Context, _ *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		for _, chunk := range m.Chunks {
			resp, err := responseWithText(chunk)
			if !yield(resp, err) {
				return
			}
		}
	}
}

// historyAwareStub remembers how many messages it saw on each Call.
// Used by multi-turn tests to confirm chat-memory loads prior turns
// before passing the request to the model.
type historyAwareStub struct {
	defaults    *chat.Options
	seenLengths []int
}

func newHistoryAwareStub() *historyAwareStub {
	opts, _ := chat.NewOptions("stub-model-history")
	return &historyAwareStub{defaults: opts}
}

func (m *historyAwareStub) DefaultOptions() chat.Options { return *m.defaults }
func (m *historyAwareStub) Metadata() chat.ModelMetadata { return chat.ModelMetadata{Provider: "stub"} }

func (m *historyAwareStub) Call(_ context.Context, req *chat.Request) (*chat.Response, error) {
	m.seenLengths = append(m.seenLengths, len(req.Messages))
	return responseWithText("ok")
}

func (m *historyAwareStub) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chat.Response, error) bool) { yield(resp, err) }
}

func hasToolMessage(messages []chat.Message) bool {
	for _, msg := range messages {
		if msg.Type() == chat.MessageTypeTool {
			return true
		}
	}
	return false
}

func responseWithText(text string) (*chat.Response, error) {
	return chat.NewResponse(
		&chat.Result{
			AssistantMessage: chat.NewAssistantMessage(text),
			Metadata:         &chat.ResultMetadata{FinishReason: chat.FinishReasonStop},
		},
		&chat.ResponseMetadata{},
	)
}

func responseWithToolCall(name, args string) (*chat.Response, error) {
	calls := []*chat.ToolCallPart{{ID: "call_1", Name: name, Arguments: args}}
	return chat.NewResponse(
		&chat.Result{
			AssistantMessage: chat.NewAssistantMessage(calls),
			Metadata:         &chat.ResultMetadata{FinishReason: chat.FinishReasonToolCalls},
		},
		&chat.ResponseMetadata{},
	)
}

// usageStubModel runs the same two-round bash dance as stubModel but
// stamps a configurable Usage on each round's Response so per-turn
// usage accumulation can be asserted end-to-end.
type usageStubModel struct {
	round1Usage chat.Usage
	round2Usage chat.Usage
	defaults    *chat.Options
}

func newUsageStubModel(round1, round2 chat.Usage) *usageStubModel {
	opts, _ := chat.NewOptions("stub-model-usage")
	return &usageStubModel{round1Usage: round1, round2Usage: round2, defaults: opts}
}

func (m *usageStubModel) DefaultOptions() chat.Options { return *m.defaults }
func (m *usageStubModel) Metadata() chat.ModelMetadata { return chat.ModelMetadata{Provider: "stub"} }

func (m *usageStubModel) Call(_ context.Context, req *chat.Request) (*chat.Response, error) {
	var (
		resp *chat.Response
		err  error
	)
	if hasToolMessage(req.Messages) {
		resp, err = responseWithTextAndUsage("done", m.round2Usage)
	} else {
		resp, err = responseWithToolCallAndUsage("bash", `{"command":"echo lyra"}`, m.round1Usage)
	}
	// Stamp the served model so per-model usage roll-up is exercised.
	if resp != nil && resp.Metadata != nil {
		resp.Metadata.Model = "stub-usage-model"
	}
	return resp, err
}

func (m *usageStubModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chat.Response, error) bool) { yield(resp, err) }
}

func responseWithTextAndUsage(text string, usage chat.Usage) (*chat.Response, error) {
	return chat.NewResponse(
		&chat.Result{
			AssistantMessage: chat.NewAssistantMessage(text),
			Metadata:         &chat.ResultMetadata{FinishReason: chat.FinishReasonStop},
		},
		&chat.ResponseMetadata{Usage: &usage},
	)
}

func responseWithToolCallAndUsage(name, args string, usage chat.Usage) (*chat.Response, error) {
	calls := []*chat.ToolCallPart{{ID: "call_1", Name: name, Arguments: args}}
	return chat.NewResponse(
		&chat.Result{
			AssistantMessage: chat.NewAssistantMessage(calls),
			Metadata:         &chat.ResultMetadata{FinishReason: chat.FinishReasonToolCalls},
		},
		&chat.ResponseMetadata{Usage: &usage},
	)
}
