package chat

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOptions(t *testing.T) {
	t.Run("valid model", func(t *testing.T) {
		opts, err := NewOptions("gpt-4")
		require.NoError(t, err)
		assert.NotNil(t, opts)
		assert.Equal(t, "gpt-4", opts.Model)
	})

	t.Run("empty model", func(t *testing.T) {
		opts, err := NewOptions("")
		assert.Error(t, err)
		assert.Nil(t, opts)
		assert.Contains(t, err.Error(), "model can not be empty")
	})
}

func TestOptions_Get(t *testing.T) {
	t.Run("get existing key", func(t *testing.T) {
		opts, _ := NewOptions("gpt-4")
		opts.Set("key", "value")

		val, exists := opts.Get("key")
		assert.True(t, exists)
		assert.Equal(t, "value", val)
	})

	t.Run("get non-existing key", func(t *testing.T) {
		opts, _ := NewOptions("gpt-4")

		val, exists := opts.Get("nonexistent")
		assert.False(t, exists)
		assert.Nil(t, val)
	})

	t.Run("get with nil extra", func(t *testing.T) {
		opts := &Options{Model: "gpt-4"}

		val, exists := opts.Get("key")
		assert.False(t, exists)
		assert.Nil(t, val)
		assert.NotNil(t, opts.Extra)
	})
}

func TestOptions_Set(t *testing.T) {
	t.Run("set on nil extra", func(t *testing.T) {
		opts := &Options{Model: "gpt-4"}
		opts.Set("key", "value")

		assert.NotNil(t, opts.Extra)
		assert.Equal(t, "value", opts.Extra["key"])
	})

	t.Run("set multiple values", func(t *testing.T) {
		opts, _ := NewOptions("gpt-4")
		opts.Set("key1", "value1")
		opts.Set("key2", 123)

		assert.Equal(t, "value1", opts.Extra["key1"])
		assert.Equal(t, 123, opts.Extra["key2"])
	})

	t.Run("overwrite existing value", func(t *testing.T) {
		opts, _ := NewOptions("gpt-4")
		opts.Set("key", "value1")
		opts.Set("key", "value2")

		assert.Equal(t, "value2", opts.Extra["key"])
	})
}

func TestOptions_Clone(t *testing.T) {
	t.Run("clone nil", func(t *testing.T) {
		var opts *Options
		cloned := opts.Clone()
		assert.Nil(t, cloned)
	})

	t.Run("clone with all fields", func(t *testing.T) {
		freq := 0.5
		maxTokens := int64(100)
		presence := 0.3
		temp := 0.7
		topK := int64(10)
		topP := 0.9

		original := &Options{
			Model:            "gpt-4",
			FrequencyPenalty: &freq,
			MaxTokens:        &maxTokens,
			PresencePenalty:  &presence,
			Stop:             []string{"stop1", "stop2"},
			Temperature:      &temp,
			TopK:             &topK,
			TopP:             &topP,
			Tools:            []Tool{},
			Extra:            map[string]any{"key": "value"},
		}

		cloned := original.Clone()

		assert.NotSame(t, original, cloned)
		assert.Equal(t, original.Model, cloned.Model)
		assert.NotSame(t, original.FrequencyPenalty, cloned.FrequencyPenalty)
		assert.Equal(t, *original.FrequencyPenalty, *cloned.FrequencyPenalty)
		assert.NotSame(t, &original.Stop, &cloned.Stop)
		assert.Equal(t, original.Stop, cloned.Stop)
		assert.NotSame(t, &original.Extra, &cloned.Extra)
		assert.Equal(t, original.Extra, cloned.Extra)
	})

	t.Run("clone independence", func(t *testing.T) {
		original, _ := NewOptions("gpt-4")
		original.Set("key", "value")

		cloned := original.Clone()
		cloned.Model = "gpt-3.5"
		cloned.Set("key", "new_value")

		assert.Equal(t, "gpt-4", original.Model)
		assert.Equal(t, "value", original.Extra["key"])
		assert.Equal(t, "gpt-3.5", cloned.Model)
		assert.Equal(t, "new_value", cloned.Extra["key"])
	})
}

func TestMergeOptions(t *testing.T) {
	t.Run("nil base options", func(t *testing.T) {
		result, err := MergeOptions(nil)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "options cannot be nil")
	})

	t.Run("merge with no additional options", func(t *testing.T) {
		base, _ := NewOptions("gpt-4")
		result, err := MergeOptions(base)

		require.NoError(t, err)
		assert.NotSame(t, base, result)
		assert.Equal(t, base.Model, result.Model)
	})

	t.Run("merge model override", func(t *testing.T) {
		base, _ := NewOptions("gpt-4")
		opt1, _ := NewOptions("gpt-3.5")

		result, err := MergeOptions(base, opt1)
		require.NoError(t, err)
		assert.Equal(t, "gpt-3.5", result.Model)
	})

	t.Run("merge scalar fields", func(t *testing.T) {
		freq1 := 0.5
		freq2 := 0.8
		temp := 0.7

		base := &Options{
			Model:            "gpt-4",
			FrequencyPenalty: &freq1,
		}
		opt1 := &Options{
			FrequencyPenalty: &freq2,
			Temperature:      &temp,
		}

		result, err := MergeOptions(base, opt1)
		require.NoError(t, err)
		assert.Equal(t, freq2, *result.FrequencyPenalty)
		assert.Equal(t, temp, *result.Temperature)
	})

	t.Run("merge stop sequences", func(t *testing.T) {
		base := &Options{
			Model: "gpt-4",
			Stop:  []string{"stop1"},
		}
		opt1 := &Options{
			Stop: []string{"stop2", "stop3"},
		}

		result, err := MergeOptions(base, opt1)
		require.NoError(t, err)
		assert.Equal(t, []string{"stop1", "stop2", "stop3"}, result.Stop)
	})

	t.Run("merge extra fields", func(t *testing.T) {
		base := &Options{
			Model: "gpt-4",
			Extra: map[string]any{"key1": "value1"},
		}
		opt1 := &Options{
			Extra: map[string]any{"key2": "value2"},
		}

		result, err := MergeOptions(base, opt1)
		require.NoError(t, err)
		assert.Equal(t, "value1", result.Extra["key1"])
		assert.Equal(t, "value2", result.Extra["key2"])
	})

	t.Run("merge with nil options", func(t *testing.T) {
		base, _ := NewOptions("gpt-4")

		result, err := MergeOptions(base, nil, nil)
		require.NoError(t, err)
		assert.Equal(t, base.Model, result.Model)
	})

	t.Run("merge multiple options", func(t *testing.T) {
		freq1 := 0.5
		freq2 := 0.8
		temp := 0.7

		base := &Options{
			Model:            "gpt-4",
			FrequencyPenalty: &freq1,
		}
		opt1 := &Options{
			FrequencyPenalty: &freq2,
		}
		opt2 := &Options{
			Temperature: &temp,
		}

		result, err := MergeOptions(base, opt1, opt2)
		require.NoError(t, err)
		assert.Equal(t, "gpt-4", result.Model)
		assert.Equal(t, freq2, *result.FrequencyPenalty)
		assert.Equal(t, temp, *result.Temperature)
	})

	t.Run("merge tools with duplicates", func(t *testing.T) {
		tool1 := &mockTool{name: "tool1"}
		tool2 := &mockTool{name: "tool2"}
		tool3 := &mockTool{name: "tool1"}

		base := &Options{
			Model: "gpt-4",
			Tools: []Tool{tool1},
		}
		opt1 := &Options{
			Tools: []Tool{tool2, tool3},
		}

		result, err := MergeOptions(base, opt1)
		require.NoError(t, err)
		assert.Len(t, result.Tools, 2)
	})
}

func TestNewRequest(t *testing.T) {
	t.Run("valid messages", func(t *testing.T) {
		messages := []Message{
			NewUserMessage("hello"),
			NewAssistantMessage("hi"),
		}

		req, err := NewRequest(messages)
		require.NoError(t, err)
		assert.NotNil(t, req)
		assert.Len(t, req.Messages, 2)
		assert.NotNil(t, req.Params)
	})

	t.Run("empty messages", func(t *testing.T) {
		req, err := NewRequest([]Message{})
		assert.Error(t, err)
		assert.Nil(t, req)
		assert.Contains(t, err.Error(), "must contain at least one valid message")
	})

	t.Run("all nil messages", func(t *testing.T) {
		messages := []Message{nil, nil}
		req, err := NewRequest(messages)
		assert.Error(t, err)
		assert.Nil(t, req)
	})

	t.Run("filter nil messages", func(t *testing.T) {
		messages := []Message{
			NewUserMessage("hello"),
			nil,
			NewAssistantMessage("hi"),
		}

		req, err := NewRequest(messages)
		require.NoError(t, err)
		assert.Len(t, req.Messages, 2)
	})
}

func TestRequest_Get(t *testing.T) {
	t.Run("get existing param", func(t *testing.T) {
		req, _ := NewRequest([]Message{NewUserMessage("test")})
		req.Set("key", "value")

		val, exists := req.Get("key")
		assert.True(t, exists)
		assert.Equal(t, "value", val)
	})

	t.Run("get non-existing param", func(t *testing.T) {
		req, _ := NewRequest([]Message{NewUserMessage("test")})

		val, exists := req.Get("nonexistent")
		assert.False(t, exists)
		assert.Nil(t, val)
	})

	t.Run("get with nil params", func(t *testing.T) {
		req := &Request{
			Messages: []Message{NewUserMessage("test")},
		}

		val, exists := req.Get("key")
		assert.False(t, exists)
		assert.Nil(t, val)
		assert.NotNil(t, req.Params)
	})
}

func TestRequest_Set(t *testing.T) {
	t.Run("set param", func(t *testing.T) {
		req, _ := NewRequest([]Message{NewUserMessage("test")})
		req.Set("key", "value")

		assert.Equal(t, "value", req.Params["key"])
	})

	t.Run("set multiple params", func(t *testing.T) {
		req, _ := NewRequest([]Message{NewUserMessage("test")})
		req.Set("key1", "value1")
		req.Set("key2", 123)

		assert.Equal(t, "value1", req.Params["key1"])
		assert.Equal(t, 123, req.Params["key2"])
	})

	t.Run("overwrite param", func(t *testing.T) {
		req, _ := NewRequest([]Message{NewUserMessage("test")})
		req.Set("key", "value1")
		req.Set("key", "value2")

		assert.Equal(t, "value2", req.Params["key"])
	})
}

func TestRequest_AppendToLastUserMessage(t *testing.T) {
	t.Run("append to existing user message", func(t *testing.T) {
		req, _ := NewRequest([]Message{
			NewUserMessage("hello"),
		})

		req.AppendToLastUserMessage("world")

		userMsg := req.Messages[0].(*UserMessage)
		assert.Equal(t, "hello\n\nworld", userMsg.Text)
	})

	t.Run("append to last user message with multiple messages", func(t *testing.T) {
		req, _ := NewRequest([]Message{
			NewUserMessage("first"),
			NewAssistantMessage("response"),
			NewUserMessage("second"),
		})

		req.AppendToLastUserMessage("appended")

		userMsg := req.Messages[2].(*UserMessage)
		assert.Equal(t, "second\n\nappended", userMsg.Text)

		firstUserMsg := req.Messages[0].(*UserMessage)
		assert.Equal(t, "first", firstUserMsg.Text)
	})

	t.Run("append with no user message", func(t *testing.T) {
		req, _ := NewRequest([]Message{
			NewSystemMessage("system"),
		})

		req.AppendToLastUserMessage("text")

		systemMsg := req.Messages[0].(*SystemMessage)
		assert.Equal(t, "system", systemMsg.Text)
	})
}

func TestRequest_ReplaceOfLastUserMessage(t *testing.T) {
	t.Run("replace existing user message", func(t *testing.T) {
		req, _ := NewRequest([]Message{
			NewUserMessage("old text"),
		})

		req.ReplaceOfLastUserMessage("new text")

		userMsg := req.Messages[0].(*UserMessage)
		assert.Equal(t, "new text", userMsg.Text)
	})

	t.Run("replace last user message with multiple messages", func(t *testing.T) {
		req, _ := NewRequest([]Message{
			NewUserMessage("first"),
			NewAssistantMessage("response"),
			NewUserMessage("second"),
		})

		req.ReplaceOfLastUserMessage("replaced")

		userMsg := req.Messages[2].(*UserMessage)
		assert.Equal(t, "replaced", userMsg.Text)

		firstUserMsg := req.Messages[0].(*UserMessage)
		assert.Equal(t, "first", firstUserMsg.Text)
	})

	t.Run("replace with no user message", func(t *testing.T) {
		req, _ := NewRequest([]Message{
			NewSystemMessage("system"),
		})

		req.ReplaceOfLastUserMessage("text")

		systemMsg := req.Messages[0].(*SystemMessage)
		assert.Equal(t, "system", systemMsg.Text)
	})
}

func TestRequest_UserMessage(t *testing.T) {
	t.Run("get existing user message", func(t *testing.T) {
		req, _ := NewRequest([]Message{
			NewUserMessage("hello"),
		})

		userMsg := req.UserMessage()
		assert.Equal(t, "hello", userMsg.Text)
	})

	t.Run("get last user message", func(t *testing.T) {
		req, _ := NewRequest([]Message{
			NewUserMessage("first"),
			NewAssistantMessage("response"),
			NewUserMessage("second"),
		})

		userMsg := req.UserMessage()
		assert.Equal(t, "second", userMsg.Text)
	})

	t.Run("no user message", func(t *testing.T) {
		req, _ := NewRequest([]Message{
			NewSystemMessage("system"),
		})

		userMsg := req.UserMessage()
		assert.Equal(t, "", userMsg.Text)
	})
}

func TestRequest_SystemMessage(t *testing.T) {
	t.Run("get existing system message", func(t *testing.T) {
		req, _ := NewRequest([]Message{
			NewSystemMessage("system instruction"),
		})

		sysMsg := req.SystemMessage()
		assert.Equal(t, "system instruction", sysMsg.Text)
	})

	t.Run("get last system message", func(t *testing.T) {
		req, _ := NewRequest([]Message{
			NewSystemMessage("first"),
			NewUserMessage("user"),
			NewSystemMessage("second"),
		})

		sysMsg := req.SystemMessage()
		assert.Equal(t, "second", sysMsg.Text)
	})

	t.Run("no system message", func(t *testing.T) {
		req, _ := NewRequest([]Message{
			NewUserMessage("user"),
		})

		sysMsg := req.SystemMessage()
		assert.Equal(t, "", sysMsg.Text)
	})
}

type mockTool struct {
	name string
}

func (m *mockTool) Metadata() ToolMetadata {
	return ToolMetadata{}
}

func (m *mockTool) Definition() ToolDefinition {
	return ToolDefinition{Name: m.name}
}
