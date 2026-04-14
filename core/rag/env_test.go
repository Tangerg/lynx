package rag

import (
	"os"
	"testing"
	"time"

	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/extensions/models/openai"
	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/model"
	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
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

func createDoc(t *testing.T, id, text string, score float64) *document.Document {
	doc, err := document.NewDocument(text, nil)
	require.NoError(t, err)
	doc.ID = id
	doc.Score = score
	return doc
}

func createMockDocuments(count int) []*document.Document {
	docs := make([]*document.Document, count)
	for i := 0; i < count; i++ {
		docs[i], _ = document.NewDocument("mock document", nil)
	}
	return docs
}

func createDocWithMetadata(t *testing.T, id, text string, score float64, metadata map[string]any) *document.Document {
	doc := createDoc(t, id, text, score)
	doc.Metadata = metadata
	return doc
}

func mustCreateQueryWithHistory(t *testing.T, text string, history []chat.Message) *Query {
	query := mustCreateQuery(t, text)
	query.Set(ChatHistoryKey, history)
	return query
}

func mustCreateQuery(t *testing.T, text string) *Query {
	query, err := NewQuery(text)
	require.NoError(t, err)
	return query
}

func mustCreateQueryWithHistoryAndMetadata(t *testing.T, text string, history []chat.Message, metadata map[string]any) *Query {
	query := mustCreateQueryWithHistory(t, text, history)
	for k, v := range metadata {
		query.Set(k, v)
	}
	return query
}

func mustCreateQueryWithMetadata(t *testing.T, text string, metadata map[string]any) *Query {
	query := mustCreateQuery(t, text)
	for k, v := range metadata {
		query.Set(k, v)
	}
	return query
}

func mustCreateQueryWithFilter(t *testing.T, text string, filterExpr ast.Expr) *Query {
	query := mustCreateQuery(t, text)
	query.Set(FilterExprKey, filterExpr)
	return query
}

func mustCreateQueryWithFilterString(t *testing.T, text string, filterStr string) *Query {
	query := mustCreateQuery(t, text)
	query.Set(FilterExprKey, filterStr)
	return query
}
