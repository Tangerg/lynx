package memory

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/extensions/models/openai"
	"github.com/Tangerg/lynx/ai/extensions/tools/fakeweatherquery"
	"github.com/Tangerg/lynx/ai/model"
	"github.com/Tangerg/lynx/ai/model/chat"
)

const (
	defaultBaseURL = "https://api.siliconflow.cn/v1"
	defaultModel   = "Qwen/Qwen2.5-7B-Instruct"
	apiKeyEnvVar   = "API_KEY"
	defaultTimeout = 300 * time.Second

	testConversationID = "test-conversation-123"
)

// testConfig holds configuration for integration tests
type testConfig struct {
	baseURL string
	model   string
	apiKey  string
	timeout time.Duration
}

// newTestConfig creates a test configuration from environment variables
func newTestConfig() *testConfig {
	return &testConfig{
		baseURL: getEnvOrDefault("TEST_BASE_URL", defaultBaseURL),
		model:   getEnvOrDefault("TEST_MODEL", defaultModel),
		apiKey:  getEnvOrDefault(apiKeyEnvVar, ""),
		timeout: defaultTimeout,
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// skipIfNoAPIKey skips the test if API key is not configured
func skipIfNoAPIKey(t *testing.T, config *testConfig) {
	if config.apiKey == "" {
		t.Skipf("Skipping integration test: %s not set", apiKeyEnvVar)
	}
}

// newTestChatModel creates a chat model for testing
func newTestChatModel(t *testing.T) *openai.ChatModel {
	t.Helper()
	config := newTestConfig()
	skipIfNoAPIKey(t, config)

	defaultOptions, err := chat.NewOptions(config.model)
	require.NoError(t, err)

	chatModel, err := openai.NewChatModel(
		model.NewApiKey(config.apiKey),
		defaultOptions,
		option.WithBaseURL(config.baseURL),
	)
	require.NoError(t, err)

	return chatModel
}

// newTestClientWithMemory creates a chat client with memory middleware
func newTestClientWithMemory(t *testing.T, storage Store) *chat.Client {
	t.Helper()
	chatModel := newTestChatModel(t)

	callMW, streamMW, err := NewMemoryMiddleware(storage)
	require.NoError(t, err)

	clientReq, err := chat.NewClientRequest(chatModel)
	require.NoError(t, err)

	clientReq.WithMiddlewares(callMW, streamMW)

	client, err := chat.NewClient(clientReq)
	require.NoError(t, err)

	return client
}

// newTestContext creates a context with timeout for testing
func newTestContext(t *testing.T) context.Context {
	t.Helper()
	config := newTestConfig()
	ctx, cancel := context.WithTimeout(context.Background(), config.timeout)
	t.Cleanup(cancel)
	return ctx
}

// Tests for NewMemoryMiddleware

func TestNewMemoryMiddleware_Success(t *testing.T) {
	storage := NewInMemoryMemory()

	callMW, streamMW, err := NewMemoryMiddleware(storage)

	require.NoError(t, err)
	assert.NotNil(t, callMW)
	assert.NotNil(t, streamMW)
}

func TestNewMemoryMiddleware_NilStorage(t *testing.T) {
	callMW, streamMW, err := NewMemoryMiddleware(nil)

	assert.Error(t, err)
	assert.Nil(t, callMW)
	assert.Nil(t, streamMW)
	assert.Contains(t, err.Error(), "memory store is required")
}

// Tests for extractConversationID

func TestExtractConversationID_Success(t *testing.T) {
	storage := NewInMemoryMemory()
	mw := &chatMemoryMiddleware{store: storage}

	req, err := chat.NewRequest([]chat.Message{
		chat.NewUserMessage("Hello"),
	})
	require.NoError(t, err)

	req.Set(ConversationIDKey, testConversationID)

	conversationID, err := mw.extractConversationID(req)

	require.NoError(t, err)
	assert.Equal(t, testConversationID, conversationID)
}

func TestExtractConversationID_NotExists(t *testing.T) {
	storage := NewInMemoryMemory()
	mw := &chatMemoryMiddleware{store: storage}

	req, err := chat.NewRequest([]chat.Message{
		chat.NewUserMessage("Hello"),
	})
	require.NoError(t, err)

	conversationID, err := mw.extractConversationID(req)

	require.NoError(t, err)
	assert.Empty(t, conversationID)
}

func TestExtractConversationID_InvalidType(t *testing.T) {
	storage := NewInMemoryMemory()
	mw := &chatMemoryMiddleware{store: storage}

	req, err := chat.NewRequest([]chat.Message{
		chat.NewUserMessage("Hello"),
	})
	require.NoError(t, err)

	req.Set(ConversationIDKey, 123) // Invalid type

	conversationID, err := mw.extractConversationID(req)

	assert.Error(t, err)
	assert.Empty(t, conversationID)
	assert.Contains(t, err.Error(), "conversation id must be a string")
}

// Tests for memory persistence - Unit tests

func TestPersistMessages_NoConversationID(t *testing.T) {
	storage := NewInMemoryMemory()
	mw := &chatMemoryMiddleware{store: storage}
	ctx := context.Background()

	req, err := chat.NewRequest([]chat.Message{
		chat.NewUserMessage("Hello"),
	})
	require.NoError(t, err)

	// Should not return error, just skip persistence
	err = mw.persistMessages(ctx, req, chat.NewUserMessage("Test"))
	assert.NoError(t, err)

	// Verify nothing was stored
	messages, err := storage.Read(ctx, testConversationID)
	require.NoError(t, err)
	assert.Empty(t, messages)
}

func TestPersistMessages_WithConversationID(t *testing.T) {
	storage := NewInMemoryMemory()
	mw := &chatMemoryMiddleware{store: storage}
	ctx := context.Background()

	req, err := chat.NewRequest([]chat.Message{
		chat.NewUserMessage("Hello"),
	})
	require.NoError(t, err)
	req.Set(ConversationIDKey, testConversationID)

	msg1 := chat.NewUserMessage("Message 1")
	msg2 := chat.NewUserMessage("Message 2")

	err = mw.persistMessages(ctx, req, msg1, msg2)
	require.NoError(t, err)

	// Verify messages were stored
	messages, err := storage.Read(ctx, testConversationID)
	require.NoError(t, err)
	assert.Len(t, messages, 2)
}

func TestRetrieveHistoryMessages_EmptyHistory(t *testing.T) {
	storage := NewInMemoryMemory()
	mw := &chatMemoryMiddleware{store: storage}
	ctx := context.Background()

	req, err := chat.NewRequest([]chat.Message{
		chat.NewUserMessage("Hello"),
	})
	require.NoError(t, err)
	req.Set(ConversationIDKey, testConversationID)

	messages, err := mw.retrieveHistoryMessages(ctx, req)

	require.NoError(t, err)
	assert.Empty(t, messages)
}

func TestRetrieveHistoryMessages_WithHistory(t *testing.T) {
	storage := NewInMemoryMemory()
	mw := &chatMemoryMiddleware{store: storage}
	ctx := context.Background()

	// Pre-populate store
	msg1 := chat.NewUserMessage("Previous message")
	msg2 := chat.NewAssistantMessage("Previous response")
	err := storage.Write(ctx, testConversationID, msg1, msg2)
	require.NoError(t, err)

	req, err := chat.NewRequest([]chat.Message{
		chat.NewUserMessage("Hello"),
	})
	require.NoError(t, err)
	req.Set(ConversationIDKey, testConversationID)

	messages, err := mw.retrieveHistoryMessages(ctx, req)

	require.NoError(t, err)
	assert.Len(t, messages, 2)
}

// Integration tests with real chat client

func TestMemoryMiddleware_Call_SingleTurn(t *testing.T) {
	storage := NewInMemoryMemory()
	client := newTestClientWithMemory(t, storage)
	ctx := newTestContext(t)

	// First turn
	text1, resp1, err := client.
		ChatWithText("What is 2+2?").
		WithParams(map[string]any{
			ConversationIDKey: testConversationID,
		}).
		Call().
		Text(ctx)

	require.NoError(t, err)
	require.NotNil(t, resp1)
	assert.NotEmpty(t, text1)
	t.Logf("Response 1: %s", text1)

	// Verify messages were stored
	messages, err := storage.Read(ctx, testConversationID)
	require.NoError(t, err)
	assert.Len(t, messages, 2) // User message + Assistant message
}

func TestMemoryMiddleware_Call_MultiTurn(t *testing.T) {
	storage := NewInMemoryMemory()
	client := newTestClientWithMemory(t, storage)
	ctx := newTestContext(t)

	// First turn
	text1, _, err := client.
		ChatWithText("My name is Alice").
		WithParams(map[string]any{
			ConversationIDKey: testConversationID,
		}).
		Call().
		Text(ctx)

	require.NoError(t, err)
	t.Logf("Turn 1: %s", text1)

	// Second turn - should remember name
	text2, _, err := client.
		ChatWithText("What is my name?").
		WithParams(map[string]any{
			ConversationIDKey: testConversationID,
		}).
		Call().
		Text(ctx)

	require.NoError(t, err)
	t.Logf("Turn 2: %s", text2)

	// The response should mention "Alice" (case-insensitive)
	assert.Contains(t, text2, "Alice")

	// Verify all messages were stored
	messages, err := storage.Read(ctx, testConversationID)
	require.NoError(t, err)
	assert.Len(t, messages, 4) // 2 user messages + 2 assistant messages
}

func TestMemoryMiddleware_Call_MultipleConversations(t *testing.T) {
	storage := NewInMemoryMemory()
	client := newTestClientWithMemory(t, storage)
	ctx := newTestContext(t)

	conversationID1 := "conversation-1"
	conversationID2 := "conversation-2"

	// Conversation 1
	_, _, err := client.
		ChatWithText("My favorite color is blue").
		WithParams(map[string]any{
			ConversationIDKey: conversationID1,
		}).
		Call().
		Text(ctx)
	require.NoError(t, err)

	// Conversation 2
	_, _, err = client.
		ChatWithText("My favorite color is red").
		WithParams(map[string]any{
			ConversationIDKey: conversationID2,
		}).
		Call().
		Text(ctx)
	require.NoError(t, err)

	// Ask about color in conversation 1
	text1, _, err := client.
		ChatWithText("What is my favorite color?").
		WithParams(map[string]any{
			ConversationIDKey: conversationID1,
		}).
		Call().
		Text(ctx)
	require.NoError(t, err)
	t.Logf("Conversation 1 response: %s", text1)

	// Ask about color in conversation 2
	text2, _, err := client.
		ChatWithText("What is my favorite color?").
		WithParams(map[string]any{
			ConversationIDKey: conversationID2,
		}).
		Call().
		Text(ctx)
	require.NoError(t, err)
	t.Logf("Conversation 2 response: %s", text2)

	// Verify separate conversation histories
	messages1, err := storage.Read(ctx, conversationID1)
	require.NoError(t, err)
	assert.Len(t, messages1, 4)

	messages2, err := storage.Read(ctx, conversationID2)
	require.NoError(t, err)
	assert.Len(t, messages2, 4)
}

func TestMemoryMiddleware_Stream_SingleTurn(t *testing.T) {
	storage := NewInMemoryMemory()
	client := newTestClientWithMemory(t, storage)
	ctx := newTestContext(t)

	var fullText string
	for text, err := range client.
		ChatWithText("Count from 1 to 5").
		WithParams(map[string]any{
			ConversationIDKey: testConversationID,
		}).
		Stream().
		Text(ctx) {

		require.NoError(t, err)
		fullText += text
		t.Logf("Chunk: %s", text)
	}

	assert.NotEmpty(t, fullText)
	t.Logf("Full response: %s", fullText)

	// Verify messages were stored after streaming completed
	messages, err := storage.Read(ctx, testConversationID)
	require.NoError(t, err)
	assert.Len(t, messages, 2) // User message + Assistant message
}

func TestMemoryMiddleware_Stream_MultiTurn(t *testing.T) {
	storage := NewInMemoryMemory()
	client := newTestClientWithMemory(t, storage)
	ctx := newTestContext(t)

	// First turn with streaming
	var text1 string
	for chunk, err := range client.
		ChatWithText("My hobby is painting").
		WithParams(map[string]any{
			ConversationIDKey: testConversationID,
		}).
		Stream().
		Text(ctx) {

		require.NoError(t, err)
		text1 += chunk
	}
	t.Logf("Turn 1 (streaming): %s", text1)

	// Second turn - should remember hobby
	var text2 string
	for chunk, err := range client.
		ChatWithText("What is my hobby?").
		WithParams(map[string]any{
			ConversationIDKey: testConversationID,
		}).
		Stream().
		Text(ctx) {

		require.NoError(t, err)
		text2 += chunk
	}
	t.Logf("Turn 2 (streaming): %s", text2)

	// Verify memory retention
	assert.Contains(t, text2, "painting")

	// Verify all messages were stored
	messages, err := storage.Read(ctx, testConversationID)
	require.NoError(t, err)
	assert.Len(t, messages, 4)
}

func TestMemoryMiddleware_NoConversationID(t *testing.T) {
	storage := NewInMemoryMemory()
	client := newTestClientWithMemory(t, storage)
	ctx := newTestContext(t)

	// First turn without conversation ID
	text1, _, err := client.
		ChatWithText("Hello").
		Call().
		Text(ctx)

	require.NoError(t, err)
	assert.NotEmpty(t, text1)

	// Second turn - should not remember previous message
	text2, _, err := client.
		ChatWithText("What did I just say?").
		Call().
		Text(ctx)

	require.NoError(t, err)
	assert.NotEmpty(t, text2)

	// Verify no messages were stored (empty conversation ID)
	messages, err := storage.Read(ctx, testConversationID)
	require.NoError(t, err)
	assert.Empty(t, messages)
}

func TestMemoryMiddleware_ClearConversation(t *testing.T) {
	storage := NewInMemoryMemory()
	client := newTestClientWithMemory(t, storage)
	ctx := newTestContext(t)

	// Create a conversation
	_, _, err := client.
		ChatWithText("Remember this: XYZ123").
		WithParams(map[string]any{
			ConversationIDKey: testConversationID,
		}).
		Call().
		Text(ctx)
	require.NoError(t, err)

	// Verify messages exist
	messages, err := storage.Read(ctx, testConversationID)
	require.NoError(t, err)
	assert.NotEmpty(t, messages)

	// Clear the conversation
	err = storage.Clear(ctx, testConversationID)
	require.NoError(t, err)

	// Verify messages were cleared
	messages, err = storage.Read(ctx, testConversationID)
	require.NoError(t, err)
	assert.Empty(t, messages)

	// New conversation should not remember cleared data
	text, _, err := client.
		ChatWithText("What did I ask you to remember?").
		WithParams(map[string]any{
			ConversationIDKey: testConversationID,
		}).
		Call().
		Text(ctx)

	require.NoError(t, err)
	// Should not contain the cleared information
	assert.NotContains(t, text, "XYZ123")
}

func TestMemoryMiddleware_Call_WithToolCall(t *testing.T) {
	storage := NewInMemoryMemory()
	client := newTestClientWithMemory(t, storage)
	ctx := newTestContext(t)

	weatherTool := fakeweatherquery.NewFakeWeatherQuery(nil)
	require.NotNil(t, weatherTool)

	// First turn - tool call
	text1, resp1, err := client.
		ChatWithText("What's the weather in Beijing on May 1st, 2023?").
		WithTools(weatherTool).
		WithParams(map[string]any{
			ConversationIDKey: testConversationID,
		}).
		Call().
		Text(ctx)

	require.NoError(t, err)
	require.NotNil(t, resp1)
	assert.NotEmpty(t, text1)
	t.Logf("Turn 1 (with tool): %s", text1)

	// Verify messages were stored including tool calls and tool returns
	messages, err := storage.Read(ctx, testConversationID)
	require.NoError(t, err)

	//// Should have: User message -> Assistant message (with tool call) -> Tool message (with returns) -> Assistant message (final)
	//assert.Greater(t, len(messages), 2, "should store user, assistant, and tool messages")

	// Check message types
	var hasUserMsg, hasAssistantMsg, hasToolMsg bool
	for _, msg := range messages {
		switch msg.Type() {
		case chat.MessageTypeUser:
			hasUserMsg = true
		case chat.MessageTypeAssistant:
			hasAssistantMsg = true
		case chat.MessageTypeTool:
			hasToolMsg = true
		}
	}
	assert.True(t, hasUserMsg, "should have user message")
	assert.True(t, hasAssistantMsg, "should have assistant message")
	// Tool message is optional depending on whether model uses tools
	t.Logf("Has tool message: %v", hasToolMsg)

	// Second turn - should remember the weather query context
	text2, _, err := client.
		ChatWithText("Is it suitable for outdoor activities?").
		WithParams(map[string]any{
			ConversationIDKey: testConversationID,
		}).
		Call().
		Text(ctx)

	require.NoError(t, err)
	assert.NotEmpty(t, text2)
	t.Logf("Turn 2 (context-aware): %s", text2)

	// The response should be aware of the previous weather query
	// (though exact content depends on the model)

	// Verify conversation history grew
	messages, err = storage.Read(ctx, testConversationID)
	require.NoError(t, err)
	assert.Greater(t, len(messages), 5, "conversation should have grown")
}

func TestMemoryMiddleware_Call_MultipleToolCalls(t *testing.T) {
	storage := NewInMemoryMemory()
	client := newTestClientWithMemory(t, storage)
	ctx := newTestContext(t)

	weatherTool := fakeweatherquery.NewFakeWeatherQuery(nil)
	require.NotNil(t, weatherTool)

	// First tool call - Beijing weather
	text1, _, err := client.
		ChatWithText("What's the weather in Beijing on May 1st, 2023?").
		WithTools(weatherTool).
		WithParams(map[string]any{
			ConversationIDKey: testConversationID,
		}).
		Call().
		Text(ctx)

	require.NoError(t, err)
	t.Logf("Beijing weather: %s", text1)

	// Second tool call - Shanghai weather
	text2, _, err := client.
		ChatWithText("And what about Shanghai on the same day?").
		WithTools(weatherTool).
		WithParams(map[string]any{
			ConversationIDKey: testConversationID,
		}).
		Call().
		Text(ctx)

	require.NoError(t, err)
	t.Logf("Shanghai weather: %s", text2)

	// Third turn - compare the two
	text3, _, err := client.
		ChatWithText("Which city has better weather for traveling?").
		WithParams(map[string]any{
			ConversationIDKey: testConversationID,
		}).
		Call().
		Text(ctx)

	require.NoError(t, err)
	assert.NotEmpty(t, text3)
	t.Logf("Comparison: %s", text3)

	// Should mention both cities in the comparison
	// (case-insensitive check)
	lowerText := strings.ToLower(text3)
	assert.True(t,
		strings.Contains(lowerText, "beijing") || strings.Contains(lowerText, "shanghai"),
		"response should mention at least one of the cities")

	// Verify all messages including multiple tool interactions were stored
	messages, err := storage.Read(ctx, testConversationID)
	require.NoError(t, err)
	assert.Greater(t, len(messages), 5, "should have multiple turns with tool calls")
}

func TestMemoryMiddleware_Stream_WithToolCall(t *testing.T) {
	storage := NewInMemoryMemory()
	client := newTestClientWithMemory(t, storage)
	ctx := newTestContext(t)

	weatherTool := fakeweatherquery.NewFakeWeatherQuery(nil)
	require.NotNil(t, weatherTool)

	// First turn - streaming with tool call
	var text1 string
	for chunk, err := range client.
		ChatWithText("Check the weather in Tokyo on June 15th, 2023").
		WithTools(weatherTool).
		WithParams(map[string]any{
			ConversationIDKey: testConversationID,
		}).
		Stream().
		Text(ctx) {

		require.NoError(t, err)
		text1 += chunk
		t.Logf("Chunk: %s", chunk)
	}

	assert.NotEmpty(t, text1)
	t.Logf("Full streaming response with tool: %s", text1)

	// Verify tool call results were stored
	messages, err := storage.Read(ctx, testConversationID)
	require.NoError(t, err)
	assert.Greater(t, len(messages), 1, "should store messages from streaming with tools")

	// Second turn - should remember the tool call context
	var text2 string
	for chunk, err := range client.
		ChatWithText("What about the next day?").
		WithTools(weatherTool).
		WithParams(map[string]any{
			ConversationIDKey: testConversationID,
		}).
		Stream().
		Text(ctx) {

		require.NoError(t, err)
		text2 += chunk
	}

	assert.NotEmpty(t, text2)
	t.Logf("Follow-up streaming response: %s", text2)

	// Verify conversation grew
	messages, err = storage.Read(ctx, testConversationID)
	require.NoError(t, err)
	assert.Greater(t, len(messages), 3, "conversation should include multiple tool interactions")
}

func TestMemoryMiddleware_ToolCallContextRetention(t *testing.T) {
	storage := NewInMemoryMemory()
	client := newTestClientWithMemory(t, storage)
	ctx := newTestContext(t)

	weatherTool := fakeweatherquery.NewFakeWeatherQuery(nil)
	require.NotNil(t, weatherTool)

	// Step 1: Ask about weather (with tool)
	text1, _, err := client.
		ChatWithText("What's the temperature in Paris on July 14th, 2023?").
		WithTools(weatherTool).
		WithParams(map[string]any{
			ConversationIDKey: testConversationID,
		}).
		Call().
		Text(ctx)

	require.NoError(t, err)
	t.Logf("Step 1 - Weather query: %s", text1)

	// Step 2: Ask follow-up without tool (should still have context)
	text2, _, err := client.
		ChatWithText("Is that temperature comfortable for walking?").
		WithParams(map[string]any{
			ConversationIDKey: testConversationID,
		}).
		Call().
		Text(ctx)

	require.NoError(t, err)
	t.Logf("Step 2 - Follow-up without tool: %s", text2)

	// Step 3: Another follow-up
	text3, _, err := client.
		ChatWithText("Should I bring an umbrella?").
		WithParams(map[string]any{
			ConversationIDKey: testConversationID,
		}).
		Call().
		Text(ctx)

	require.NoError(t, err)
	t.Logf("Step 3 - Another follow-up: %s", text3)

	// Verify the conversation maintained context across tool and non-tool calls
	messages, err := storage.Read(ctx, testConversationID)
	require.NoError(t, err)

	// Should have multiple turns stored
	assert.Greater(t, len(messages), 5, "should maintain full conversation history")

	// Count message types
	userCount := 0
	assistantCount := 0
	toolCount := 0

	for _, msg := range messages {
		switch msg.Type() {
		case chat.MessageTypeUser:
			userCount++
		case chat.MessageTypeAssistant:
			assistantCount++
		case chat.MessageTypeTool:
			toolCount++
		}
	}

	assert.Equal(t, 3, userCount, "should have 3 user messages")
	assert.GreaterOrEqual(t, assistantCount, 2, "should have at least 2 assistant messages")
	t.Logf("Message counts - User: %d, Assistant: %d, Tool: %d", userCount, assistantCount, toolCount)
}

func TestMemoryMiddleware_MixedToolAndNonToolConversation(t *testing.T) {
	storage := NewInMemoryMemory()
	client := newTestClientWithMemory(t, storage)
	ctx := newTestContext(t)

	weatherTool := fakeweatherquery.NewFakeWeatherQuery(nil)
	require.NotNil(t, weatherTool)

	// Turn 1: Normal conversation (no tool)
	_, _, err := client.
		ChatWithText("Hello, I'm planning a trip").
		WithParams(map[string]any{
			ConversationIDKey: testConversationID,
		}).
		Call().
		Text(ctx)
	require.NoError(t, err)

	// Turn 2: With tool
	text2, _, err := client.
		ChatWithText("What's the weather in London on August 20th, 2023?").
		WithTools(weatherTool).
		WithParams(map[string]any{
			ConversationIDKey: testConversationID,
		}).
		Call().
		Text(ctx)
	require.NoError(t, err)
	t.Logf("Weather response: %s", text2)

	// Turn 3: Follow-up without tool
	text3, _, err := client.
		ChatWithText("Based on that weather, what should I pack?").
		WithParams(map[string]any{
			ConversationIDKey: testConversationID,
		}).
		Call().
		Text(ctx)
	require.NoError(t, err)
	t.Logf("Packing advice: %s", text3)

	// Turn 4: Another tool call
	text4, _, err := client.
		ChatWithText("And what about Berlin on the same date?").
		WithTools(weatherTool).
		WithParams(map[string]any{
			ConversationIDKey: testConversationID,
		}).
		Call().
		Text(ctx)
	require.NoError(t, err)
	t.Logf("Berlin weather: %s", text4)

	// Turn 5: Conclusion without tool
	text5, _, err := client.
		ChatWithText("Which city would you recommend?").
		WithParams(map[string]any{
			ConversationIDKey: testConversationID,
		}).
		Call().
		Text(ctx)
	require.NoError(t, err)
	t.Logf("Recommendation: %s", text5)

	// Verify complete conversation history
	messages, err := storage.Read(ctx, testConversationID)
	require.NoError(t, err)

	// Should have all turns stored
	assert.Greater(t, len(messages), 8, "should store full mixed conversation")

	// Log all stored messages for debugging
	t.Log("Stored messages:")
	for i, msg := range messages {
		t.Logf("  [%d] Type: %s", i, msg.Type())
	}
}

func TestMemoryMiddleware_ToolCallAcrossConversations(t *testing.T) {
	storage := NewInMemoryMemory()
	client := newTestClientWithMemory(t, storage)
	ctx := newTestContext(t)

	weatherTool := fakeweatherquery.NewFakeWeatherQuery(nil)
	require.NotNil(t, weatherTool)

	conversationID1 := "trip-planning-1"
	conversationID2 := "trip-planning-2"

	// Conversation 1: Check weather for vacation
	text1a, _, err := client.
		ChatWithText("What's the weather in Hawaii on December 25th, 2023?").
		WithTools(weatherTool).
		WithParams(map[string]any{
			ConversationIDKey: conversationID1,
		}).
		Call().
		Text(ctx)
	require.NoError(t, err)
	t.Logf("Conversation 1 - Hawaii: %s", text1a)

	// Conversation 2: Check weather for business trip
	text2a, _, err := client.
		ChatWithText("What's the weather in New York on December 25th, 2023?").
		WithTools(weatherTool).
		WithParams(map[string]any{
			ConversationIDKey: conversationID2,
		}).
		Call().
		Text(ctx)
	require.NoError(t, err)
	t.Logf("Conversation 2 - New York: %s", text2a)

	// Continue conversation 1 - should only know about Hawaii
	text1b, _, err := client.
		ChatWithText("Is it good for beach activities?").
		WithParams(map[string]any{
			ConversationIDKey: conversationID1,
		}).
		Call().
		Text(ctx)
	require.NoError(t, err)
	t.Logf("Conversation 1 follow-up: %s", text1b)

	// Continue conversation 2 - should only know about New York
	text2b, _, err := client.
		ChatWithText("Should I bring a heavy coat?").
		WithParams(map[string]any{
			ConversationIDKey: conversationID2,
		}).
		Call().
		Text(ctx)
	require.NoError(t, err)
	t.Logf("Conversation 2 follow-up: %s", text2b)

	// Verify separate conversation contexts
	messages1, err := storage.Read(ctx, conversationID1)
	require.NoError(t, err)

	messages2, err := storage.Read(ctx, conversationID2)
	require.NoError(t, err)

	assert.Greater(t, len(messages1), 2, "conversation 1 should have multiple messages")
	assert.Greater(t, len(messages2), 2, "conversation 2 should have multiple messages")

	t.Logf("Conversation 1 message count: %d", len(messages1))
	t.Logf("Conversation 2 message count: %d", len(messages2))
}
