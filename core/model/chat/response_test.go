package chat

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFinishReason_String(t *testing.T) {
	tests := []struct {
		name     string
		reason   FinishReason
		expected string
	}{
		{"stop", FinishReasonStop, "stop"},
		{"length", FinishReasonLength, "length"},
		{"tool_calls", FinishReasonToolCalls, "tool_calls"},
		{"content_filter", FinishReasonContentFilter, "content_filter"},
		{"return_direct", FinishReasonReturnDirect, "return_direct"},
		{"other", FinishReasonOther, "other"},
		{"null", FinishReasonNull, "null"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.reason.String())
		})
	}
}

func TestResultMetadata_Get(t *testing.T) {
	t.Run("get existing key", func(t *testing.T) {
		metadata := &ResultMetadata{}
		metadata.Set("key", "value")

		val, exists := metadata.Get("key")
		assert.True(t, exists)
		assert.Equal(t, "value", val)
	})

	t.Run("get non-existing key", func(t *testing.T) {
		metadata := &ResultMetadata{}

		val, exists := metadata.Get("nonexistent")
		assert.False(t, exists)
		assert.Nil(t, val)
	})

	t.Run("get with nil extra", func(t *testing.T) {
		metadata := &ResultMetadata{FinishReason: FinishReasonStop}

		val, exists := metadata.Get("key")
		assert.False(t, exists)
		assert.Nil(t, val)
		assert.NotNil(t, metadata.Extra)
	})
}

func TestResultMetadata_Set(t *testing.T) {
	t.Run("set on nil extra", func(t *testing.T) {
		metadata := &ResultMetadata{FinishReason: FinishReasonStop}
		metadata.Set("key", "value")

		assert.NotNil(t, metadata.Extra)
		assert.Equal(t, "value", metadata.Extra["key"])
	})

	t.Run("set multiple values", func(t *testing.T) {
		metadata := &ResultMetadata{}
		metadata.Set("key1", "value1")
		metadata.Set("key2", 123)

		assert.Equal(t, "value1", metadata.Extra["key1"])
		assert.Equal(t, 123, metadata.Extra["key2"])
	})

	t.Run("overwrite existing value", func(t *testing.T) {
		metadata := &ResultMetadata{}
		metadata.Set("key", "value1")
		metadata.Set("key", "value2")

		assert.Equal(t, "value2", metadata.Extra["key"])
	})
}

func TestNewResult(t *testing.T) {
	t.Run("valid result", func(t *testing.T) {
		msg := NewAssistantMessage("response")
		metadata := &ResultMetadata{FinishReason: FinishReasonStop}

		result, err := NewResult(msg, metadata)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, msg, result.AssistantMessage)
		assert.Equal(t, metadata, result.Metadata)
		assert.Nil(t, result.ToolMessage)
	})

	t.Run("nil assistant message", func(t *testing.T) {
		metadata := &ResultMetadata{FinishReason: FinishReasonStop}

		result, err := NewResult(nil, metadata)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "assistant message cannot be nil")
	})

	t.Run("nil metadata", func(t *testing.T) {
		msg := NewAssistantMessage("response")

		result, err := NewResult(msg, nil)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "result metadata cannot be nil")
	})

	t.Run("with tool message", func(t *testing.T) {
		msg := NewAssistantMessage("response")
		metadata := &ResultMetadata{FinishReason: FinishReasonToolCalls}
		toolMsg, _ := NewToolMessage([]*ToolReturn{{ID: "1", Name: "tool", Result: "result"}})

		result, err := NewResult(msg, metadata)
		require.NoError(t, err)
		result.ToolMessage = toolMsg

		assert.NotNil(t, result.ToolMessage)
		assert.Len(t, result.ToolMessage.ToolReturns, 1)
	})
}

func TestUsage_TotalTokens(t *testing.T) {
	tests := []struct {
		name             string
		promptTokens     int64
		completionTokens int64
		expected         int64
	}{
		{"both zero", 0, 0, 0},
		{"only prompt", 100, 0, 100},
		{"only completion", 0, 50, 50},
		{"both present", 100, 50, 150},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := &Usage{
				PromptTokens:     tt.promptTokens,
				CompletionTokens: tt.completionTokens,
			}
			assert.Equal(t, tt.expected, usage.TotalTokens())
		})
	}
}

func TestUsage_OriginalUsage(t *testing.T) {
	t.Run("with original usage", func(t *testing.T) {
		originalData := map[string]any{"custom": "data"}
		usage := &Usage{
			PromptTokens:     100,
			CompletionTokens: 50,
			OriginalUsage:    originalData,
		}

		assert.Equal(t, originalData, usage.OriginalUsage)
	})

	t.Run("without original usage", func(t *testing.T) {
		usage := &Usage{
			PromptTokens:     100,
			CompletionTokens: 50,
		}

		assert.Nil(t, usage.OriginalUsage)
	})
}

func TestRateLimit(t *testing.T) {
	t.Run("full rate limit info", func(t *testing.T) {
		rateLimit := &RateLimit{
			RequestsLimit:     1000,
			RequestsRemaining: 500,
			RequestsReset:     time.Hour,
			TokensLimit:       100000,
			TokensRemaining:   50000,
			TokensReset:       time.Minute * 30,
		}

		assert.Equal(t, int64(1000), rateLimit.RequestsLimit)
		assert.Equal(t, int64(500), rateLimit.RequestsRemaining)
		assert.Equal(t, time.Hour, rateLimit.RequestsReset)
		assert.Equal(t, int64(100000), rateLimit.TokensLimit)
		assert.Equal(t, int64(50000), rateLimit.TokensRemaining)
		assert.Equal(t, time.Minute*30, rateLimit.TokensReset)
	})

	t.Run("zero values", func(t *testing.T) {
		rateLimit := &RateLimit{}

		assert.Equal(t, int64(0), rateLimit.RequestsLimit)
		assert.Equal(t, int64(0), rateLimit.TokensLimit)
		assert.Equal(t, time.Duration(0), rateLimit.RequestsReset)
	})
}

func TestResponseMetadata_Get(t *testing.T) {
	t.Run("get existing key", func(t *testing.T) {
		metadata := &ResponseMetadata{}
		metadata.Set("key", "value")

		val, exists := metadata.Get("key")
		assert.True(t, exists)
		assert.Equal(t, "value", val)
	})

	t.Run("get non-existing key", func(t *testing.T) {
		metadata := &ResponseMetadata{}

		val, exists := metadata.Get("nonexistent")
		assert.False(t, exists)
		assert.Nil(t, val)
	})

	t.Run("get with nil extra", func(t *testing.T) {
		metadata := &ResponseMetadata{ID: "resp-123"}

		val, exists := metadata.Get("key")
		assert.False(t, exists)
		assert.Nil(t, val)
		assert.NotNil(t, metadata.Extra)
	})
}

func TestResponseMetadata_Set(t *testing.T) {
	t.Run("set on nil extra", func(t *testing.T) {
		metadata := &ResponseMetadata{ID: "resp-123"}
		metadata.Set("key", "value")

		assert.NotNil(t, metadata.Extra)
		assert.Equal(t, "value", metadata.Extra["key"])
	})

	t.Run("set multiple values", func(t *testing.T) {
		metadata := &ResponseMetadata{}
		metadata.Set("key1", "value1")
		metadata.Set("key2", 123)

		assert.Equal(t, "value1", metadata.Extra["key1"])
		assert.Equal(t, 123, metadata.Extra["key2"])
	})

	t.Run("overwrite existing value", func(t *testing.T) {
		metadata := &ResponseMetadata{}
		metadata.Set("key", "value1")
		metadata.Set("key", "value2")

		assert.Equal(t, "value2", metadata.Extra["key"])
	})
}

func TestResponseMetadata_CompleteFields(t *testing.T) {
	t.Run("all fields populated", func(t *testing.T) {
		usage := &Usage{PromptTokens: 100, CompletionTokens: 50}
		rateLimit := &RateLimit{RequestsLimit: 1000, TokensLimit: 100000}

		metadata := &ResponseMetadata{
			ID:        "resp-123",
			Model:     "gpt-4",
			Usage:     usage,
			RateLimit: rateLimit,
			Created:   1234567890,
			Extra:     map[string]any{"custom": "data"},
		}

		assert.Equal(t, "resp-123", metadata.ID)
		assert.Equal(t, "gpt-4", metadata.Model)
		assert.Equal(t, usage, metadata.Usage)
		assert.Equal(t, rateLimit, metadata.RateLimit)
		assert.Equal(t, int64(1234567890), metadata.Created)
		assert.Equal(t, "data", metadata.Extra["custom"])
	})
}

func TestNewResponse(t *testing.T) {
	t.Run("valid response", func(t *testing.T) {
		msg := NewAssistantMessage("response")
		resultMetadata := &ResultMetadata{FinishReason: FinishReasonStop}
		result, _ := NewResult(msg, resultMetadata)

		respMetadata := &ResponseMetadata{
			ID:    "resp-123",
			Model: "gpt-4",
		}

		response, err := NewResponse([]*Result{result}, respMetadata)
		require.NoError(t, err)
		assert.NotNil(t, response)
		assert.Len(t, response.Results, 1)
		assert.Equal(t, respMetadata, response.Metadata)
	})

	t.Run("empty results", func(t *testing.T) {
		metadata := &ResponseMetadata{ID: "resp-123"}

		response, err := NewResponse([]*Result{}, metadata)
		assert.Error(t, err)
		assert.Nil(t, response)
		assert.Contains(t, err.Error(), "must contain at least one result")
	})

	t.Run("nil metadata", func(t *testing.T) {
		msg := NewAssistantMessage("response")
		resultMetadata := &ResultMetadata{FinishReason: FinishReasonStop}
		result, _ := NewResult(msg, resultMetadata)

		response, err := NewResponse([]*Result{result}, nil)
		assert.Error(t, err)
		assert.Nil(t, response)
		assert.Contains(t, err.Error(), "response metadata cannot be nil")
	})

	t.Run("multiple results", func(t *testing.T) {
		msg1 := NewAssistantMessage("response1")
		metadata1 := &ResultMetadata{FinishReason: FinishReasonStop}
		result1, _ := NewResult(msg1, metadata1)

		msg2 := NewAssistantMessage("response2")
		metadata2 := &ResultMetadata{FinishReason: FinishReasonLength}
		result2, _ := NewResult(msg2, metadata2)

		respMetadata := &ResponseMetadata{ID: "resp-123"}

		response, err := NewResponse([]*Result{result1, result2}, respMetadata)
		require.NoError(t, err)
		assert.Len(t, response.Results, 2)
	})
}

func TestResponse_Result(t *testing.T) {
	t.Run("get first result", func(t *testing.T) {
		msg := NewAssistantMessage("response")
		resultMetadata := &ResultMetadata{FinishReason: FinishReasonStop}
		result, _ := NewResult(msg, resultMetadata)

		respMetadata := &ResponseMetadata{ID: "resp-123"}
		response, _ := NewResponse([]*Result{result}, respMetadata)

		firstResult := response.Result()
		assert.NotNil(t, firstResult)
		assert.Equal(t, result, firstResult)
	})

	t.Run("multiple results returns first", func(t *testing.T) {
		msg1 := NewAssistantMessage("response1")
		metadata1 := &ResultMetadata{FinishReason: FinishReasonStop}
		result1, _ := NewResult(msg1, metadata1)

		msg2 := NewAssistantMessage("response2")
		metadata2 := &ResultMetadata{FinishReason: FinishReasonLength}
		result2, _ := NewResult(msg2, metadata2)

		respMetadata := &ResponseMetadata{ID: "resp-123"}
		response, _ := NewResponse([]*Result{result1, result2}, respMetadata)

		firstResult := response.Result()
		assert.Equal(t, result1, firstResult)
		assert.NotEqual(t, result2, firstResult)
	})

	t.Run("empty results", func(t *testing.T) {
		response := &Response{
			Results:  []*Result{},
			Metadata: &ResponseMetadata{ID: "resp-123"},
		}

		result := response.Result()
		assert.Nil(t, result)
	})
}

func TestResponse_findFirstResultWithToolCalls(t *testing.T) {
	t.Run("find result with tool calls", func(t *testing.T) {
		toolCall := &ToolCall{ID: "1", Name: "test", Arguments: "{}"}
		msgWithTools := NewAssistantMessage([]*ToolCall{toolCall})
		metadataWithTools := &ResultMetadata{FinishReason: FinishReasonToolCalls}
		resultWithTools, _ := NewResult(msgWithTools, metadataWithTools)

		respMetadata := &ResponseMetadata{ID: "resp-123"}
		response, _ := NewResponse([]*Result{resultWithTools}, respMetadata)

		found := response.findFirstResultWithToolCalls()
		assert.NotNil(t, found)
		assert.Equal(t, resultWithTools, found)
	})

	t.Run("find first among multiple results", func(t *testing.T) {
		msg1 := NewAssistantMessage("no tools")
		metadata1 := &ResultMetadata{FinishReason: FinishReasonStop}
		result1, _ := NewResult(msg1, metadata1)

		toolCall := &ToolCall{ID: "1", Name: "test", Arguments: "{}"}
		msg2 := NewAssistantMessage([]*ToolCall{toolCall})
		metadata2 := &ResultMetadata{FinishReason: FinishReasonToolCalls}
		result2, _ := NewResult(msg2, metadata2)

		respMetadata := &ResponseMetadata{ID: "resp-123"}
		response, _ := NewResponse([]*Result{result1, result2}, respMetadata)

		found := response.findFirstResultWithToolCalls()
		assert.NotNil(t, found)
		assert.Equal(t, result2, found)
	})

	t.Run("no result with tool calls", func(t *testing.T) {
		msg := NewAssistantMessage("response")
		metadata := &ResultMetadata{FinishReason: FinishReasonStop}
		result, _ := NewResult(msg, metadata)

		respMetadata := &ResponseMetadata{ID: "resp-123"}
		response, _ := NewResponse([]*Result{result}, respMetadata)

		found := response.findFirstResultWithToolCalls()
		assert.Nil(t, found)
	})

	t.Run("empty results", func(t *testing.T) {
		response := &Response{
			Results:  []*Result{},
			Metadata: &ResponseMetadata{ID: "resp-123"},
		}

		found := response.findFirstResultWithToolCalls()
		assert.Nil(t, found)
	})
}

func TestResult_WithToolMessage(t *testing.T) {
	t.Run("result with tool message", func(t *testing.T) {
		toolCall := &ToolCall{ID: "1", Name: "test", Arguments: "{}"}
		msg := NewAssistantMessage([]*ToolCall{toolCall})
		metadata := &ResultMetadata{FinishReason: FinishReasonToolCalls}
		result, _ := NewResult(msg, metadata)

		toolReturn := &ToolReturn{ID: "1", Name: "test", Result: "result"}
		toolMsg, _ := NewToolMessage([]*ToolReturn{toolReturn})
		result.ToolMessage = toolMsg

		assert.NotNil(t, result.ToolMessage)
		assert.Len(t, result.ToolMessage.ToolReturns, 1)
		assert.Equal(t, "result", result.ToolMessage.ToolReturns[0].Result)
	})
}

func TestResponseMetadata_WithUsage(t *testing.T) {
	t.Run("calculate total tokens", func(t *testing.T) {
		usage := &Usage{
			PromptTokens:     100,
			CompletionTokens: 50,
		}

		metadata := &ResponseMetadata{
			ID:    "resp-123",
			Usage: usage,
		}

		assert.Equal(t, int64(150), metadata.Usage.TotalTokens())
	})
}

func TestFinishReasonConstants(t *testing.T) {
	t.Run("all constants defined", func(t *testing.T) {
		reasons := []FinishReason{
			FinishReasonStop,
			FinishReasonLength,
			FinishReasonToolCalls,
			FinishReasonContentFilter,
			FinishReasonReturnDirect,
			FinishReasonOther,
			FinishReasonNull,
		}

		for _, reason := range reasons {
			assert.NotEmpty(t, reason.String())
		}
	})
}
