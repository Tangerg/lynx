package chat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/media"
	"github.com/Tangerg/lynx/pkg/mime"
)

// TestMessageType tests the MessageType enum and its methods
func TestMessageType(t *testing.T) {
	tests := []struct {
		name           string
		messageType    MessageType
		expectedString string
		isSystem       bool
		isUser         bool
		isAssistant    bool
		isTool         bool
	}{
		{
			name:           "System message type",
			messageType:    MessageTypeSystem,
			expectedString: "system",
			isSystem:       true,
			isUser:         false,
			isAssistant:    false,
			isTool:         false,
		},
		{
			name:           "User message type",
			messageType:    MessageTypeUser,
			expectedString: "user",
			isSystem:       false,
			isUser:         true,
			isAssistant:    false,
			isTool:         false,
		},
		{
			name:           "Assistant message type",
			messageType:    MessageTypeAssistant,
			expectedString: "assistant",
			isSystem:       false,
			isUser:         false,
			isAssistant:    true,
			isTool:         false,
		},
		{
			name:           "Tool message type",
			messageType:    MessageTypeTool,
			expectedString: "tool",
			isSystem:       false,
			isUser:         false,
			isAssistant:    false,
			isTool:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedString, tt.messageType.String())
			assert.Equal(t, tt.isSystem, tt.messageType.IsSystem())
			assert.Equal(t, tt.isUser, tt.messageType.IsUser())
			assert.Equal(t, tt.isAssistant, tt.messageType.IsAssistant())
			assert.Equal(t, tt.isTool, tt.messageType.IsTool())
		})
	}
}

// TestNewSystemMessage tests system message creation
func TestNewSystemMessage(t *testing.T) {
	t.Run("Create from string", func(t *testing.T) {
		text := "You are a helpful assistant"
		msg := NewSystemMessage(text)

		assert.NotNil(t, msg)
		assert.Equal(t, MessageTypeSystem, msg.Type())
		assert.Equal(t, text, msg.Text)
		assert.NotNil(t, msg.Metadata)
		assert.Empty(t, msg.Metadata)
	})

	t.Run("Create from MessageParams", func(t *testing.T) {
		params := MessageParams{
			Type: MessageTypeSystem,
			Text: "System instructions",
			Metadata: map[string]any{
				"priority": "high",
				"version":  1,
			},
		}
		msg := NewSystemMessage(params)

		assert.NotNil(t, msg)
		assert.Equal(t, MessageTypeSystem, msg.Type())
		assert.Equal(t, params.Text, msg.Text)
		assert.Equal(t, "high", msg.Metadata["priority"])
		assert.Equal(t, 1, msg.Metadata["version"])
	})

	t.Run("Create with empty metadata", func(t *testing.T) {
		params := MessageParams{
			Text: "Test",
		}
		msg := NewSystemMessage(params)

		assert.NotNil(t, msg.Metadata)
		assert.Empty(t, msg.Metadata)
	})
}

// TestNewUserMessage tests user message creation
func TestNewUserMessage(t *testing.T) {
	t.Run("Create from string", func(t *testing.T) {
		text := "Hello, how are you?"
		msg := NewUserMessage(text)

		assert.NotNil(t, msg)
		assert.Equal(t, MessageTypeUser, msg.Type())
		assert.Equal(t, text, msg.Text)
		assert.NotNil(t, msg.Media)
		assert.Empty(t, msg.Media)
		assert.NotNil(t, msg.Metadata)
		assert.Empty(t, msg.Metadata)
	})

	t.Run("Create from media slice", func(t *testing.T) {
		mediaList := []*media.Media{
			{MimeType: mime.MustNew("image", "png")},
			{MimeType: mime.MustNew("image", "jpeg")},
		}
		msg := NewUserMessage(mediaList)

		assert.NotNil(t, msg)
		assert.Equal(t, MessageTypeUser, msg.Type())
		assert.Len(t, msg.Media, 2)
		assert.True(t, msg.HasMedia())
	})

	t.Run("Create from MessageParams", func(t *testing.T) {
		params := MessageParams{
			Type: MessageTypeUser,
			Text: "What's in this image?",
			Media: []*media.Media{
				{MimeType: mime.MustNew("image", "png")},
			},
			Metadata: map[string]any{
				"source": "mobile_app",
			},
		}
		msg := NewUserMessage(params)

		assert.NotNil(t, msg)
		assert.Equal(t, params.Text, msg.Text)
		assert.Len(t, msg.Media, 1)
		assert.Equal(t, "mobile_app", msg.Metadata["source"])
		assert.True(t, msg.HasMedia())
	})

	t.Run("HasMedia returns false for empty media", func(t *testing.T) {
		msg := NewUserMessage("text only")
		assert.False(t, msg.HasMedia())
	})
}

// TestNewAssistantMessage tests assistant message creation
func TestNewAssistantMessage(t *testing.T) {
	t.Run("Create from string", func(t *testing.T) {
		text := "I'm doing well, thank you!"
		msg := NewAssistantMessage(text)

		assert.NotNil(t, msg)
		assert.Equal(t, MessageTypeAssistant, msg.Type())
		assert.Equal(t, text, msg.Text)
		assert.NotNil(t, msg.Media)
		assert.Empty(t, msg.Media)
		assert.NotNil(t, msg.ToolCalls)
		assert.Empty(t, msg.ToolCalls)
	})

	t.Run("Create from media slice", func(t *testing.T) {
		mediaList := []*media.Media{
			{MimeType: mime.MustNew("image", "png")},
		}
		msg := NewAssistantMessage(mediaList)

		assert.NotNil(t, msg)
		assert.Len(t, msg.Media, 1)
		assert.True(t, msg.HasMedia())
	})

	t.Run("Create from tool calls", func(t *testing.T) {
		toolCalls := []*ToolCall{
			{
				ID:        "call_1",
				Name:      "get_weather",
				Arguments: `{"location":"Beijing"}`,
			},
		}
		msg := NewAssistantMessage(toolCalls)

		assert.NotNil(t, msg)
		assert.Len(t, msg.ToolCalls, 1)
		assert.True(t, msg.HasToolCalls())
		assert.Equal(t, "call_1", msg.ToolCalls[0].ID)
	})

	t.Run("Create from metadata only", func(t *testing.T) {
		metadata := map[string]any{
			"confidence": 0.95,
			"model":      "gpt-4",
		}
		msg := NewAssistantMessage(metadata)

		assert.NotNil(t, msg)
		assert.Equal(t, 0.95, msg.Metadata["confidence"])
		assert.Equal(t, "gpt-4", msg.Metadata["model"])
	})

	t.Run("Create from MessageParams with all fields", func(t *testing.T) {
		params := MessageParams{
			Type: MessageTypeAssistant,
			Text: "Let me check the weather for you",
			Media: []*media.Media{
				{MimeType: mime.MustNew("image", "png")},
			},
			ToolCalls: []*ToolCall{
				{
					ID:        "call_1",
					Name:      "get_weather",
					Arguments: `{"location":"Beijing"}`,
				},
			},
			Metadata: map[string]any{
				"model": "gpt-4",
			},
		}
		msg := NewAssistantMessage(params)

		assert.NotNil(t, msg)
		assert.Equal(t, params.Text, msg.Text)
		assert.Len(t, msg.Media, 1)
		assert.Len(t, msg.ToolCalls, 1)
		assert.Equal(t, "gpt-4", msg.Metadata["model"])
		assert.True(t, msg.HasMedia())
		assert.True(t, msg.HasToolCalls())
	})

	t.Run("HasToolCalls returns false for empty tool calls", func(t *testing.T) {
		msg := NewAssistantMessage("text only")
		assert.False(t, msg.HasToolCalls())
	})
}

// TestNewToolMessage tests tool message creation
func TestNewToolMessage(t *testing.T) {
	t.Run("Create from tool returns", func(t *testing.T) {
		toolReturns := []*ToolReturn{
			{
				ID:     "call_1",
				Name:   "get_weather",
				Result: `{"temperature":20,"condition":"sunny"}`,
			},
		}
		msg, err := NewToolMessage(toolReturns)

		require.NoError(t, err)
		assert.NotNil(t, msg)
		assert.Equal(t, MessageTypeTool, msg.Type())
		assert.Len(t, msg.ToolReturns, 1)
		assert.Equal(t, "call_1", msg.ToolReturns[0].ID)
	})

	t.Run("Create from MessageParams", func(t *testing.T) {
		params := MessageParams{
			Type: MessageTypeTool,
			ToolReturns: []*ToolReturn{
				{
					ID:     "call_1",
					Name:   "get_weather",
					Result: `{"temperature":20}`,
				},
			},
			Metadata: map[string]any{
				"execution_time": "2ms",
			},
		}
		msg, err := NewToolMessage(params)

		require.NoError(t, err)
		assert.NotNil(t, msg)
		assert.Len(t, msg.ToolReturns, 1)
		assert.Equal(t, "2ms", msg.Metadata["execution_time"])
	})

	t.Run("Error on empty tool returns", func(t *testing.T) {
		msg, err := NewToolMessage([]*ToolReturn{})

		assert.Error(t, err)
		assert.Nil(t, msg)
		assert.Contains(t, err.Error(), "at least one")
	})
}

// TestNewMessage tests the message factory function
func TestNewMessage(t *testing.T) {
	t.Run("Create system message", func(t *testing.T) {
		params := MessageParams{
			Type: MessageTypeSystem,
			Text: "System prompt",
		}
		msg, err := NewMessage(params)

		require.NoError(t, err)
		assert.NotNil(t, msg)
		assert.Equal(t, MessageTypeSystem, msg.Type())
	})

	t.Run("Create user message", func(t *testing.T) {
		params := MessageParams{
			Type: MessageTypeUser,
			Text: "User query",
		}
		msg, err := NewMessage(params)

		require.NoError(t, err)
		assert.NotNil(t, msg)
		assert.Equal(t, MessageTypeUser, msg.Type())
	})

	t.Run("Create assistant message", func(t *testing.T) {
		params := MessageParams{
			Type: MessageTypeAssistant,
			Text: "Assistant response",
		}
		msg, err := NewMessage(params)

		require.NoError(t, err)
		assert.NotNil(t, msg)
		assert.Equal(t, MessageTypeAssistant, msg.Type())
	})

	t.Run("Create tool message", func(t *testing.T) {
		params := MessageParams{
			Type: MessageTypeTool,
			ToolReturns: []*ToolReturn{
				{ID: "1", Name: "test", Result: "result"},
			},
		}
		msg, err := NewMessage(params)

		require.NoError(t, err)
		assert.NotNil(t, msg)
		assert.Equal(t, MessageTypeTool, msg.Type())
	})

	t.Run("Error on unsupported type", func(t *testing.T) {
		params := MessageParams{
			Type: MessageType("invalid"),
		}
		msg, err := NewMessage(params)

		assert.Error(t, err)
		assert.Nil(t, msg)
		assert.Contains(t, err.Error(), "unsupported message type")
	})
}

// TestFilterMessages tests message filtering functionality
func TestFilterMessages(t *testing.T) {
	messages := []Message{
		NewSystemMessage("system"),
		NewUserMessage("user1"),
		NewAssistantMessage("assistant1"),
		NewUserMessage("user2"),
		nil,
	}

	t.Run("Filter by predicate", func(t *testing.T) {
		filtered := FilterMessages(messages, func(msg Message) bool {
			return msg != nil && msg.Type() == MessageTypeUser
		})

		assert.Len(t, filtered, 2)
		assert.Equal(t, MessageTypeUser, filtered[0].Type())
		assert.Equal(t, MessageTypeUser, filtered[1].Type())
	})

	t.Run("Filter out nil messages", func(t *testing.T) {
		filtered := filterOutNilMessages(messages)

		assert.Len(t, filtered, 4)
		for _, msg := range filtered {
			assert.NotNil(t, msg)
		}
	})

	t.Run("Empty slice returns empty result", func(t *testing.T) {
		filtered := FilterMessages([]Message{}, func(msg Message) bool {
			return true
		})

		assert.Empty(t, filtered)
	})

	t.Run("Panic on nil predicate", func(t *testing.T) {
		assert.Panics(t, func() {
			FilterMessages(messages, nil)
		})
	})
}

// TestFilterMessagesByMessageTypes tests filtering by message types
func TestFilterMessagesByMessageTypes(t *testing.T) {
	messages := []Message{
		NewSystemMessage("system"),
		NewUserMessage("user1"),
		NewAssistantMessage("assistant1"),
		NewUserMessage("user2"),
		nil,
	}

	t.Run("Filter single type", func(t *testing.T) {
		filtered := FilterMessagesByMessageTypes(messages, MessageTypeUser)

		assert.Len(t, filtered, 2)
		for _, msg := range filtered {
			assert.Equal(t, MessageTypeUser, msg.Type())
		}
	})

	t.Run("Filter multiple types", func(t *testing.T) {
		filtered := FilterMessagesByMessageTypes(messages, MessageTypeUser, MessageTypeAssistant)

		assert.Len(t, filtered, 3)
	})

	t.Run("No types specified returns all", func(t *testing.T) {
		filtered := FilterMessagesByMessageTypes(messages)

		assert.Equal(t, len(messages), len(filtered))
	})

	t.Run("Filter excludes nil messages", func(t *testing.T) {
		filtered := FilterMessagesByMessageTypes(messages, MessageTypeUser, MessageTypeAssistant, MessageTypeSystem)

		for _, msg := range filtered {
			assert.NotNil(t, msg)
		}
	})
}

// TestMergeSystemMessages tests system message merging
func TestMergeSystemMessages(t *testing.T) {
	t.Run("Merge multiple system messages", func(t *testing.T) {
		messages := []Message{
			NewSystemMessage(MessageParams{
				Text:     "First instruction",
				Metadata: map[string]any{"key1": "value1"},
			}),
			NewUserMessage("user message"),
			NewSystemMessage(MessageParams{
				Text:     "Second instruction",
				Metadata: map[string]any{"key2": "value2"},
			}),
		}

		merged := MergeSystemMessages(messages)

		require.NotNil(t, merged)
		assert.Contains(t, merged.Text, "First instruction")
		assert.Contains(t, merged.Text, "Second instruction")
		assert.Equal(t, "value1", merged.Metadata["key1"])
		assert.Equal(t, "value2", merged.Metadata["key2"])
	})

	t.Run("Single system message returns unchanged", func(t *testing.T) {
		messages := []Message{
			NewSystemMessage("Only system message"),
		}

		merged := MergeSystemMessages(messages)

		require.NotNil(t, merged)
		assert.Equal(t, "Only system message", merged.Text)
	})

	t.Run("No system messages returns nil", func(t *testing.T) {
		messages := []Message{
			NewUserMessage("user"),
			NewAssistantMessage("assistant"),
		}

		merged := MergeSystemMessages(messages)

		assert.Nil(t, merged)
	})

	t.Run("Metadata override with later values", func(t *testing.T) {
		messages := []Message{
			NewSystemMessage(MessageParams{
				Text:     "First",
				Metadata: map[string]any{"key": "old"},
			}),
			NewSystemMessage(MessageParams{
				Text:     "Second",
				Metadata: map[string]any{"key": "new"},
			}),
		}

		merged := MergeSystemMessages(messages)

		require.NotNil(t, merged)
		assert.Equal(t, "new", merged.Metadata["key"])
	})
}

// TestMergeUserMessages tests user message merging
func TestMergeUserMessages(t *testing.T) {
	t.Run("Merge multiple user messages", func(t *testing.T) {
		messages := []Message{
			NewUserMessage(MessageParams{
				Text:  "First question",
				Media: []*media.Media{{MimeType: mime.MustNew("image", "png")}},
			}),
			NewUserMessage(MessageParams{
				Text:  "Second question",
				Media: []*media.Media{{MimeType: mime.MustNew("image", "jpeg")}},
			}),
		}

		merged := MergeUserMessages(messages)

		require.NotNil(t, merged)
		assert.Contains(t, merged.Text, "First question")
		assert.Contains(t, merged.Text, "Second question")
		assert.Len(t, merged.Media, 2)
	})

	t.Run("Single user message returns unchanged", func(t *testing.T) {
		messages := []Message{
			NewUserMessage("Only user message"),
		}

		merged := MergeUserMessages(messages)

		require.NotNil(t, merged)
		assert.Equal(t, "Only user message", merged.Text)
	})

	t.Run("No user messages returns nil", func(t *testing.T) {
		messages := []Message{
			NewSystemMessage("system"),
			NewAssistantMessage("assistant"),
		}

		merged := MergeUserMessages(messages)

		assert.Nil(t, merged)
	})
}

// TestMergeToolMessages tests tool message merging
func TestMergeToolMessages(t *testing.T) {
	t.Run("Merge multiple tool messages", func(t *testing.T) {
		toolReturn1 := &ToolReturn{ID: "1", Name: "tool1", Result: "result1"}
		toolReturn2 := &ToolReturn{ID: "2", Name: "tool2", Result: "result2"}

		msg1, _ := NewToolMessage([]*ToolReturn{toolReturn1})
		msg2, _ := NewToolMessage([]*ToolReturn{toolReturn2})

		messages := []Message{msg1, msg2}

		merged, err := MergeToolMessages(messages)

		require.NoError(t, err)
		require.NotNil(t, merged)
		assert.Len(t, merged.ToolReturns, 2)
	})

	t.Run("Single tool message returns unchanged", func(t *testing.T) {
		toolReturn := &ToolReturn{ID: "1", Name: "tool1", Result: "result1"}
		msg, _ := NewToolMessage([]*ToolReturn{toolReturn})
		messages := []Message{msg}

		merged, err := MergeToolMessages(messages)

		require.NoError(t, err)
		require.NotNil(t, merged)
		assert.Len(t, merged.ToolReturns, 1)
	})

	t.Run("No tool messages returns nil", func(t *testing.T) {
		messages := []Message{
			NewUserMessage("user"),
		}

		merged, err := MergeToolMessages(messages)

		require.NoError(t, err)
		assert.Nil(t, merged)
	})
}

// TestMergeMessages tests the generic merge function
func TestMergeMessages(t *testing.T) {
	t.Run("Merge system messages", func(t *testing.T) {
		messages := []Message{
			NewSystemMessage("first"),
			NewSystemMessage("second"),
		}

		merged, err := MergeMessages(messages, MessageTypeSystem)

		require.NoError(t, err)
		assert.NotNil(t, merged)
		assert.Equal(t, MessageTypeSystem, merged.Type())
	})

	t.Run("Merge user messages", func(t *testing.T) {
		messages := []Message{
			NewUserMessage("first"),
			NewUserMessage("second"),
		}

		merged, err := MergeMessages(messages, MessageTypeUser)

		require.NoError(t, err)
		assert.NotNil(t, merged)
		assert.Equal(t, MessageTypeUser, merged.Type())
	})

	t.Run("Error on assistant message type", func(t *testing.T) {
		messages := []Message{
			NewAssistantMessage("assistant"),
		}

		merged, err := MergeMessages(messages, MessageTypeAssistant)

		assert.Error(t, err)
		assert.Nil(t, merged)
		assert.Contains(t, err.Error(), "unsupported message type for merging")
	})
}

// TestMergeAdjacentSameTypeMessages tests adjacent message merging
func TestMergeAdjacentSameTypeMessages(t *testing.T) {
	t.Run("Merge adjacent same type messages", func(t *testing.T) {
		messages := []Message{
			NewUserMessage("user1"),
			NewUserMessage("user2"),
			NewSystemMessage("system"),
			NewUserMessage("user3"),
		}

		merged := MergeAdjacentSameTypeMessages(messages)

		assert.Len(t, merged, 3)
		assert.Equal(t, MessageTypeUser, merged[0].Type())
		assert.Equal(t, MessageTypeSystem, merged[1].Type())
		assert.Equal(t, MessageTypeUser, merged[2].Type())
	})

	t.Run("No adjacent same types returns unchanged", func(t *testing.T) {
		messages := []Message{
			NewUserMessage("user"),
			NewSystemMessage("system"),
			NewAssistantMessage("assistant"),
		}

		merged := MergeAdjacentSameTypeMessages(messages)

		assert.Len(t, merged, 3)
	})

	t.Run("Empty slice returns empty", func(t *testing.T) {
		merged := MergeAdjacentSameTypeMessages([]Message{})

		assert.Empty(t, merged)
	})

	t.Run("Single message returns unchanged", func(t *testing.T) {
		messages := []Message{
			NewUserMessage("user"),
		}

		merged := MergeAdjacentSameTypeMessages(messages)

		assert.Len(t, merged, 1)
	})

	t.Run("Filter out nil messages", func(t *testing.T) {
		messages := []Message{
			NewUserMessage("user1"),
			nil,
			NewUserMessage("user2"),
		}

		merged := MergeAdjacentSameTypeMessages(messages)

		for _, msg := range merged {
			assert.NotNil(t, msg)
		}
	})
}

// TestAppendTextToLastMessageOfType tests text appending functionality
func TestAppendTextToLastMessageOfType(t *testing.T) {
	t.Run("Append to last user message", func(t *testing.T) {
		messages := []Message{
			NewUserMessage("Original text"),
			NewAssistantMessage("Response"),
		}

		appendTextToLastMessageOfType(messages, MessageTypeUser, "Additional text")

		userMsg := messages[0].(*UserMessage)
		assert.Contains(t, userMsg.Text, "Original text")
		assert.Contains(t, userMsg.Text, "Additional text")
		assert.Contains(t, userMsg.Text, "\n\n")
	})

	t.Run("Append to last system message", func(t *testing.T) {
		messages := []Message{
			NewSystemMessage("System prompt"),
			NewUserMessage("User query"),
		}

		appendTextToLastMessageOfType(messages, MessageTypeSystem, "Extra instructions")

		systemMsg := messages[0].(*SystemMessage)
		assert.Contains(t, systemMsg.Text, "System prompt")
		assert.Contains(t, systemMsg.Text, "Extra instructions")
	})

	t.Run("No effect on unsupported message types", func(t *testing.T) {
		messages := []Message{
			NewAssistantMessage("Assistant response"),
		}

		appendTextToLastMessageOfType(messages, MessageTypeAssistant, "Should not append")

		assistantMsg := messages[0].(*AssistantMessage)
		assert.Equal(t, "Assistant response", assistantMsg.Text)
	})

	t.Run("No effect when message type not found", func(t *testing.T) {
		messages := []Message{
			NewUserMessage("User message"),
		}

		appendTextToLastMessageOfType(messages, MessageTypeSystem, "Should not append")

		// Should not panic or modify anything
		assert.Len(t, messages, 1)
	})
}

// TestReplaceTextOfLastMessageOfType tests text replacement functionality
func TestReplaceTextOfLastMessageOfType(t *testing.T) {
	t.Run("Replace last user message text", func(t *testing.T) {
		messages := []Message{
			NewUserMessage("Original text"),
			NewAssistantMessage("Response"),
		}

		replaceTextOfLastMessageOfType(messages, MessageTypeUser, "New text")

		userMsg := messages[0].(*UserMessage)
		assert.Equal(t, "New text", userMsg.Text)
		assert.NotContains(t, userMsg.Text, "Original text")
	})

	t.Run("Replace last system message text", func(t *testing.T) {
		messages := []Message{
			NewSystemMessage("Old system prompt"),
		}

		replaceTextOfLastMessageOfType(messages, MessageTypeSystem, "New system prompt")

		systemMsg := messages[0].(*SystemMessage)
		assert.Equal(t, "New system prompt", systemMsg.Text)
	})

	t.Run("No effect on unsupported message types", func(t *testing.T) {
		messages := []Message{
			NewAssistantMessage("Assistant response"),
		}

		replaceTextOfLastMessageOfType(messages, MessageTypeAssistant, "New text")

		assistantMsg := messages[0].(*AssistantMessage)
		assert.Equal(t, "Assistant response", assistantMsg.Text)
	})
}

// TestMessageToString tests message to string conversion
func TestMessageToString(t *testing.T) {
	t.Run("Convert user message", func(t *testing.T) {
		msg := NewUserMessage("Hello world")
		str := MessageToString(msg)

		assert.Contains(t, str, "user:")
		assert.Contains(t, str, "Hello world")
	})

	t.Run("Convert system message", func(t *testing.T) {
		msg := NewSystemMessage("System instructions")
		str := MessageToString(msg)

		assert.Contains(t, str, "system:")
		assert.Contains(t, str, "System instructions")
	})

	t.Run("Convert assistant message with text", func(t *testing.T) {
		msg := NewAssistantMessage("Assistant response")
		str := MessageToString(msg)

		assert.Contains(t, str, "assistant:")
		assert.Contains(t, str, "Assistant response")
	})

	t.Run("Convert assistant message with tool calls", func(t *testing.T) {
		msg := NewAssistantMessage([]*ToolCall{
			{ID: "1", Name: "test", Arguments: `{"arg":"value"}`},
		})
		str := MessageToString(msg)

		assert.Contains(t, str, "assistant:")
		assert.Contains(t, str, "test")

		// Should contain valid JSON
		var toolCalls []*ToolCall
		lines := []byte(str[len("assistant: "):])
		err := json.Unmarshal(lines, &toolCalls)
		assert.NoError(t, err)
	})

	t.Run("Convert tool message", func(t *testing.T) {
		msg, _ := NewToolMessage([]*ToolReturn{
			{ID: "1", Name: "test", Result: "result"},
		})
		str := MessageToString(msg)

		assert.Contains(t, str, "tool:")

		// Should contain valid JSON
		var toolReturns []*ToolReturn
		lines := []byte(str[len("tool: "):])
		err := json.Unmarshal(lines, &toolReturns)
		assert.NoError(t, err)
	})
}

// TestMessagesToStrings tests batch message to string conversion
func TestMessagesToStrings(t *testing.T) {
	messages := []Message{
		NewSystemMessage("System"),
		NewUserMessage("User"),
		NewAssistantMessage("Assistant"),
	}

	strings := MessagesToStrings(messages)

	assert.Len(t, strings, 3)
	assert.Contains(t, strings[0], "system:")
	assert.Contains(t, strings[1], "user:")
	assert.Contains(t, strings[2], "assistant:")
}

// TestFindLastMessageIndexOfType tests finding last message by type
func TestFindLastMessageIndexOfType(t *testing.T) {
	messages := []Message{
		NewSystemMessage("system"),
		NewUserMessage("user1"),
		NewAssistantMessage("assistant"),
		NewUserMessage("user2"),
	}

	t.Run("Find existing message type", func(t *testing.T) {
		index, msg := findLastMessageIndexOfType(messages, MessageTypeUser)

		assert.Equal(t, 3, index)
		assert.NotNil(t, msg)
		assert.Equal(t, MessageTypeUser, msg.Type())
	})

	t.Run("Find non-existing message type", func(t *testing.T) {
		index, msg := findLastMessageIndexOfType(messages, MessageTypeTool)

		assert.Equal(t, -1, index)
		assert.Nil(t, msg)
	})

	t.Run("Skip nil messages", func(t *testing.T) {
		messagesWithNil := []Message{
			NewUserMessage("user1"),
			nil,
			NewUserMessage("user2"),
		}

		index, msg := findLastMessageIndexOfType(messagesWithNil, MessageTypeUser)

		assert.Equal(t, 2, index)
		assert.NotNil(t, msg)
	})
}

// TestHasMessageTypeAt tests message type checking at specific index
func TestHasMessageTypeAt(t *testing.T) {
	messages := []Message{
		NewSystemMessage("system"),
		NewUserMessage("user"),
		NewAssistantMessage("assistant"),
	}

	t.Run("Positive index", func(t *testing.T) {
		assert.True(t, hasMessageTypeAt(messages, 0, MessageTypeSystem))
		assert.True(t, hasMessageTypeAt(messages, 1, MessageTypeUser))
		assert.False(t, hasMessageTypeAt(messages, 1, MessageTypeSystem))
	})

	t.Run("Negative index", func(t *testing.T) {
		assert.True(t, hasMessageTypeAt(messages, -1, MessageTypeAssistant))
		assert.True(t, hasMessageTypeAt(messages, -2, MessageTypeUser))
		assert.True(t, hasMessageTypeAt(messages, -3, MessageTypeSystem))
	})

	t.Run("Out of bounds", func(t *testing.T) {
		assert.False(t, hasMessageTypeAt(messages, 10, MessageTypeUser))
		assert.False(t, hasMessageTypeAt(messages, -10, MessageTypeUser))
	})

	t.Run("Nil message", func(t *testing.T) {
		messagesWithNil := []Message{nil}
		assert.False(t, hasMessageTypeAt(messagesWithNil, 0, MessageTypeUser))
	})
}

// TestHasMessageTypeAtLast tests message type checking at last position
func TestHasMessageTypeAtLast(t *testing.T) {
	t.Run("Has expected type at last", func(t *testing.T) {
		messages := []Message{
			NewSystemMessage("system"),
			NewUserMessage("user"),
		}

		assert.True(t, hasMessageTypeAtLast(messages, MessageTypeUser))
		assert.False(t, hasMessageTypeAtLast(messages, MessageTypeSystem))
	})

	t.Run("Empty slice", func(t *testing.T) {
		assert.False(t, hasMessageTypeAtLast([]Message{}, MessageTypeUser))
	})

	t.Run("Nil at last", func(t *testing.T) {
		messages := []Message{
			NewUserMessage("user"),
			nil,
		}

		assert.False(t, hasMessageTypeAtLast(messages, MessageTypeUser))
	})
}
