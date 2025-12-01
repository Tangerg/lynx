package chat_test

import (
	"context"
	"os"
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
	defaultTimeout = 30 * time.Second
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

// newTestClient creates a chat client for testing
func newTestClient(t *testing.T) *chat.Client {
	t.Helper()
	chatModel := newTestChatModel(t)

	client, err := chat.NewClientWithModel(chatModel)
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

func TestClient_Call_Chat(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := newTestClient(t)
	ctx := newTestContext(t)

	text, _, err := client.
		Chat().
		Call().
		Text(ctx)

	require.NoError(t, err)
	assert.NotEmpty(t, text)
	t.Logf("Response: %s", text)
}

func TestClient_Call_ChatText(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := newTestClient(t)
	ctx := newTestContext(t)

	text, resp, err := client.
		ChatWithText("Hi! How are you!").
		Call().
		Text(ctx)

	require.NoError(t, err)
	assert.NotEmpty(t, text)
	assert.NotNil(t, resp)
	assert.NotNil(t, resp.Metadata)
	t.Logf("Response: %s", text)
	t.Logf("Tokens used: %d", resp.Metadata.Usage.TotalTokens())
}

func TestClient_Call_ChatText_List(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := newTestClient(t)
	ctx := newTestContext(t)

	colors, resp, err := client.
		ChatWithText("List five colors").
		Call().
		Structured(ctx, chat.ListParserAsAny())
	require.NoError(t, err)
	assert.NotNil(t, resp)
	colorList := colors.([]string)
	assert.NotEmpty(t, colorList)
	assert.LessOrEqual(t, len(colorList), 10, "should return reasonable number of colors")

	t.Logf("Colors returned: %d", len(colorList))
	for i, color := range colorList {
		t.Logf("%d: %s", i+1, color)
		assert.NotEmpty(t, color)
	}
}

func TestClient_Call_ChatText_Map(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := newTestClient(t)
	ctx := newTestContext(t)

	userInfo, resp, err := client.
		ChatWithText("Tom, 18 years old, Email is Tom@gmail.com. Please format this user's information as JSON").
		Call().
		Structured(ctx, chat.MapParserAsAny())

	require.NoError(t, err)
	assert.NotNil(t, resp)
	userInfoMap := userInfo.(map[string]any)
	assert.NotEmpty(t, userInfoMap)

	t.Log("User info map:")
	for key, value := range userInfoMap {
		t.Logf("  %s: %v", key, value)
		assert.NotNil(t, value)
	}
}

func TestClient_Call_ChatText_Structured(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	type User struct {
		Name  string `json:"name"`
		Age   int    `json:"age"`
		Email string `json:"email"`
	}

	client := newTestClient(t)
	ctx := newTestContext(t)

	userAny, resp, err := client.
		ChatWithText("Tom, 18 years old, Email is Tom@gmail.com. Please format this user's information as JSON").
		Call().
		Structured(ctx, chat.JSONParserAsAnyOf[*User]())

	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.NotNil(t, userAny)

	userInfo, ok := userAny.(*User)
	require.True(t, ok, "should return User type")
	assert.Equal(t, "Tom", userInfo.Name)
	assert.Equal(t, 18, userInfo.Age)
	assert.Equal(t, "Tom@gmail.com", userInfo.Email)

	t.Logf("User: %s, %d years old, %s", userInfo.Name, userInfo.Age, userInfo.Email)
}

func TestClient_Call_PromptTemplate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := newTestClient(t)
	ctx := newTestContext(t)

	text, resp, err := client.
		ChatWithPromptTemplate(
			chat.
				NewPromptTemplate().
				WithTemplate("Hi! My name is {{.name}}, how are you?").
				WithVariable("name", "Tom"),
		).
		Call().
		Text(ctx)

	require.NoError(t, err)
	assert.NotEmpty(t, text)
	assert.NotNil(t, resp)
	t.Logf("Response: %s", text)
}

func TestClient_Call_ChatRequest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := newTestClient(t)
	ctx := newTestContext(t)

	messages := []chat.Message{
		chat.NewSystemMessage("You are a helpful assistant."),
		chat.NewUserMessage("Hi! How are you!"),
	}
	chatRequest, err := chat.NewRequest(messages)
	require.NoError(t, err)

	text, resp, err := client.
		ChatWithRequest(chatRequest).
		Call().
		Text(ctx)

	require.NoError(t, err)
	assert.NotEmpty(t, text)
	assert.NotNil(t, resp)
	t.Logf("Response: %s", text)
}

func TestClient_Call_Tool(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := newTestClient(t)
	ctx := newTestContext(t)

	weatherTool := fakeweatherquery.NewFakeWeatherQuery(nil)
	require.NotNil(t, weatherTool)

	text, resp, err := client.
		ChatWithText("I want to inquire about the weather conditions in Beijing on May 1st, 2023").
		WithTools(weatherTool).
		Call().
		Text(ctx)

	require.NoError(t, err)
	assert.NotEmpty(t, text)
	assert.NotNil(t, resp)
	t.Logf("Response: %s", text)
}

func TestClient_Call_MultipleMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := newTestClient(t)
	ctx := newTestContext(t)

	text1, resp1, err := client.
		ChatWithText("What is 2+2?").
		Call().
		Text(ctx)

	require.NoError(t, err)
	assert.NotEmpty(t, text1)
	t.Logf("First response: %s", text1)

	// Continue conversation
	messages := []chat.Message{
		chat.NewUserMessage("What is 2+2?"),
		resp1.Result().AssistantMessage,
		chat.NewUserMessage("What about 3+3?"),
	}
	chatRequest, err := chat.NewRequest(messages)
	require.NoError(t, err)

	text2, resp2, err := client.
		ChatWithRequest(chatRequest).
		Call().
		Text(ctx)

	require.NoError(t, err)
	assert.NotEmpty(t, text2)
	assert.NotNil(t, resp2)
	t.Logf("Second response: %s", text2)
}

func TestClient_Stream_Chat(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := newTestClient(t)
	ctx := newTestContext(t)

	var chunks []string
	responseStream := client.
		Chat().
		Stream().
		Text(ctx)

	for text, err := range responseStream {
		require.NoError(t, err)
		chunks = append(chunks, text)
		t.Logf("Chunk: %s", text)
	}

	assert.NotEmpty(t, chunks, "should receive at least one chunk")
	t.Logf("Total chunks received: %d", len(chunks))
}

func TestClient_Stream_ChatText(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := newTestClient(t)
	ctx := newTestContext(t)

	var chunks []string
	responseStream := client.
		ChatWithText("Tell me a short joke").
		Stream().
		Text(ctx)

	for text, err := range responseStream {
		require.NoError(t, err)
		chunks = append(chunks, text)
		t.Logf("Chunk: %s", text)
	}

	assert.NotEmpty(t, chunks)
	t.Logf("Total chunks: %d", len(chunks))
}

func TestClient_Stream_PromptTemplate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := newTestClient(t)
	ctx := newTestContext(t)

	var chunks []string
	responseStream := client.
		ChatWithPromptTemplate(
			chat.
				NewPromptTemplate().
				WithTemplate("Hi! My name is {{.name}}, tell me a short fact").
				WithVariable("name", "Tom"),
		).
		Stream().
		Text(ctx)

	for text, err := range responseStream {
		require.NoError(t, err)
		chunks = append(chunks, text)
		t.Logf("Chunk: %s", text)
	}

	assert.NotEmpty(t, chunks)
}

func TestClient_Stream_ChatRequest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := newTestClient(t)
	ctx := newTestContext(t)

	messages := []chat.Message{
		chat.NewSystemMessage("You are a helpful assistant."),
		chat.NewUserMessage("Count from 1 to 5"),
	}
	chatRequest, err := chat.NewRequest(messages)
	require.NoError(t, err)

	var chunks []string
	responseStream := client.
		ChatWithRequest(chatRequest).
		Stream().
		Text(ctx)

	for text, err := range responseStream {
		require.NoError(t, err)
		chunks = append(chunks, text)
		t.Logf("Chunk: %s", text)
	}

	assert.NotEmpty(t, chunks)
}

func TestClient_Stream_Tool(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := newTestClient(t)
	ctx := newTestContext(t)

	weatherTool := fakeweatherquery.NewFakeWeatherQuery(nil)
	require.NotNil(t, weatherTool)

	var chunks []string
	responseStream := client.
		ChatWithText("What's the weather in Beijing on May 1st, 2023?").
		WithTools(weatherTool).
		Stream().
		Text(ctx)

	for text, err := range responseStream {
		require.NoError(t, err)
		chunks = append(chunks, text)
		t.Logf("Chunk: %s", text)
	}

	assert.NotEmpty(t, chunks)
}

func TestClient_ErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := newTestClient(t)

	t.Run("context timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		_, _, err := client.
			ChatWithText("Tell me a long story").
			Call().
			Text(ctx)

		assert.Error(t, err)
		t.Logf("Expected timeout error: %v", err)
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, _, err := client.
			ChatWithText("Hello").
			Call().
			Text(ctx)

		assert.Error(t, err)
		t.Logf("Expected cancellation error: %v", err)
	})
}

func TestClient_WithOptions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := newTestClient(t)
	ctx := newTestContext(t)

	config := newTestConfig()
	opts, err := chat.NewOptions(config.model)
	require.NoError(t, err)

	temp := 0.7
	maxTokens := int64(100)
	opts.Temperature = &temp
	opts.MaxTokens = &maxTokens

	text, resp, err := client.
		ChatWithText("Say hello").
		WithOptions(opts).
		Call().
		Text(ctx)

	require.NoError(t, err)
	assert.NotEmpty(t, text)
	assert.NotNil(t, resp)
	t.Logf("Response: %s", text)
}

// Benchmark tests for performance measurement
func BenchmarkClient_Call_Simple(b *testing.B) {
	config := newTestConfig()
	if config.apiKey == "" {
		b.Skipf("Skipping benchmark: %s not set", apiKeyEnvVar)
	}

	client := newTestClient(&testing.T{})
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := client.
			ChatWithText("Hi").
			Call().
			Text(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}
