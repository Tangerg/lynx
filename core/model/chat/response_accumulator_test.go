package chat_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
)

// TestResponseAccumulator_TextDeltas exercises the headline streaming
// case: text and reasoning fields concatenate while finish reason and
// metadata overwrite from the final chunk.
func TestResponseAccumulator_TextDeltas(t *testing.T) {
	acc := chat.NewResponseAccumulator()

	acc.AddChunk(textChunk("Hello", chat.FinishReasonNull))
	acc.AddChunk(textChunk(", ", chat.FinishReasonNull))
	acc.AddChunk(textChunk("world", chat.FinishReasonStop))

	got := acc.Result
	if got == nil {
		t.Fatal("accumulator produced no Result")
	}
	if got.AssistantMessage.JoinedText() != "Hello, world" {
		t.Fatalf("Text = %q, want %q", got.AssistantMessage.JoinedText(), "Hello, world")
	}
	if got.Metadata.FinishReason != chat.FinishReasonStop {
		t.Fatalf("FinishReason = %q, want stop", got.Metadata.FinishReason)
	}
}

func TestResponseAccumulator_ReasoningDeltas(t *testing.T) {
	acc := chat.NewResponseAccumulator()

	acc.AddChunk(reasoningChunk("Step 1: "))
	acc.AddChunk(reasoningChunk("Step 2"))

	got := acc.Result.AssistantMessage.JoinedReasoning()
	if got != "Step 1: Step 2" {
		t.Fatalf("Reasoning = %q, want %q", got, "Step 1: Step 2")
	}
}

// TestResponseAccumulator_ToolCallChunks verifies that tool-call deltas
// concatenate per ID — providers may split JSON-shaped Arguments
// across chunks. The same ID flows through to the merged ToolCallPart;
// later deltas with an empty ID also continue the in-flight call.
func TestResponseAccumulator_ToolCallChunks(t *testing.T) {
	acc := chat.NewResponseAccumulator()

	acc.AddChunk(toolCallChunk("call_1", "search", `{"q":"`))
	acc.AddChunk(toolCallChunk("call_1", "", `lynx"}`))

	got := slices.Collect(acc.Result.AssistantMessage.ToolCalls())
	if len(got) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(got))
	}
	if got[0].ID != "call_1" {
		t.Fatalf("ToolCall.ID = %q, want call_1", got[0].ID)
	}
	if got[0].Arguments != `{"q":"lynx"}` {
		t.Fatalf("Arguments = %q, want %q", got[0].Arguments, `{"q":"lynx"}`)
	}
}

func TestResponseAccumulator_InterleavedPartBoundary(t *testing.T) {
	// Text → ToolCall → Text should produce 3 ordered parts, not get
	// re-merged by the accumulator.
	acc := chat.NewResponseAccumulator()
	acc.AddChunk(textChunk("查天气：", chat.FinishReasonNull))
	acc.AddChunk(toolCallChunk("tu_1", "weather", `{"city":"BJ"}`))
	acc.AddChunk(textChunk("查日历：", chat.FinishReasonNull))

	parts := acc.Result.AssistantMessage.Parts
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(parts))
	}
	if parts[0].Kind() != chat.PartKindText ||
		parts[1].Kind() != chat.PartKindToolCall ||
		parts[2].Kind() != chat.PartKindText {
		t.Errorf("part kinds wrong: %s/%s/%s", parts[0].Kind(), parts[1].Kind(), parts[2].Kind())
	}
}

func TestResponseAccumulator_MetadataOverwrites(t *testing.T) {
	acc := chat.NewResponseAccumulator()

	tokens1 := int64(5)
	tokens2 := int64(11)

	acc.AddChunk(chunkWithUsage(&chat.Usage{PromptTokens: 4, CompletionTokens: tokens1}))
	acc.AddChunk(chunkWithUsage(&chat.Usage{PromptTokens: 4, CompletionTokens: tokens2}))

	if acc.Metadata == nil || acc.Metadata.Usage == nil {
		t.Fatal("metadata.Usage missing after AddChunk")
	}
	if acc.Metadata.Usage.CompletionTokens != tokens2 {
		t.Fatalf("Usage.CompletionTokens = %d, want %d (latest wins)",
			acc.Metadata.Usage.CompletionTokens, tokens2)
	}
}

func TestResponseAccumulator_NilChunkSafe(t *testing.T) {
	acc := chat.NewResponseAccumulator()

	// Empty Response{} should not panic.
	acc.AddChunk(&chat.Response{})

	if got := acc.Result; got != nil {
		t.Fatalf("empty AddChunk should not produce a Result, got %+v", got)
	}
}

// TestResponseAccumulator_SharedChunksNoAliasing pins the bug where two
// accumulators fed the SAME chunk objects (as happens when the tool-loop
// and memory stream middlewares both accumulate one provider stream)
// double-counted every delta: the inner accumulator's in-place merge
// mutated the delta part the outer accumulator still held, so a "pong"
// reply persisted as "pongong". add() must adopt a clone, never the
// caller's part, so both accumulators stay independent.
func TestResponseAccumulator_SharedChunksNoAliasing(t *testing.T) {
	chunks := []*chat.Response{
		textChunk("p", chat.FinishReasonNull),
		textChunk("ong", chat.FinishReasonStop),
	}

	inner := chat.NewResponseAccumulator()
	outer := chat.NewResponseAccumulator()
	for _, c := range chunks {
		inner.AddChunk(c) // mimics the inner (tool-loop) middleware
		outer.AddChunk(c) // mimics the outer (memory) middleware on the same objects
	}

	if got := inner.Result.AssistantMessage.JoinedText(); got != "pong" {
		t.Fatalf("inner text = %q, want %q", got, "pong")
	}
	if got := outer.Result.AssistantMessage.JoinedText(); got != "pong" {
		t.Fatalf("outer text = %q, want %q (delta double-counted via shared part)", got, "pong")
	}
}

// --- helpers --------------------------------------------------------------

func textChunk(text string, fr chat.FinishReason) *chat.Response {
	parts := []chat.OutputPart{}
	if text != "" {
		parts = append(parts, &chat.TextPart{Text: text})
	}
	return &chat.Response{
		Result: &chat.Result{
			AssistantMessage: chat.NewAssistantMessage(chat.MessageParams{Parts: parts}),
			Metadata:         &chat.ResultMetadata{FinishReason: fr},
		},
	}
}

func reasoningChunk(text string) *chat.Response {
	return &chat.Response{
		Result: &chat.Result{
			AssistantMessage: chat.NewAssistantMessage(chat.MessageParams{
				Parts: []chat.OutputPart{&chat.ReasoningPart{Text: text}},
			}),
			Metadata: &chat.ResultMetadata{},
		},
	}
}

func toolCallChunk(id, name, args string) *chat.Response {
	return &chat.Response{
		Result: &chat.Result{
			AssistantMessage: chat.NewAssistantMessage(chat.MessageParams{
				Parts: []chat.OutputPart{&chat.ToolCallPart{
					ID:        id,
					Name:      name,
					Arguments: args,
				}},
			}),
			Metadata: &chat.ResultMetadata{},
		},
	}
}

func chunkWithUsage(u *chat.Usage) *chat.Response {
	return &chat.Response{
		Result: &chat.Result{
			AssistantMessage: chat.NewAssistantMessage(chat.MessageParams{}),
			Metadata:         &chat.ResultMetadata{},
		},
		Metadata: &chat.ResponseMetadata{Usage: u},
	}
}
