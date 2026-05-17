package chat_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
)

// TestResponseAccumulator_TextDeltas exercises the headline streaming
// case: text and reasoning fields concatenate while finish reason and
// metadata overwrite from the final chunk.
func TestResponseAccumulator_TextDeltas(t *testing.T) {
	acc := chat.NewResponseAccumulator()

	acc.AddChunk(chunkWithText("Hello", "", chat.FinishReasonNull))
	acc.AddChunk(chunkWithText(", ", "", chat.FinishReasonNull))
	acc.AddChunk(chunkWithText("world", "", chat.FinishReasonStop))

	got := acc.Result
	if got == nil {
		t.Fatal("accumulator produced no Result")
	}
	if got.AssistantMessage.Text != "Hello, world" {
		t.Fatalf("Text = %q, want %q", got.AssistantMessage.Text, "Hello, world")
	}
	if got.Metadata.FinishReason != chat.FinishReasonStop {
		t.Fatalf("FinishReason = %q, want stop", got.Metadata.FinishReason)
	}
}

func TestResponseAccumulator_ReasoningDeltas(t *testing.T) {
	acc := chat.NewResponseAccumulator()

	acc.AddChunk(chunkWithText("", "Step 1: ", chat.FinishReasonNull))
	acc.AddChunk(chunkWithText("", "Step 2", chat.FinishReasonNull))

	got := acc.Result.AssistantMessage.Reasoning
	if got != "Step 1: Step 2" {
		t.Fatalf("Reasoning = %q, want %q", got, "Step 1: Step 2")
	}
}

// TestResponseAccumulator_ToolCallChunks verifies that tool-call deltas
// concatenate per index — providers may split JSON-shaped Arguments
// across chunks.
func TestResponseAccumulator_ToolCallChunks(t *testing.T) {
	acc := chat.NewResponseAccumulator()

	acc.AddChunk(toolCallChunk("call_1", "search", `{"q":"`))
	acc.AddChunk(toolCallChunk("", "", `lynx"}`))

	got := acc.Result.AssistantMessage.ToolCalls
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

// --- helpers --------------------------------------------------------------

func chunkWithText(text, reasoning string, fr chat.FinishReason) *chat.Response {
	return &chat.Response{
		Result: &chat.Result{
			AssistantMessage: chat.NewAssistantMessage(chat.MessageParams{
				Text:      text,
				Reasoning: reasoning,
			}),
			Metadata: &chat.ResultMetadata{FinishReason: fr},
		},
	}
}

func toolCallChunk(id, name, args string) *chat.Response {
	return &chat.Response{
		Result: &chat.Result{
			AssistantMessage: chat.NewAssistantMessage(chat.MessageParams{
				ToolCalls: []*chat.ToolCall{{ID: id, Name: name, Arguments: args}},
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
