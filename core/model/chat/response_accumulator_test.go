package chat

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewResponseAccumulator tests the creation of a new accumulator instance
func TestNewResponseAccumulator(t *testing.T) {
	acc := NewResponseAccumulator()

	assert.NotNil(t, acc, "accumulator should not be nil")
	assert.Nil(t, acc.Results, "results should be nil initially")
	assert.Nil(t, acc.Metadata, "metadata should be nil initially")
}

// TestAddChunk_SingleChunk tests adding a single response chunk
func TestAddChunk_SingleChunk(t *testing.T) {
	acc := NewResponseAccumulator()

	// Create a test response chunk
	chunk := &Response{
		Results: []*Result{
			{
				AssistantMessage: NewAssistantMessage("Hello"),
				Metadata: &ResultMetadata{
					FinishReason: FinishReasonNull,
				},
			},
		},
		Metadata: &ResponseMetadata{
			ID:    "test-id-1",
			Model: "gpt-4",
			Usage: &Usage{
				PromptTokens:     10,
				CompletionTokens: 5,
			},
		},
	}

	acc.AddChunk(chunk)

	assert.Len(t, acc.Results, 1, "should have one result")
	assert.Equal(t, "Hello", acc.Results[0].AssistantMessage.Text)
	assert.Equal(t, "test-id-1", acc.Metadata.ID)
	assert.Equal(t, "gpt-4", acc.Metadata.Model)
	assert.Equal(t, int64(10), acc.Metadata.Usage.PromptTokens)
	assert.Equal(t, int64(5), acc.Metadata.Usage.CompletionTokens)
}

// TestAddChunk_MultipleChunks_TextConcatenation tests text concatenation across chunks
func TestAddChunk_MultipleChunks_TextConcatenation(t *testing.T) {
	acc := NewResponseAccumulator()

	// First chunk
	chunk1 := &Response{
		Results: []*Result{
			{
				AssistantMessage: NewAssistantMessage("Hello"),
				Metadata: &ResultMetadata{
					FinishReason: FinishReasonNull,
				},
			},
		},
		Metadata: &ResponseMetadata{
			ID:    "test-id-1",
			Model: "gpt-4",
		},
	}

	// Second chunk
	chunk2 := &Response{
		Results: []*Result{
			{
				AssistantMessage: NewAssistantMessage(" world"),
				Metadata: &ResultMetadata{
					FinishReason: FinishReasonNull,
				},
			},
		},
		Metadata: &ResponseMetadata{
			ID:    "test-id-1",
			Model: "gpt-4",
		},
	}

	// Third chunk with finish reason
	chunk3 := &Response{
		Results: []*Result{
			{
				AssistantMessage: NewAssistantMessage("!"),
				Metadata: &ResultMetadata{
					FinishReason: FinishReasonStop,
				},
			},
		},
		Metadata: &ResponseMetadata{
			ID:    "test-id-1",
			Model: "gpt-4",
		},
	}

	acc.AddChunk(chunk1)
	acc.AddChunk(chunk2)
	acc.AddChunk(chunk3)

	assert.Equal(t, "Hello world!", acc.Results[0].AssistantMessage.Text)
	assert.Equal(t, FinishReasonStop, acc.Results[0].Metadata.FinishReason)
}

// TestAddChunk_ToolCalls_Concatenation tests tool call accumulation
func TestAddChunk_ToolCalls_Concatenation(t *testing.T) {
	acc := NewResponseAccumulator()

	// First chunk - partial tool call
	chunk1 := &Response{
		Results: []*Result{
			{
				AssistantMessage: &AssistantMessage{
					ToolCalls: []*ToolCall{
						{
							ID:        "call_",
							Name:      "get_",
							Arguments: "{\"loc",
						},
					},
				},
				Metadata: &ResultMetadata{
					FinishReason: FinishReasonNull,
				},
			},
		},
		Metadata: &ResponseMetadata{ID: "test-id"},
	}

	// Second chunk - continue tool call
	chunk2 := &Response{
		Results: []*Result{
			{
				AssistantMessage: &AssistantMessage{
					ToolCalls: []*ToolCall{
						{
							ID:        "abc123",
							Name:      "weather",
							Arguments: "ation\"",
						},
					},
				},
				Metadata: &ResultMetadata{
					FinishReason: FinishReasonNull,
				},
			},
		},
		Metadata: &ResponseMetadata{ID: "test-id"},
	}

	// Third chunk - complete tool call
	chunk3 := &Response{
		Results: []*Result{
			{
				AssistantMessage: &AssistantMessage{
					ToolCalls: []*ToolCall{
						{
							ID:        "",
							Name:      "",
							Arguments: ":\"Beijing\"}",
						},
					},
				},
				Metadata: &ResultMetadata{
					FinishReason: FinishReasonToolCalls,
				},
			},
		},
		Metadata: &ResponseMetadata{ID: "test-id"},
	}

	acc.AddChunk(chunk1)
	acc.AddChunk(chunk2)
	acc.AddChunk(chunk3)

	require.Len(t, acc.Results, 1)
	require.Len(t, acc.Results[0].AssistantMessage.ToolCalls, 1)

	toolCall := acc.Results[0].AssistantMessage.ToolCalls[0]
	assert.Equal(t, "call_abc123", toolCall.ID)
	assert.Equal(t, "get_weather", toolCall.Name)
	assert.Equal(t, "{\"location\":\"Beijing\"}", toolCall.Arguments)
	assert.Equal(t, FinishReasonToolCalls, acc.Results[0].Metadata.FinishReason)
}

// TestAddChunk_MultipleToolCalls tests accumulation of multiple tool calls
func TestAddChunk_MultipleToolCalls(t *testing.T) {
	acc := NewResponseAccumulator()

	chunk1 := &Response{
		Results: []*Result{
			{
				AssistantMessage: &AssistantMessage{
					ToolCalls: []*ToolCall{
						{ID: "call_1", Name: "tool_", Arguments: "{\"a\""},
						{ID: "call_2", Name: "tool_", Arguments: "{\"b\""},
					},
				},
				Metadata: &ResultMetadata{FinishReason: FinishReasonNull},
			},
		},
		Metadata: &ResponseMetadata{ID: "test"},
	}

	chunk2 := &Response{
		Results: []*Result{
			{
				AssistantMessage: &AssistantMessage{
					ToolCalls: []*ToolCall{
						{ID: "", Name: "one", Arguments: ":1}"},
						{ID: "", Name: "two", Arguments: ":2}"},
					},
				},
				Metadata: &ResultMetadata{FinishReason: FinishReasonToolCalls},
			},
		},
		Metadata: &ResponseMetadata{ID: "test"},
	}

	acc.AddChunk(chunk1)
	acc.AddChunk(chunk2)

	require.Len(t, acc.Results[0].AssistantMessage.ToolCalls, 2)

	assert.Equal(t, "call_1", acc.Results[0].AssistantMessage.ToolCalls[0].ID)
	assert.Equal(t, "tool_one", acc.Results[0].AssistantMessage.ToolCalls[0].Name)
	assert.Equal(t, "{\"a\":1}", acc.Results[0].AssistantMessage.ToolCalls[0].Arguments)

	assert.Equal(t, "call_2", acc.Results[0].AssistantMessage.ToolCalls[1].ID)
	assert.Equal(t, "tool_two", acc.Results[0].AssistantMessage.ToolCalls[1].Name)
	assert.Equal(t, "{\"b\":2}", acc.Results[0].AssistantMessage.ToolCalls[1].Arguments)
}

// TestAddChunk_ToolMessage_Overwrite tests that tool messages are overwritten, not accumulated
func TestAddChunk_ToolMessage_Overwrite(t *testing.T) {
	acc := NewResponseAccumulator()

	// First chunk with tool message
	chunk1 := &Response{
		Results: []*Result{
			{
				AssistantMessage: NewAssistantMessage(""),
				Metadata:         &ResultMetadata{FinishReason: FinishReasonNull},
				ToolMessage: &ToolMessage{
					ToolReturns: []*ToolReturn{
						{ID: "call_1", Name: "tool_one", Result: "incomplete"},
					},
				},
			},
		},
		Metadata: &ResponseMetadata{ID: "test"},
	}

	// Second chunk with complete tool message
	chunk2 := &Response{
		Results: []*Result{
			{
				AssistantMessage: NewAssistantMessage(""),
				Metadata:         &ResultMetadata{FinishReason: FinishReasonStop},
				ToolMessage: &ToolMessage{
					ToolReturns: []*ToolReturn{
						{ID: "call_1", Name: "tool_one", Result: "complete result"},
					},
				},
			},
		},
		Metadata: &ResponseMetadata{ID: "test"},
	}

	acc.AddChunk(chunk1)
	acc.AddChunk(chunk2)

	require.NotNil(t, acc.Results[0].ToolMessage)
	require.Len(t, acc.Results[0].ToolMessage.ToolReturns, 1)

	// Tool message should be overwritten, not concatenated
	assert.Equal(t, "complete result", acc.Results[0].ToolMessage.ToolReturns[0].Result)
}

// TestAddChunk_Metadata_Overwrite tests metadata overwriting behavior
func TestAddChunk_Metadata_Overwrite(t *testing.T) {
	acc := NewResponseAccumulator()

	chunk1 := &Response{
		Results: []*Result{
			{
				AssistantMessage: NewAssistantMessage("Hello"),
				Metadata: &ResultMetadata{
					FinishReason: FinishReasonNull,
					Extra: map[string]any{
						"key1": "value1",
						"key2": "value2",
					},
				},
			},
		},
		Metadata: &ResponseMetadata{
			ID:    "id-1",
			Model: "model-1",
			Usage: &Usage{PromptTokens: 10},
			Extra: map[string]any{
				"meta1": "val1",
			},
		},
	}

	chunk2 := &Response{
		Results: []*Result{
			{
				AssistantMessage: NewAssistantMessage(" world"),
				Metadata: &ResultMetadata{
					FinishReason: FinishReasonStop,
					Extra: map[string]any{
						"key2": "updated_value2",
						"key3": "value3",
					},
				},
			},
		},
		Metadata: &ResponseMetadata{
			ID:    "id-2",
			Model: "model-2",
			Usage: &Usage{PromptTokens: 20, CompletionTokens: 5},
			Extra: map[string]any{
				"meta1": "updated_val1",
				"meta2": "val2",
			},
		},
	}

	acc.AddChunk(chunk1)
	acc.AddChunk(chunk2)

	// Response metadata should be overwritten
	assert.Equal(t, "id-2", acc.Metadata.ID)
	assert.Equal(t, "model-2", acc.Metadata.Model)
	assert.Equal(t, int64(20), acc.Metadata.Usage.PromptTokens)
	assert.Equal(t, int64(5), acc.Metadata.Usage.CompletionTokens)

	// Extra metadata should be merged
	assert.Equal(t, "updated_val1", acc.Metadata.Extra["meta1"])
	assert.Equal(t, "val2", acc.Metadata.Extra["meta2"])

	// Result metadata should be overwritten
	assert.Equal(t, FinishReasonStop, acc.Results[0].Metadata.FinishReason)

	// Result extra should be merged
	assert.Equal(t, "value1", acc.Results[0].Metadata.Extra["key1"])
	assert.Equal(t, "updated_value2", acc.Results[0].Metadata.Extra["key2"])
	assert.Equal(t, "value3", acc.Results[0].Metadata.Extra["key3"])
}

// TestAddChunk_MultipleResults tests accumulation with multiple results
func TestAddChunk_MultipleResults(t *testing.T) {
	acc := NewResponseAccumulator()

	chunk1 := &Response{
		Results: []*Result{
			{
				AssistantMessage: NewAssistantMessage("First result: "),
				Metadata:         &ResultMetadata{FinishReason: FinishReasonNull},
			},
			{
				AssistantMessage: NewAssistantMessage("Second result: "),
				Metadata:         &ResultMetadata{FinishReason: FinishReasonNull},
			},
		},
		Metadata: &ResponseMetadata{ID: "test"},
	}

	chunk2 := &Response{
		Results: []*Result{
			{
				AssistantMessage: NewAssistantMessage("part A"),
				Metadata:         &ResultMetadata{FinishReason: FinishReasonStop},
			},
			{
				AssistantMessage: NewAssistantMessage("part B"),
				Metadata:         &ResultMetadata{FinishReason: FinishReasonStop},
			},
		},
		Metadata: &ResponseMetadata{ID: "test"},
	}

	acc.AddChunk(chunk1)
	acc.AddChunk(chunk2)

	require.Len(t, acc.Results, 2)
	assert.Equal(t, "First result: part A", acc.Results[0].AssistantMessage.Text)
	assert.Equal(t, "Second result: part B", acc.Results[1].AssistantMessage.Text)
	assert.Equal(t, FinishReasonStop, acc.Results[0].Metadata.FinishReason)
	assert.Equal(t, FinishReasonStop, acc.Results[1].Metadata.FinishReason)
}

// TestAddChunk_NilValues tests handling of nil values in chunks
func TestAddChunk_NilValues(t *testing.T) {
	acc := NewResponseAccumulator()

	// Chunk with nil metadata
	chunk1 := &Response{
		Results: []*Result{
			{
				AssistantMessage: NewAssistantMessage("Hello"),
				Metadata:         &ResultMetadata{FinishReason: FinishReasonNull},
			},
		},
		Metadata: nil,
	}

	acc.AddChunk(chunk1)
	assert.Nil(t, acc.Metadata, "metadata should remain nil")
	assert.Equal(t, "Hello", acc.Results[0].AssistantMessage.Text)

	// Chunk with nil tool message
	chunk2 := &Response{
		Results: []*Result{
			{
				AssistantMessage: NewAssistantMessage(" world"),
				Metadata:         &ResultMetadata{FinishReason: FinishReasonStop},
				ToolMessage:      nil,
			},
		},
		Metadata: &ResponseMetadata{ID: "test"},
	}

	acc.AddChunk(chunk2)
	assert.NotNil(t, acc.Metadata)
	assert.Nil(t, acc.Results[0].ToolMessage)
	assert.Equal(t, "Hello world", acc.Results[0].AssistantMessage.Text)
}

// TestAddChunk_EmptyChunks tests handling of empty response chunks
func TestAddChunk_EmptyChunks(t *testing.T) {
	acc := NewResponseAccumulator()

	// Empty results
	chunk := &Response{
		Results:  []*Result{},
		Metadata: &ResponseMetadata{ID: "test"},
	}

	acc.AddChunk(chunk)

	assert.Len(t, acc.Results, 0, "results should remain empty")
	assert.NotNil(t, acc.Metadata)
	assert.Equal(t, "test", acc.Metadata.ID)
}

// TestAddChunk_AssistantMessageMetadata tests assistant message metadata merging
func TestAddChunk_AssistantMessageMetadata(t *testing.T) {
	acc := NewResponseAccumulator()

	chunk1 := &Response{
		Results: []*Result{
			{
				AssistantMessage: &AssistantMessage{
					Text: "Hello",
					Metadata: map[string]any{
						"key1": "value1",
						"key2": "value2",
					},
				},
				Metadata: &ResultMetadata{FinishReason: FinishReasonNull},
			},
		},
		Metadata: &ResponseMetadata{ID: "test"},
	}

	chunk2 := &Response{
		Results: []*Result{
			{
				AssistantMessage: &AssistantMessage{
					Text: " world",
					Metadata: map[string]any{
						"key2": "updated_value2",
						"key3": "value3",
					},
				},
				Metadata: &ResultMetadata{FinishReason: FinishReasonStop},
			},
		},
		Metadata: &ResponseMetadata{ID: "test"},
	}

	acc.AddChunk(chunk1)
	acc.AddChunk(chunk2)

	assert.Equal(t, "Hello world", acc.Results[0].AssistantMessage.Text)
	assert.Equal(t, "value1", acc.Results[0].AssistantMessage.Metadata["key1"])
	assert.Equal(t, "updated_value2", acc.Results[0].AssistantMessage.Metadata["key2"])
	assert.Equal(t, "value3", acc.Results[0].AssistantMessage.Metadata["key3"])
}

// TestAddChunk_UsageAccumulation tests token usage metadata accumulation
func TestAddChunk_UsageAccumulation(t *testing.T) {
	acc := NewResponseAccumulator()

	chunk1 := &Response{
		Results: []*Result{
			{
				AssistantMessage: NewAssistantMessage("Hello"),
				Metadata:         &ResultMetadata{FinishReason: FinishReasonNull},
			},
		},
		Metadata: &ResponseMetadata{
			ID:    "test",
			Model: "gpt-4",
			Usage: &Usage{
				PromptTokens:     10,
				CompletionTokens: 5,
			},
		},
	}

	chunk2 := &Response{
		Results: []*Result{
			{
				AssistantMessage: NewAssistantMessage(" world"),
				Metadata:         &ResultMetadata{FinishReason: FinishReasonStop},
			},
		},
		Metadata: &ResponseMetadata{
			ID:    "test",
			Model: "gpt-4",
			Usage: &Usage{
				PromptTokens:     10,
				CompletionTokens: 10,
			},
		},
	}

	acc.AddChunk(chunk1)
	acc.AddChunk(chunk2)

	// Usage should be overwritten with latest values
	assert.Equal(t, int64(10), acc.Metadata.Usage.PromptTokens)
	assert.Equal(t, int64(10), acc.Metadata.Usage.CompletionTokens)
	assert.Equal(t, int64(20), acc.Metadata.Usage.TotalTokens())
}

// TestAddChunk_RateLimitAccumulation tests rate limit metadata accumulation
func TestAddChunk_RateLimitAccumulation(t *testing.T) {
	acc := NewResponseAccumulator()

	chunk := &Response{
		Results: []*Result{
			{
				AssistantMessage: NewAssistantMessage("Hello"),
				Metadata:         &ResultMetadata{FinishReason: FinishReasonStop},
			},
		},
		Metadata: &ResponseMetadata{
			ID:    "test",
			Model: "gpt-4",
			RateLimit: &RateLimit{
				RequestsLimit:     1000,
				RequestsRemaining: 999,
				TokensLimit:       100000,
				TokensRemaining:   99990,
			},
		},
	}

	acc.AddChunk(chunk)

	assert.NotNil(t, acc.Metadata.RateLimit)
	assert.Equal(t, int64(1000), acc.Metadata.RateLimit.RequestsLimit)
	assert.Equal(t, int64(999), acc.Metadata.RateLimit.RequestsRemaining)
	assert.Equal(t, int64(100000), acc.Metadata.RateLimit.TokensLimit)
	assert.Equal(t, int64(99990), acc.Metadata.RateLimit.TokensRemaining)
}
