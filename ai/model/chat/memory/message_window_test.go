package memory

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/model/chat"
)

// TestNewMessageWindowMemory tests the constructor
func TestNewMessageWindowMemory(t *testing.T) {
	t.Run("with default limit", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, err := NewMessageWindowMemory(inner)

		require.NoError(t, err)
		assert.NotNil(t, memory)
		assert.Equal(t, 10, memory.maximumMessages)
	})

	t.Run("with custom valid limit", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, err := NewMessageWindowMemory(inner, 20)

		require.NoError(t, err)
		assert.Equal(t, 20, memory.maximumMessages)
	})

	t.Run("with minimum valid limit", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, err := NewMessageWindowMemory(inner, 10)

		require.NoError(t, err)
		assert.Equal(t, 10, memory.maximumMessages)
	})

	t.Run("with maximum valid limit", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, err := NewMessageWindowMemory(inner, 100)

		require.NoError(t, err)
		assert.Equal(t, 100, memory.maximumMessages)
	})

	t.Run("with limit below minimum clamped to 10", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, err := NewMessageWindowMemory(inner, 5)

		require.NoError(t, err)
		assert.Equal(t, 10, memory.maximumMessages)
	})

	t.Run("with limit above maximum clamped to 100", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, err := NewMessageWindowMemory(inner, 150)

		require.NoError(t, err)
		assert.Equal(t, 100, memory.maximumMessages)
	})

	t.Run("with zero limit defaults to 10", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, err := NewMessageWindowMemory(inner, 0)

		require.NoError(t, err)
		assert.Equal(t, 10, memory.maximumMessages)
	})

	t.Run("with negative limit clamped to 10", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, err := NewMessageWindowMemory(inner, -5)

		require.NoError(t, err)
		assert.Equal(t, 10, memory.maximumMessages)
	})

	t.Run("with nil inner memory", func(t *testing.T) {
		memory, err := NewMessageWindowMemory(nil)

		assert.Error(t, err)
		assert.Nil(t, memory)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("avoid double wrapping", func(t *testing.T) {
		inner := NewInMemoryMemory()
		first, _ := NewMessageWindowMemory(inner, 15)
		second, err := NewMessageWindowMemory(first, 20)

		require.NoError(t, err)
		assert.Equal(t, first, second)
	})

	t.Run("with multiple limit values uses first", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, err := NewMessageWindowMemory(inner, 25, 30, 35)

		require.NoError(t, err)
		assert.Equal(t, 25, memory.maximumMessages)
	})

	t.Run("with empty limit slice defaults to 10", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, err := NewMessageWindowMemory(inner)

		require.NoError(t, err)
		assert.Equal(t, 10, memory.maximumMessages)
	})
}

// TestWrite tests the write operation
func TestWrite(t *testing.T) {
	ctx := context.Background()

	t.Run("write single message", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 10)

		msg := chat.NewUserMessage("Hello")
		err := memory.Write(ctx, "conv1", msg)

		require.NoError(t, err)

		messages, _ := memory.Read(ctx, "conv1")
		assert.Len(t, messages, 1)
	})

	t.Run("write multiple messages", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 15)

		msg1 := chat.NewUserMessage("First")
		msg2 := chat.NewAssistantMessage("Second")
		msg3 := chat.NewUserMessage("Third")

		err := memory.Write(ctx, "conv1", msg1, msg2, msg3)
		require.NoError(t, err)

		messages, _ := memory.Read(ctx, "conv1")
		assert.Len(t, messages, 3)
	})

	t.Run("write delegates to inner memory", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 20)

		msg := chat.NewUserMessage("Test")
		memory.Write(ctx, "conv1", msg)

		// Verify inner memory has the message
		innerMessages, _ := inner.Read(ctx, "conv1")
		assert.Len(t, innerMessages, 1)
	})
}

// TestRead_WithinLimit tests reading when messages are within the limit
func TestRead_WithinLimit(t *testing.T) {
	ctx := context.Background()

	t.Run("empty conversation", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 10)

		messages, err := memory.Read(ctx, "conv1")

		require.NoError(t, err)
		assert.Empty(t, messages)
	})

	t.Run("messages below limit", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 20)

		for i := 0; i < 15; i++ {
			memory.Write(ctx, "conv1", chat.NewUserMessage("message"))
		}

		messages, _ := memory.Read(ctx, "conv1")
		assert.Len(t, messages, 15)
	})

	t.Run("messages exactly at limit", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 10)

		for i := 0; i < 10; i++ {
			memory.Write(ctx, "conv1", chat.NewUserMessage("message"))
		}

		messages, _ := memory.Read(ctx, "conv1")
		assert.Len(t, messages, 10)
	})

	t.Run("single message within large limit", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 100)

		memory.Write(ctx, "conv1", chat.NewUserMessage("message"))

		messages, _ := memory.Read(ctx, "conv1")
		assert.Len(t, messages, 1)
	})
}

// TestRead_ExceedingLimit tests reading when messages exceed the limit
func TestRead_ExceedingLimit(t *testing.T) {
	ctx := context.Background()

	t.Run("keeps recent messages with minimum limit", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 10)

		// Write 15 messages (exceeds minimum limit)
		for i := 0; i < 15; i++ {
			memory.Write(ctx, "conv1", chat.NewUserMessage("message"))
		}

		messages, _ := memory.Read(ctx, "conv1")
		assert.Len(t, messages, 10)
	})

	t.Run("keeps recent messages with mid-range limit", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 50)

		// Write 75 messages
		for i := 0; i < 75; i++ {
			memory.Write(ctx, "conv1", chat.NewUserMessage("message"))
		}

		messages, _ := memory.Read(ctx, "conv1")
		assert.Len(t, messages, 50)
	})

	t.Run("keeps recent messages with maximum limit", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 100)

		// Write 150 messages
		for i := 0; i < 150; i++ {
			memory.Write(ctx, "conv1", chat.NewUserMessage("message"))
		}

		messages, _ := memory.Read(ctx, "conv1")
		assert.Len(t, messages, 100)
	})

	t.Run("preserves message order", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 15)

		// Write messages
		for i := 0; i < 25; i++ {
			memory.Write(ctx, "conv1", chat.NewUserMessage("message"))
		}

		messages, _ := memory.Read(ctx, "conv1")
		assert.Len(t, messages, 15)

		// All messages should be user messages
		for _, msg := range messages {
			assert.Equal(t, chat.MessageTypeUser, msg.Type())
		}
	})
}

// TestRead_WithSystemMessages tests system message handling
func TestRead_WithSystemMessages(t *testing.T) {
	ctx := context.Background()

	t.Run("preserves single system message within limit", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 20)

		sysMsg := chat.NewSystemMessage("You are a helpful assistant")
		memory.Write(ctx, "conv1", sysMsg)

		for i := 0; i < 15; i++ {
			memory.Write(ctx, "conv1", chat.NewUserMessage("message"))
		}

		messages, _ := memory.Read(ctx, "conv1")
		assert.Len(t, messages, 16)
		assert.Equal(t, chat.MessageTypeSystem, messages[0].Type())
	})

	t.Run("preserves single system message exceeding limit", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 10)

		sysMsg := chat.NewSystemMessage("You are a helpful assistant")
		memory.Write(ctx, "conv1", sysMsg)

		for i := 0; i < 20; i++ {
			memory.Write(ctx, "conv1", chat.NewUserMessage("message"))
		}

		messages, _ := memory.Read(ctx, "conv1")
		assert.Len(t, messages, 10)
		assert.Equal(t, chat.MessageTypeSystem, messages[0].Type())
	})

	t.Run("merges multiple system messages", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 15)

		sysMsg1 := chat.NewSystemMessage("First instruction")
		sysMsg2 := chat.NewSystemMessage("Second instruction")
		memory.Write(ctx, "conv1", sysMsg1)
		memory.Write(ctx, "conv1", sysMsg2)

		for i := 0; i < 20; i++ {
			memory.Write(ctx, "conv1", chat.NewUserMessage("message"))
		}

		messages, _ := memory.Read(ctx, "conv1")
		assert.Len(t, messages, 15)

		// Should have one merged system message
		systemCount := 0
		for _, msg := range messages {
			if msg.Type() == chat.MessageTypeSystem {
				systemCount++
			}
		}
		assert.Equal(t, 1, systemCount)
	})

	t.Run("system message reduces available slots", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 10)

		sysMsg := chat.NewSystemMessage("System")
		memory.Write(ctx, "conv1", sysMsg)

		for i := 0; i < 20; i++ {
			memory.Write(ctx, "conv1", chat.NewUserMessage("message"))
		}

		messages, _ := memory.Read(ctx, "conv1")
		assert.Len(t, messages, 10)

		// Should have 1 system + 9 user messages
		systemCount := 0
		userCount := 0
		for _, msg := range messages {
			if msg.Type() == chat.MessageTypeSystem {
				systemCount++
			} else if msg.Type() == chat.MessageTypeUser {
				userCount++
			}
		}
		assert.Equal(t, 1, systemCount)
		assert.Equal(t, 9, userCount)
	})

	t.Run("system message with maximum limit", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 100)

		sysMsg := chat.NewSystemMessage("System")
		memory.Write(ctx, "conv1", sysMsg)

		for i := 0; i < 120; i++ {
			memory.Write(ctx, "conv1", chat.NewUserMessage("message"))
		}

		messages, _ := memory.Read(ctx, "conv1")
		assert.Len(t, messages, 100)

		// Should have 1 system + 99 user messages
		systemCount := 0
		userCount := 0
		for _, msg := range messages {
			if msg.Type() == chat.MessageTypeSystem {
				systemCount++
			} else if msg.Type() == chat.MessageTypeUser {
				userCount++
			}
		}
		assert.Equal(t, 1, systemCount)
		assert.Equal(t, 99, userCount)
	})
}

// TestRead_WithMixedMessageTypes tests handling of different message types
func TestRead_WithMixedMessageTypes(t *testing.T) {
	ctx := context.Background()

	t.Run("filters and keeps non-system messages", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 20)

		sysMsg := chat.NewSystemMessage("System")
		memory.Write(ctx, "conv1", sysMsg)

		// Add user and assistant messages
		for i := 0; i < 15; i++ {
			memory.Write(ctx, "conv1",
				chat.NewUserMessage("question"),
				chat.NewAssistantMessage("answer"),
			)
		}

		messages, _ := memory.Read(ctx, "conv1")
		assert.Len(t, messages, 20)
		assert.Equal(t, chat.MessageTypeSystem, messages[0].Type())
	})

	t.Run("handles tool messages", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 15)

		// Add various message types
		memory.Write(ctx, "conv1", chat.NewSystemMessage("System"))
		memory.Write(ctx, "conv1", chat.NewUserMessage("What's the weather?"))

		toolCall := &chat.ToolCall{
			ID:        "call_123",
			Name:      "get_weather",
			Arguments: `{"location": "NYC"}`,
		}
		memory.Write(ctx, "conv1", chat.NewAssistantMessage([]*chat.ToolCall{toolCall}))

		toolReturn := &chat.ToolReturn{
			ID:     "call_123",
			Name:   "get_weather",
			Result: `{"temp": 72}`,
		}
		toolMsg, _ := chat.NewToolMessage([]*chat.ToolReturn{toolReturn})
		memory.Write(ctx, "conv1", toolMsg)

		memory.Write(ctx, "conv1", chat.NewAssistantMessage("It's 72°F"))

		messages, _ := memory.Read(ctx, "conv1")
		assert.Len(t, messages, 5)

		// Verify order: system, user, assistant (with tool), tool, assistant
		assert.Equal(t, chat.MessageTypeSystem, messages[0].Type())
		assert.Equal(t, chat.MessageTypeUser, messages[1].Type())
		assert.Equal(t, chat.MessageTypeAssistant, messages[2].Type())
		assert.Equal(t, chat.MessageTypeTool, messages[3].Type())
		assert.Equal(t, chat.MessageTypeAssistant, messages[4].Type())
	})

	t.Run("complete conversation flow with window", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 12)

		// Write a conversation that exceeds the limit
		memory.Write(ctx, "conv1", chat.NewSystemMessage("System"))

		for i := 0; i < 10; i++ {
			memory.Write(ctx, "conv1",
				chat.NewUserMessage("question"),
				chat.NewAssistantMessage("answer"),
			)
		}

		messages, _ := memory.Read(ctx, "conv1")
		assert.Len(t, messages, 12)

		// Should have: 1 system + 11 recent non-system messages
		assert.Equal(t, chat.MessageTypeSystem, messages[0].Type())
	})

	t.Run("conversation with tool calls exceeding limit", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 30)

		memory.Write(ctx, "conv1", chat.NewSystemMessage("System"))

		// Add 20 complete tool call cycles
		for i := 0; i < 20; i++ {
			memory.Write(ctx, "conv1", chat.NewUserMessage("question"))

			toolCall := &chat.ToolCall{
				ID:        "call_123",
				Name:      "test",
				Arguments: `{}`,
			}
			memory.Write(ctx, "conv1", chat.NewAssistantMessage([]*chat.ToolCall{toolCall}))

			toolReturn := &chat.ToolReturn{
				ID:     "call_123",
				Name:   "test",
				Result: "result",
			}
			toolMsg, _ := chat.NewToolMessage([]*chat.ToolReturn{toolReturn})
			memory.Write(ctx, "conv1", toolMsg)

			memory.Write(ctx, "conv1", chat.NewAssistantMessage("answer"))
		}

		messages, _ := memory.Read(ctx, "conv1")
		assert.Len(t, messages, 30)
		assert.Equal(t, chat.MessageTypeSystem, messages[0].Type())
	})
}

// TestRead_EdgeCases tests edge cases
func TestRead_EdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("only system messages", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 15)

		for i := 0; i < 10; i++ {
			memory.Write(ctx, "conv1", chat.NewSystemMessage("system"))
		}

		messages, _ := memory.Read(ctx, "conv1")
		assert.Len(t, messages, 1) // Merged into one
		assert.Equal(t, chat.MessageTypeSystem, messages[0].Type())
	})

	t.Run("exactly one message per type", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 10)

		memory.Write(ctx, "conv1", chat.NewSystemMessage("system"))
		memory.Write(ctx, "conv1", chat.NewUserMessage("user"))
		memory.Write(ctx, "conv1", chat.NewAssistantMessage("assistant"))

		toolReturn := &chat.ToolReturn{
			ID:     "1",
			Name:   "test",
			Result: "result",
		}
		toolMsg, _ := chat.NewToolMessage([]*chat.ToolReturn{toolReturn})
		memory.Write(ctx, "conv1", toolMsg)

		messages, _ := memory.Read(ctx, "conv1")
		assert.Len(t, messages, 4)
	})

	t.Run("exactly at limit with system message", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 10)

		memory.Write(ctx, "conv1", chat.NewSystemMessage("system"))

		for i := 0; i < 10; i++ {
			memory.Write(ctx, "conv1", chat.NewUserMessage("message"))
		}

		messages, _ := memory.Read(ctx, "conv1")
		assert.Len(t, messages, 10)
		assert.Equal(t, chat.MessageTypeSystem, messages[0].Type())

		// Should have 9 user messages (10 - 1 system)
		userCount := 0
		for _, msg := range messages {
			if msg.Type() == chat.MessageTypeUser {
				userCount++
			}
		}
		assert.Equal(t, 9, userCount)
	})

	t.Run("one message over limit", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 10)

		memory.Write(ctx, "conv1", chat.NewSystemMessage("system"))

		for i := 0; i < 11; i++ {
			memory.Write(ctx, "conv1", chat.NewUserMessage("message"))
		}

		messages, _ := memory.Read(ctx, "conv1")
		assert.Len(t, messages, 10)
		assert.Equal(t, chat.MessageTypeSystem, messages[0].Type())

		// Should have 9 user messages
		userCount := 0
		for _, msg := range messages {
			if msg.Type() == chat.MessageTypeUser {
				userCount++
			}
		}
		assert.Equal(t, 9, userCount)
	})

	t.Run("maximum limit with many messages", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 100)

		memory.Write(ctx, "conv1", chat.NewSystemMessage("system"))

		for i := 0; i < 200; i++ {
			memory.Write(ctx, "conv1", chat.NewUserMessage("message"))
		}

		messages, _ := memory.Read(ctx, "conv1")
		assert.Len(t, messages, 100)
		assert.Equal(t, chat.MessageTypeSystem, messages[0].Type())
	})
}

// TestClear tests the clear operation
func TestClear(t *testing.T) {
	ctx := context.Background()

	t.Run("clear existing conversation", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 15)

		memory.Write(ctx, "conv1", chat.NewUserMessage("message"))
		memory.Clear(ctx, "conv1")

		messages, _ := memory.Read(ctx, "conv1")
		assert.Empty(t, messages)
	})

	t.Run("clear non-existent conversation", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 20)

		err := memory.Clear(ctx, "nonexistent")
		require.NoError(t, err)
	})

	t.Run("clear delegates to inner memory", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 25)

		memory.Write(ctx, "conv1", chat.NewUserMessage("message"))
		memory.Clear(ctx, "conv1")

		// Verify inner memory is also cleared
		innerMessages, _ := inner.Read(ctx, "conv1")
		assert.Empty(t, innerMessages)
	})

	t.Run("clear and rewrite", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 30)

		memory.Write(ctx, "conv1", chat.NewUserMessage("first"))
		memory.Clear(ctx, "conv1")
		memory.Write(ctx, "conv1", chat.NewUserMessage("second"))

		messages, _ := memory.Read(ctx, "conv1")
		assert.Len(t, messages, 1)
	})
}

// TestMultipleConversations tests handling of multiple conversations
func TestMultipleConversations(t *testing.T) {
	ctx := context.Background()

	t.Run("independent conversation windows", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 15)

		// Fill conv1 beyond limit
		for i := 0; i < 25; i++ {
			memory.Write(ctx, "conv1", chat.NewUserMessage("conv1"))
		}

		// Fill conv2 within limit
		for i := 0; i < 10; i++ {
			memory.Write(ctx, "conv2", chat.NewUserMessage("conv2"))
		}

		messages1, _ := memory.Read(ctx, "conv1")
		messages2, _ := memory.Read(ctx, "conv2")

		assert.Len(t, messages1, 15)
		assert.Len(t, messages2, 10)
	})

	t.Run("clearing one conversation doesn't affect others", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 20)

		memory.Write(ctx, "conv1", chat.NewUserMessage("conv1"))
		memory.Write(ctx, "conv2", chat.NewUserMessage("conv2"))

		memory.Clear(ctx, "conv1")

		messages1, _ := memory.Read(ctx, "conv1")
		messages2, _ := memory.Read(ctx, "conv2")

		assert.Empty(t, messages1)
		assert.Len(t, messages2, 1)
	})

	t.Run("different limits for different instances", func(t *testing.T) {
		inner1 := NewInMemoryMemory()
		memory1, _ := NewMessageWindowMemory(inner1, 10)

		inner2 := NewInMemoryMemory()
		memory2, _ := NewMessageWindowMemory(inner2, 50)

		for i := 0; i < 60; i++ {
			memory1.Write(ctx, "conv1", chat.NewUserMessage("message"))
			memory2.Write(ctx, "conv1", chat.NewUserMessage("message"))
		}

		messages1, _ := memory1.Read(ctx, "conv1")
		messages2, _ := memory2.Read(ctx, "conv1")

		assert.Len(t, messages1, 10)
		assert.Len(t, messages2, 50)
	})
}

// TestConcurrentOperations tests thread safety
func TestConcurrentOperations(t *testing.T) {
	ctx := context.Background()

	t.Run("concurrent writes", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 50)

		var wg sync.WaitGroup
		goroutines := 10
		messagesPerGoroutine := 5

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < messagesPerGoroutine; j++ {
					memory.Write(ctx, "conv1", chat.NewUserMessage("message"))
				}
			}()
		}

		wg.Wait()

		messages, _ := memory.Read(ctx, "conv1")
		assert.LessOrEqual(t, len(messages), 50)
	})

	t.Run("concurrent reads and writes", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 30)

		var wg sync.WaitGroup

		// Initialize with some data
		for i := 0; i < 10; i++ {
			memory.Write(ctx, "conv1", chat.NewUserMessage("init"))
		}

		// Concurrent operations
		for i := 0; i < 20; i++ {
			wg.Add(2)

			go func() {
				defer wg.Done()
				memory.Write(ctx, "conv1", chat.NewUserMessage("writer"))
			}()

			go func() {
				defer wg.Done()
				_, err := memory.Read(ctx, "conv1")
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})

	t.Run("concurrent operations on different conversations", func(t *testing.T) {
		inner := NewInMemoryMemory()
		memory, _ := NewMessageWindowMemory(inner, 25)

		var wg sync.WaitGroup
		conversations := 5

		for i := 0; i < conversations; i++ {
			wg.Add(3)
			convID := string(rune('A' + i))

			go func(id string) {
				defer wg.Done()
				memory.Write(ctx, id, chat.NewUserMessage("message"))
			}(convID)

			go func(id string) {
				defer wg.Done()
				memory.Read(ctx, id)
			}(convID)

			go func(id string, index int) {
				defer wg.Done()
				if index%2 == 0 {
					memory.Clear(ctx, id)
				}
			}(convID, i)
		}

		wg.Wait()
	})
}

// TestApplySlidingWindow tests the internal sliding window logic
func TestApplySlidingWindow(t *testing.T) {
	inner := NewInMemoryMemory()
	memory, _ := NewMessageWindowMemory(inner, 15)

	t.Run("empty input", func(t *testing.T) {
		result := memory.applySlidingWindow([]chat.Message{})
		assert.Empty(t, result)
	})

	t.Run("input within limit", func(t *testing.T) {
		messages := []chat.Message{
			chat.NewUserMessage("1"),
			chat.NewUserMessage("2"),
			chat.NewUserMessage("3"),
		}

		result := memory.applySlidingWindow(messages)
		assert.Len(t, result, 3)
	})

	t.Run("input exceeds limit with no system messages", func(t *testing.T) {
		messages := make([]chat.Message, 25)
		for i := 0; i < 25; i++ {
			messages[i] = chat.NewUserMessage("message")
		}

		result := memory.applySlidingWindow(messages)
		assert.Len(t, result, 15)
	})

	t.Run("input exceeds limit with system messages", func(t *testing.T) {
		messages := []chat.Message{
			chat.NewSystemMessage("sys1"),
			chat.NewSystemMessage("sys2"),
		}

		for i := 0; i < 20; i++ {
			messages = append(messages, chat.NewUserMessage("message"))
		}

		result := memory.applySlidingWindow(messages)
		assert.Len(t, result, 15)
		assert.Equal(t, chat.MessageTypeSystem, result[0].Type())
	})
}

// TestErrorPropagation tests error handling from inner memory
func TestErrorPropagation(t *testing.T) {
	ctx := context.Background()

	t.Run("read error from inner memory", func(t *testing.T) {
		failingMemory := &failingMemory{
			readError: errors.New("read failed"),
		}

		memory, _ := NewMessageWindowMemory(failingMemory, 20)

		_, err := memory.Read(ctx, "conv1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "read failed")
	})

	t.Run("write error from inner memory", func(t *testing.T) {
		failingMemory := &failingMemory{
			writeError: errors.New("write failed"),
		}

		memory, _ := NewMessageWindowMemory(failingMemory, 25)

		err := memory.Write(ctx, "conv1", chat.NewUserMessage("test"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "write failed")
	})

	t.Run("clear error from inner memory", func(t *testing.T) {
		failingMemory := &failingMemory{
			clearError: errors.New("clear failed"),
		}

		memory, _ := NewMessageWindowMemory(failingMemory, 30)

		err := memory.Clear(ctx, "conv1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "clear failed")
	})
}

// failingMemory is a mock memory implementation that returns errors
type failingMemory struct {
	readError  error
	writeError error
	clearError error
}

func (f *failingMemory) Write(ctx context.Context, conversationID string, messages ...chat.Message) error {
	if f.writeError != nil {
		return f.writeError
	}
	return nil
}

func (f *failingMemory) Read(ctx context.Context, conversationID string) ([]chat.Message, error) {
	if f.readError != nil {
		return nil, f.readError
	}
	return []chat.Message{}, nil
}

func (f *failingMemory) Clear(ctx context.Context, conversationID string) error {
	if f.clearError != nil {
		return f.clearError
	}
	return nil
}

// BenchmarkRead benchmarks read performance
func BenchmarkMessageWindowRead(b *testing.B) {
	ctx := context.Background()
	inner := NewInMemoryMemory()
	memory, _ := NewMessageWindowMemory(inner, 50)

	// Prepare data
	for i := 0; i < 100; i++ {
		memory.Write(ctx, "bench", chat.NewUserMessage("test"))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		memory.Read(ctx, "bench")
	}
}

// BenchmarkWrite benchmarks write performance
func BenchmarkMessageWindowWrite(b *testing.B) {
	ctx := context.Background()
	inner := NewInMemoryMemory()
	memory, _ := NewMessageWindowMemory(inner, 50)
	msg := chat.NewUserMessage("benchmark")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		memory.Write(ctx, "bench", msg)
	}
}

// BenchmarkApplySlidingWindow benchmarks the sliding window algorithm
func BenchmarkApplySlidingWindow(b *testing.B) {
	inner := NewInMemoryMemory()
	memory, _ := NewMessageWindowMemory(inner, 50)

	// Prepare messages
	messages := make([]chat.Message, 100)
	messages[0] = chat.NewSystemMessage("system")
	for i := 1; i < 100; i++ {
		messages[i] = chat.NewUserMessage("message")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		memory.applySlidingWindow(messages)
	}
}

// BenchmarkReadWithDifferentLimits benchmarks read performance with different limits
func BenchmarkReadWithDifferentLimits(b *testing.B) {
	ctx := context.Background()
	limits := []int{10, 25, 50, 100}

	for _, limit := range limits {
		b.Run(string(rune('0'+limit)), func(b *testing.B) {
			inner := NewInMemoryMemory()
			memory, _ := NewMessageWindowMemory(inner, limit)

			// Prepare data exceeding the limit
			for i := 0; i < limit*2; i++ {
				memory.Write(ctx, "bench", chat.NewUserMessage("test"))
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				memory.Read(ctx, "bench")
			}
		})
	}
}
