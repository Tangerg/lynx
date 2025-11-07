package expanders

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
	"github.com/Tangerg/lynx/ai/model"
	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/rag"
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
		model.NewApiKey(config.apiKey),
		defaultOptions,
		option.WithBaseURL(config.baseURL),
	)
	require.NoError(t, err)

	return chatModel
}

func TestMultiExpanderConfig_validate(t *testing.T) {
	chatModel := newTestChatModel(t)

	tests := []struct {
		name    string
		config  *MultiExpanderConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "multi expander config cannot be nil",
		},
		{
			name: "nil chat model",
			config: &MultiExpanderConfig{
				ChatModel: nil,
			},
			wantErr: true,
			errMsg:  "chat model is required",
		},
		{
			name: "negative number of queries",
			config: &MultiExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: -1,
			},
			wantErr: true,
			errMsg:  "number of queries must be positive",
		},
		{
			name: "zero number of queries defaults to 3",
			config: &MultiExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 0,
			},
			wantErr: false,
		},
		{
			name: "valid config",
			config: &MultiExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 5,
			},
			wantErr: false,
		},
		{
			name: "valid config with include original",
			config: &MultiExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 3,
				IncludeOriginal: true,
			},
			wantErr: false,
		},
		{
			name: "valid config with custom template",
			config: &MultiExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 2,
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate("Generate {{.Number}} variants for: {{.Query}}"),
			},
			wantErr: false,
		},
		{
			name: "invalid template missing Number variable",
			config: &MultiExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 3,
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate("Generate variants for: {{.Query}}"),
			},
			wantErr: true,
		},
		{
			name: "invalid template missing Query variable",
			config: &MultiExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 3,
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate("Generate {{.Number}} variants"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				if tt.config != nil && tt.config.NumberOfQueries == 0 {
					assert.Equal(t, 3, tt.config.NumberOfQueries)
				}
				if tt.config != nil && tt.config.PromptTemplate == nil {
					assert.NotNil(t, tt.config.PromptTemplate)
				}
			}
		})
	}
}

func TestNewMultiExpander(t *testing.T) {
	chatModel := newTestChatModel(t)

	tests := []struct {
		name    string
		config  *MultiExpanderConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "valid config",
			config: &MultiExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 3,
			},
			wantErr: false,
		},
		{
			name: "valid config with custom settings",
			config: &MultiExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 5,
				IncludeOriginal: true,
			},
			wantErr: false,
		},
		{
			name: "invalid config",
			config: &MultiExpanderConfig{
				ChatModel: nil,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expander, err := NewMultiExpander(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, expander)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, expander)
				assert.NotNil(t, expander.chatClient)
				assert.NotNil(t, expander.promptTemplate)
				assert.Equal(t, tt.config.IncludeOriginal, expander.includeOriginal)
				assert.Equal(t, tt.config.NumberOfQueries, expander.numberOfQueries)
				assert.Implements(t, (*rag.QueryExpander)(nil), expander)
			}
		})
	}
}

func TestMultiExpander_Expand(t *testing.T) {
	chatModel := newTestChatModel(t)

	tests := []struct {
		name           string
		config         *MultiExpanderConfig
		query          *rag.Query
		wantErr        bool
		validateResult func(t *testing.T, result []*rag.Query, original *rag.Query)
	}{
		{
			name: "nil query",
			config: &MultiExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 3,
			},
			query:   nil,
			wantErr: true,
		},
		{
			name: "basic expansion without original",
			config: &MultiExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 3,
				IncludeOriginal: false,
			},
			query:   mustCreateQuery(t, "What is machine learning?"),
			wantErr: false,
			validateResult: func(t *testing.T, result []*rag.Query, original *rag.Query) {
				assert.NotEmpty(t, result)
				assert.LessOrEqual(t, len(result), 3)
				for _, q := range result {
					assert.NotEmpty(t, q.Text)
					assert.NotEqual(t, original.Text, q.Text)
				}
			},
		},
		{
			name: "expansion with original included",
			config: &MultiExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 3,
				IncludeOriginal: true,
			},
			query:   mustCreateQuery(t, "What is artificial intelligence?"),
			wantErr: false,
			validateResult: func(t *testing.T, result []*rag.Query, original *rag.Query) {
				assert.NotEmpty(t, result)
				assert.LessOrEqual(t, len(result), 4)
				found := false
				for _, q := range result {
					if q.Text == original.Text {
						found = true
						break
					}
				}
				assert.True(t, found, "original query should be included")
			},
		},
		{
			name: "expansion with different number of queries",
			config: &MultiExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 5,
				IncludeOriginal: false,
			},
			query:   mustCreateQuery(t, "How does neural network work?"),
			wantErr: false,
			validateResult: func(t *testing.T, result []*rag.Query, original *rag.Query) {
				assert.NotEmpty(t, result)
				assert.LessOrEqual(t, len(result), 5)
			},
		},
		{
			name: "expansion with custom template",
			config: &MultiExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 2,
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate(`Generate {{.Number}} alternative search queries for: {{.Query}}
Provide only the queries, one per line.`),
			},
			query:   mustCreateQuery(t, "deep learning applications"),
			wantErr: false,
			validateResult: func(t *testing.T, result []*rag.Query, original *rag.Query) {
				assert.NotEmpty(t, result)
				assert.LessOrEqual(t, len(result), 2)
			},
		},
		{
			name: "expansion preserves query metadata",
			config: &MultiExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 2,
			},
			query:   mustCreateQueryWithMetadata(t, "What is NLP?", map[string]any{"source": "test"}),
			wantErr: false,
			validateResult: func(t *testing.T, result []*rag.Query, original *rag.Query) {
				assert.NotEmpty(t, result)
				for _, q := range result {
					val, exists := q.Get("source")
					assert.True(t, exists)
					assert.Equal(t, "test", val)
				}
			},
		},
		{
			name: "short query expansion",
			config: &MultiExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 3,
			},
			query:   mustCreateQuery(t, "AI"),
			wantErr: false,
			validateResult: func(t *testing.T, result []*rag.Query, original *rag.Query) {
				assert.NotEmpty(t, result)
			},
		},
		{
			name: "complex query expansion",
			config: &MultiExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 3,
			},
			query:   mustCreateQuery(t, "How can transformer models be applied to natural language understanding tasks in production environments?"),
			wantErr: false,
			validateResult: func(t *testing.T, result []*rag.Query, original *rag.Query) {
				assert.NotEmpty(t, result)
				for _, q := range result {
					assert.NotEmpty(t, q.Text)
				}
			},
		},
		{
			name: "single query expansion",
			config: &MultiExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 1,
				IncludeOriginal: false,
			},
			query:   mustCreateQuery(t, "What is deep learning?"),
			wantErr: false,
			validateResult: func(t *testing.T, result []*rag.Query, original *rag.Query) {
				assert.NotEmpty(t, result)
				assert.LessOrEqual(t, len(result), 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expander, err := NewMultiExpander(tt.config)
			require.NoError(t, err)

			ctx := context.Background()
			result, err := expander.Expand(ctx, tt.query)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tt.validateResult != nil {
				tt.validateResult(t, result, tt.query)
			}
		})
	}
}

func TestMultiExpander_Expand_EmptyResponse(t *testing.T) {
	chatModel := newTestChatModel(t)

	config := &MultiExpanderConfig{
		ChatModel:       chatModel,
		NumberOfQueries: 3,
	}

	expander, err := NewMultiExpander(config)
	require.NoError(t, err)

	ctx := context.Background()
	query := mustCreateQuery(t, "test query")

	result, err := expander.Expand(ctx, query)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}

func TestMultiExpander_Expand_FilterEmptyVariants(t *testing.T) {
	chatModel := newTestChatModel(t)

	config := &MultiExpanderConfig{
		ChatModel:       chatModel,
		NumberOfQueries: 5,
		IncludeOriginal: false,
	}

	expander, err := NewMultiExpander(config)
	require.NoError(t, err)

	ctx := context.Background()
	query := mustCreateQuery(t, "What is deep learning?")

	result, err := expander.Expand(ctx, query)
	require.NoError(t, err)

	for _, q := range result {
		assert.NotEmpty(t, strings.TrimSpace(q.Text))
	}
}

func TestMultiExpander_Expand_RespectNumberLimit(t *testing.T) {
	chatModel := newTestChatModel(t)

	config := &MultiExpanderConfig{
		ChatModel:       chatModel,
		NumberOfQueries: 2,
		IncludeOriginal: false,
	}

	expander, err := NewMultiExpander(config)
	require.NoError(t, err)

	ctx := context.Background()
	query := mustCreateQuery(t, "Explain reinforcement learning")

	result, err := expander.Expand(ctx, query)
	require.NoError(t, err)

	assert.LessOrEqual(t, len(result), 2)
}

func TestMultiExpander_Expand_WithIncludeOriginalAndLimit(t *testing.T) {
	chatModel := newTestChatModel(t)

	config := &MultiExpanderConfig{
		ChatModel:       chatModel,
		NumberOfQueries: 2,
		IncludeOriginal: true,
	}

	expander, err := NewMultiExpander(config)
	require.NoError(t, err)

	ctx := context.Background()
	query := mustCreateQuery(t, "computer vision techniques")

	result, err := expander.Expand(ctx, query)
	require.NoError(t, err)

	assert.LessOrEqual(t, len(result), 3)

	originalFound := false
	for _, q := range result {
		if q.Text == query.Text {
			originalFound = true
		}
	}
	assert.True(t, originalFound, "original query should be in results")
}

func TestMultiExpander_Expand_NilContext(t *testing.T) {
	chatModel := newTestChatModel(t)

	config := &MultiExpanderConfig{
		ChatModel:       chatModel,
		NumberOfQueries: 2,
	}

	expander, err := NewMultiExpander(config)
	require.NoError(t, err)

	query := mustCreateQuery(t, "test query")

	result, err := expander.Expand(nil, query)
	require.Error(t, err)
	assert.Empty(t, result)
}

func mustCreateQuery(t *testing.T, text string) *rag.Query {
	query, err := rag.NewQuery(text)
	require.NoError(t, err)
	return query
}

func mustCreateQueryWithMetadata(t *testing.T, text string, metadata map[string]any) *rag.Query {
	query := mustCreateQuery(t, text)
	for k, v := range metadata {
		query.Set(k, v)
	}
	return query
}
