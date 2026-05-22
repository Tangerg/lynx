package engine

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/core/model/chat"
)

// stubModel is a deterministic chat.Model used by engine + chat-service
// tests to drive the agent loop without a real LLM.
//
// State machine:
//
//   - Round 1 (no tool message in history): emit a single tool call
//     against ToolName with ToolArgs.
//   - Round 2 (tool message present): emit the configured FinalText.
//
// Two-round behaviour is enough to exercise the ToolMiddleware tool
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
func (m *streamingStubModel) Metadata() chat.ModelMetadata { return chat.ModelMetadata{Provider: "stub"} }

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
	if hasToolMessage(req.Messages) {
		return responseWithTextAndUsage("done", m.round2Usage)
	}
	return responseWithToolCallAndUsage("bash", `{"command":"echo lyra"}`, m.round1Usage)
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
