package chat

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewInMemoryMemory tests the constructor
func TestNewInMemoryMemory(t *testing.T) {
	memory := NewInMemoryMemory()

	assert.NotNil(t, memory)
	assert.NotNil(t, memory.conversationMessages)
	assert.Equal(t, 0, len(memory.conversationMessages))
}

// TestWrite_SingleMessage tests writing a single message
func TestWrite_SingleMessage(t *testing.T) {
	ctx := context.Background()
	memory := NewInMemoryMemory()

	msg := NewUserMessage("Hello")
	err := memory.Write(ctx, "conv1", msg)

	require.NoError(t, err)

	messages, _ := memory.Read(ctx, "conv1")
	assert.Len(t, messages, 1)
	assert.Equal(t, MessageTypeUser, messages[0].Type())
}

// TestWrite_MultipleMessages tests writing multiple messages at once
func TestWrite_MultipleMessages(t *testing.T) {
	ctx := context.Background()
	memory := NewInMemoryMemory()

	msg1 := NewUserMessage("Question 1")
	msg2 := NewAssistantMessage("Answer 1")
	msg3 := NewUserMessage("Question 2")

	err := memory.Write(ctx, "conv1", msg1, msg2, msg3)
	require.NoError(t, err)

	messages, _ := memory.Read(ctx, "conv1")
	assert.Len(t, messages, 3)
	assert.Equal(t, MessageTypeUser, messages[0].Type())
	assert.Equal(t, MessageTypeAssistant, messages[1].Type())
	assert.Equal(t, MessageTypeUser, messages[2].Type())
}

// TestWrite_AppendToExistingConversation tests appending messages
func TestWrite_AppendToExistingConversation(t *testing.T) {
	ctx := context.Background()
	memory := NewInMemoryMemory()

	msg1 := NewUserMessage("First")
	err := memory.Write(ctx, "conv1", msg1)
	require.NoError(t, err)

	msg2 := NewUserMessage("Second")
	err = memory.Write(ctx, "conv1", msg2)
	require.NoError(t, err)

	messages, _ := memory.Read(ctx, "conv1")
	assert.Len(t, messages, 2)
}

// TestWrite_EmptyMessageList tests writing empty message list
func TestWrite_EmptyMessageList(t *testing.T) {
	ctx := context.Background()
	memory := NewInMemoryMemory()

	err := memory.Write(ctx, "conv1")
	require.NoError(t, err)

	messages, _ := memory.Read(ctx, "conv1")
	assert.Empty(t, messages)
}

// TestWrite_DifferentConversations tests isolation between conversations
func TestWrite_DifferentConversations(t *testing.T) {
	ctx := context.Background()
	memory := NewInMemoryMemory()

	msg1 := NewUserMessage("Conv A Message")
	msg2 := NewUserMessage("Conv B Message")

	memory.Write(ctx, "convA", msg1)
	memory.Write(ctx, "convB", msg2)

	messagesA, _ := memory.Read(ctx, "convA")
	messagesB, _ := memory.Read(ctx, "convB")

	assert.Len(t, messagesA, 1)
	assert.Len(t, messagesB, 1)
}

// TestWrite_DifferentMessageTypes tests writing various message types
func TestWrite_DifferentMessageTypes(t *testing.T) {
	ctx := context.Background()
	memory := NewInMemoryMemory()

	systemMsg := NewSystemMessage("System instruction")
	userMsg := NewUserMessage("User query")
	assistantMsg := NewAssistantMessage("Assistant response")

	err := memory.Write(ctx, "conv1", systemMsg, userMsg, assistantMsg)
	require.NoError(t, err)

	messages, _ := memory.Read(ctx, "conv1")
	assert.Len(t, messages, 3)
	assert.Equal(t, MessageTypeSystem, messages[0].Type())
	assert.Equal(t, MessageTypeUser, messages[1].Type())
	assert.Equal(t, MessageTypeAssistant, messages[2].Type())
}

// TestWrite_AssistantMessageWithToolCalls tests assistant message with tool calls
func TestWrite_AssistantMessageWithToolCalls(t *testing.T) {
	ctx := context.Background()
	memory := NewInMemoryMemory()

	toolCall := &ToolCall{
		ID:        "call_123",
		Name:      "search",
		Arguments: `{"query": "weather"}`,
	}

	msg := NewAssistantMessage([]*ToolCall{toolCall})
	err := memory.Write(ctx, "conv1", msg)
	require.NoError(t, err)

	messages, _ := memory.Read(ctx, "conv1")
	assert.Len(t, messages, 1)
	assert.Equal(t, MessageTypeAssistant, messages[0].Type())
}

// TestWrite_ToolMessage tests tool message
func TestWrite_ToolMessage(t *testing.T) {
	ctx := context.Background()
	memory := NewInMemoryMemory()

	toolReturn := &ToolReturn{
		ID:     "call_123",
		Name:   "search",
		Result: `{"temperature": 25}`,
	}

	msg, err := NewToolMessage([]*ToolReturn{toolReturn})
	require.NoError(t, err)

	err = memory.Write(ctx, "conv1", msg)
	require.NoError(t, err)

	messages, _ := memory.Read(ctx, "conv1")
	assert.Len(t, messages, 1)
	assert.Equal(t, MessageTypeTool, messages[0].Type())
}

// TestWrite_MessageWithMetadata tests writing message with metadata
func TestWrite_MessageWithMetadata(t *testing.T) {
	ctx := context.Background()
	memory := NewInMemoryMemory()

	metadata := map[string]any{
		"source":    "mobile_app",
		"timestamp": time.Now().Unix(),
		"user_id":   "user123",
	}

	msg := NewUserMessage(MessageParams{
		Type:     MessageTypeUser,
		Text:     "Hello with metadata",
		Metadata: metadata,
	})

	err := memory.Write(ctx, "conv1", msg)
	require.NoError(t, err)

	messages, _ := memory.Read(ctx, "conv1")
	assert.Len(t, messages, 1)
}

// TestWrite_CompleteConversationFlow tests a complete conversation flow
func TestWrite_CompleteConversationFlow(t *testing.T) {
	ctx := context.Background()
	memory := NewInMemoryMemory()

	// System message
	systemMsg := NewSystemMessage("You are a helpful assistant")
	memory.Write(ctx, "conv1", systemMsg)

	// User message
	userMsg1 := NewUserMessage("What's the weather?")
	memory.Write(ctx, "conv1", userMsg1)

	// Assistant message with tool call
	toolCall := &ToolCall{
		ID:        "call_123",
		Name:      "get_weather",
		Arguments: `{"location": "New York"}`,
	}
	assistantMsg1 := NewAssistantMessage([]*ToolCall{toolCall})
	memory.Write(ctx, "conv1", assistantMsg1)

	// Tool response
	toolReturn := &ToolReturn{
		ID:     "call_123",
		Name:   "get_weather",
		Result: `{"temperature": 72, "condition": "sunny"}`,
	}
	toolMsg, _ := NewToolMessage([]*ToolReturn{toolReturn})
	memory.Write(ctx, "conv1", toolMsg)

	// Assistant final response
	assistantMsg2 := NewAssistantMessage("The weather in New York is sunny with 72°F")
	memory.Write(ctx, "conv1", assistantMsg2)

	messages, _ := memory.Read(ctx, "conv1")
	assert.Len(t, messages, 5)
	assert.Equal(t, MessageTypeSystem, messages[0].Type())
	assert.Equal(t, MessageTypeUser, messages[1].Type())
	assert.Equal(t, MessageTypeAssistant, messages[2].Type())
	assert.Equal(t, MessageTypeTool, messages[3].Type())
	assert.Equal(t, MessageTypeAssistant, messages[4].Type())
}

// TestRead_NonExistentConversation tests reading non-existent conversation
func TestRead_NonExistentConversation(t *testing.T) {
	ctx := context.Background()
	memory := NewInMemoryMemory()

	messages, err := memory.Read(ctx, "nonexistent")

	require.NoError(t, err)
	assert.Empty(t, messages)
	assert.NotNil(t, messages)
}

// TestRead_ExistingConversation tests reading existing conversation
func TestRead_ExistingConversation(t *testing.T) {
	ctx := context.Background()
	memory := NewInMemoryMemory()

	msg := NewUserMessage("Test")
	memory.Write(ctx, "conv1", msg)

	messages, err := memory.Read(ctx, "conv1")

	require.NoError(t, err)
	assert.Len(t, messages, 1)
}

// TestRead_ReturnsImmutableCopy tests that returned slice is a copy
func TestRead_ReturnsImmutableCopy(t *testing.T) {
	ctx := context.Background()
	memory := NewInMemoryMemory()

	msg1 := NewUserMessage("Original")
	memory.Write(ctx, "conv1", msg1)

	messages, _ := memory.Read(ctx, "conv1")
	originalLen := len(messages)

	// Attempt to modify returned slice
	messages = append(messages, NewUserMessage("Added"))

	// Read again and verify original data is unchanged
	messagesAgain, _ := memory.Read(ctx, "conv1")
	assert.Len(t, messagesAgain, originalLen)
}

// TestRead_AfterMultipleWrites tests reading after multiple write operations
func TestRead_AfterMultipleWrites(t *testing.T) {
	ctx := context.Background()
	memory := NewInMemoryMemory()

	for i := 0; i < 5; i++ {
		msg := NewUserMessage("Message")
		memory.Write(ctx, "conv1", msg)
	}

	messages, err := memory.Read(ctx, "conv1")
	require.NoError(t, err)
	assert.Len(t, messages, 5)
}

// TestRead_MessageOrder tests that messages maintain insertion order
func TestRead_MessageOrder(t *testing.T) {
	ctx := context.Background()
	memory := NewInMemoryMemory()

	msg1 := NewUserMessage("First")
	msg2 := NewAssistantMessage("Second")
	msg3 := NewUserMessage("Third")

	memory.Write(ctx, "conv1", msg1, msg2, msg3)

	messages, _ := memory.Read(ctx, "conv1")
	assert.Equal(t, MessageTypeUser, messages[0].Type())
	assert.Equal(t, MessageTypeAssistant, messages[1].Type())
	assert.Equal(t, MessageTypeUser, messages[2].Type())
}

// TestClear_ExistingConversation tests clearing existing conversation
func TestClear_ExistingConversation(t *testing.T) {
	ctx := context.Background()
	memory := NewInMemoryMemory()

	msg := NewUserMessage("To be cleared")
	memory.Write(ctx, "conv1", msg)

	err := memory.Clear(ctx, "conv1")
	require.NoError(t, err)

	messages, _ := memory.Read(ctx, "conv1")
	assert.Empty(t, messages)
}

// TestClear_NonExistentConversation tests clearing non-existent conversation
func TestClear_NonExistentConversation(t *testing.T) {
	ctx := context.Background()
	memory := NewInMemoryMemory()

	err := memory.Clear(ctx, "nonexistent")
	require.NoError(t, err)
}

// TestClear_AndRewrite tests writing after clearing
func TestClear_AndRewrite(t *testing.T) {
	ctx := context.Background()
	memory := NewInMemoryMemory()

	msg1 := NewUserMessage("First")
	memory.Write(ctx, "conv1", msg1)
	memory.Clear(ctx, "conv1")

	msg2 := NewUserMessage("Second")
	memory.Write(ctx, "conv1", msg2)

	messages, _ := memory.Read(ctx, "conv1")
	assert.Len(t, messages, 1)
}

// TestClear_IsolationBetweenConversations tests that clearing one conversation doesn't affect others
func TestClear_IsolationBetweenConversations(t *testing.T) {
	ctx := context.Background()
	memory := NewInMemoryMemory()

	memory.Write(ctx, "convA", NewUserMessage("A"))
	memory.Write(ctx, "convB", NewUserMessage("B"))

	memory.Clear(ctx, "convA")

	messagesA, _ := memory.Read(ctx, "convA")
	messagesB, _ := memory.Read(ctx, "convB")

	assert.Empty(t, messagesA)
	assert.Len(t, messagesB, 1)
}

// TestConcurrentWrites tests concurrent write operations
func TestConcurrentWrites(t *testing.T) {
	ctx := context.Background()
	memory := NewInMemoryMemory()
	conversationID := "concurrent-test"

	var wg sync.WaitGroup
	goroutines := 100
	messagesPerGoroutine := 10

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				msg := NewUserMessage("concurrent message")
				err := memory.Write(ctx, conversationID, msg)
				assert.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()

	messages, _ := memory.Read(ctx, conversationID)
	expectedCount := goroutines * messagesPerGoroutine
	assert.Equal(t, expectedCount, len(messages))
}

// TestConcurrentReadsAndWrites tests concurrent read and write operations
func TestConcurrentReadsAndWrites(t *testing.T) {
	ctx := context.Background()
	memory := NewInMemoryMemory()
	convID := "read-write-test"

	// Initialize with some data
	for i := 0; i < 10; i++ {
		memory.Write(ctx, convID, NewUserMessage("init"))
	}

	var wg sync.WaitGroup

	// Concurrent reads and writes
	for i := 0; i < 50; i++ {
		wg.Add(2)

		// Writer goroutine
		go func() {
			defer wg.Done()
			err := memory.Write(ctx, convID, NewUserMessage("writer"))
			assert.NoError(t, err)
		}()

		// Reader goroutine
		go func() {
			defer wg.Done()
			_, err := memory.Read(ctx, convID)
			assert.NoError(t, err)
		}()
	}

	wg.Wait()
}

// TestConcurrentClears tests concurrent clear operations
func TestConcurrentClears(t *testing.T) {
	ctx := context.Background()
	memory := NewInMemoryMemory()
	var wg sync.WaitGroup

	// Create multiple conversations
	conversations := 10
	for i := 0; i < conversations; i++ {
		convID := string(rune('A' + i))
		memory.Write(ctx, convID, NewUserMessage("test"))
	}

	// Concurrent clears
	for i := 0; i < conversations; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			convID := string(rune('A' + index))
			err := memory.Clear(ctx, convID)
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()

	// Verify all conversations are cleared
	for i := 0; i < conversations; i++ {
		convID := string(rune('A' + i))
		messages, _ := memory.Read(ctx, convID)
		assert.Empty(t, messages)
	}
}

// TestConcurrentOperationsOnDifferentConversations tests operations on different conversations simultaneously
func TestConcurrentOperationsOnDifferentConversations(t *testing.T) {
	ctx := context.Background()
	memory := NewInMemoryMemory()
	var wg sync.WaitGroup

	operations := 100

	for i := 0; i < operations; i++ {
		wg.Add(3)
		convID := string(rune('A' + (i % 26)))

		// Write
		go func(id string) {
			defer wg.Done()
			memory.Write(ctx, id, NewUserMessage("test"))
		}(convID)

		// Read
		go func(id string) {
			defer wg.Done()
			memory.Read(ctx, id)
		}(convID)

		// Clear (occasionally)
		go func(id string, index int) {
			defer wg.Done()
			if index%10 == 0 {
				memory.Clear(ctx, id)
			}
		}(convID, i)
	}

	wg.Wait()
}

// TestConcurrentWritesDifferentMessageTypes tests concurrent writes with different message types
func TestConcurrentWritesDifferentMessageTypes(t *testing.T) {
	ctx := context.Background()
	memory := NewInMemoryMemory()
	var wg sync.WaitGroup

	iterations := 50

	for i := 0; i < iterations; i++ {
		wg.Add(4)

		go func() {
			defer wg.Done()
			memory.Write(ctx, "conv1", NewSystemMessage("system"))
		}()

		go func() {
			defer wg.Done()
			memory.Write(ctx, "conv1", NewUserMessage("user"))
		}()

		go func() {
			defer wg.Done()
			memory.Write(ctx, "conv1", NewAssistantMessage("assistant"))
		}()

		go func() {
			defer wg.Done()
			toolReturn := &ToolReturn{
				ID:     "call_123",
				Name:   "test",
				Result: "result",
			}
			msg, _ := NewToolMessage([]*ToolReturn{toolReturn})
			memory.Write(ctx, "conv1", msg)
		}()
	}

	wg.Wait()

	messages, _ := memory.Read(ctx, "conv1")
	assert.Equal(t, iterations*4, len(messages))
}

// TestMemoryLeakPrevention tests basic memory management
func TestMemoryLeakPrevention(t *testing.T) {
	ctx := context.Background()
	memory := NewInMemoryMemory()

	// Create many conversations
	for i := 0; i < 1000; i++ {
		convID := string(rune(i))
		memory.Write(ctx, convID, NewUserMessage("test"))
	}

	// Clear all
	for i := 0; i < 1000; i++ {
		convID := string(rune(i))
		memory.Clear(ctx, convID)
	}

	// Verify map is cleaned
	assert.Equal(t, 0, len(memory.conversationMessages))
}

// TestContextWithTimeout tests behavior with timeout context
func TestContextWithTimeout(t *testing.T) {
	memory := NewInMemoryMemory()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	msg := NewUserMessage("test")
	err := memory.Write(ctx, "conv1", msg)

	// Current implementation doesn't check context
	assert.NoError(t, err)
}

// TestContextCancellation tests behavior with cancelled context
func TestContextCancellation(t *testing.T) {
	memory := NewInMemoryMemory()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	msg := NewUserMessage("test")
	err := memory.Write(ctx, "conv1", msg)

	// Current implementation doesn't check context
	assert.Error(t, err)
}

// BenchmarkWrite benchmarks write performance
func BenchmarkWrite(b *testing.B) {
	ctx := context.Background()
	memory := NewInMemoryMemory()
	msg := NewUserMessage("benchmark")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		memory.Write(ctx, "bench", msg)
	}
}

// BenchmarkRead benchmarks read performance
func BenchmarkRead(b *testing.B) {
	ctx := context.Background()
	memory := NewInMemoryMemory()

	// Prepare data
	for i := 0; i < 100; i++ {
		memory.Write(ctx, "bench", NewUserMessage("test"))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		memory.Read(ctx, "bench")
	}
}

// BenchmarkConcurrentWrites benchmarks concurrent write performance
func BenchmarkConcurrentWrites(b *testing.B) {
	ctx := context.Background()
	memory := NewInMemoryMemory()
	msg := NewUserMessage("benchmark")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			memory.Write(ctx, "bench", msg)
		}
	})
}

// BenchmarkConcurrentReads benchmarks concurrent read performance
func BenchmarkConcurrentReads(b *testing.B) {
	ctx := context.Background()
	memory := NewInMemoryMemory()

	// Prepare data
	for i := 0; i < 100; i++ {
		memory.Write(ctx, "bench", NewUserMessage("test"))
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			memory.Read(ctx, "bench")
		}
	})
}
