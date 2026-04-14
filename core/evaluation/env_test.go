package evaluation

import (
	"os"
	"testing"
	"time"

	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/extensions/models/openai"
	"github.com/Tangerg/lynx/ai/model"
	"github.com/Tangerg/lynx/ai/model/chat"
)

const (
	defaultBaseURL = "https://api.siliconflow.cn/v1"
	defaultModel   = "Qwen/Qwen2.5-7B-Instruct"
	apiKeyEnvVar   = "API_KEY"
	defaultTimeout = 30 * time.Second
)

type testConfig struct {
	baseURL string
	model   string
	apiKey  string
	timeout time.Duration
}

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

func skipIfNoAPIKey(t *testing.T, config *testConfig) {
	if config.apiKey == "" {
		t.Skipf("Skipping integration test: %s not set", apiKeyEnvVar)
	}
}

func newTestChatModel(t *testing.T) *openai.ChatModel {
	t.Helper()
	config := newTestConfig()
	skipIfNoAPIKey(t, config)

	defaultOptions, err := chat.NewOptions(config.model)
	require.NoError(t, err)

	chatModel, err := openai.NewChatModel(
		&openai.ChatModelConfig{
			ApiKey:         model.NewApiKey(config.apiKey),
			DefaultOptions: defaultOptions,
			RequestOptions: []option.RequestOption{
				option.WithBaseURL(config.baseURL),
			},
		},
	)
	require.NoError(t, err)

	return chatModel
}
