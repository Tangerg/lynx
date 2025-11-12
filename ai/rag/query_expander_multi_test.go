package rag

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/model/chat"
)

func TestMultiExpanderConfig_validate(t *testing.T) {
	chatModel := newTestChatModel(t)

	tests := []struct {
		name    string
		config  *MultiQueryExpanderConfig
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
			config: &MultiQueryExpanderConfig{
				ChatModel: nil,
			},
			wantErr: true,
			errMsg:  "chat model is required",
		},
		{
			name: "negative number of queries",
			config: &MultiQueryExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: -1,
			},
			wantErr: true,
			errMsg:  "number of queries must be positive",
		},
		{
			name: "zero number of queries defaults to 3",
			config: &MultiQueryExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 0,
			},
			wantErr: false,
		},
		{
			name: "valid config",
			config: &MultiQueryExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 5,
			},
			wantErr: false,
		},
		{
			name: "valid config with include original",
			config: &MultiQueryExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 3,
				IncludeOriginal: true,
			},
			wantErr: false,
		},
		{
			name: "valid config with custom template",
			config: &MultiQueryExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 2,
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate("Generate {{.Number}} variants for: {{.Query}}"),
			},
			wantErr: false,
		},
		{
			name: "invalid template missing Number variable",
			config: &MultiQueryExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 3,
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate("Generate variants for: {{.Query}}"),
			},
			wantErr: true,
		},
		{
			name: "invalid template missing Query variable",
			config: &MultiQueryExpanderConfig{
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
		config  *MultiQueryExpanderConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "valid config",
			config: &MultiQueryExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 3,
			},
			wantErr: false,
		},
		{
			name: "valid config with custom settings",
			config: &MultiQueryExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 5,
				IncludeOriginal: true,
			},
			wantErr: false,
		},
		{
			name: "invalid config",
			config: &MultiQueryExpanderConfig{
				ChatModel: nil,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expander, err := NewMultiQueryExpander(tt.config)
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
				assert.Implements(t, (*QueryExpander)(nil), expander)
			}
		})
	}
}

func TestMultiExpander_Expand(t *testing.T) {
	chatModel := newTestChatModel(t)

	tests := []struct {
		name           string
		config         *MultiQueryExpanderConfig
		query          *Query
		wantErr        bool
		validateResult func(t *testing.T, result []*Query, original *Query)
	}{
		{
			name: "nil query",
			config: &MultiQueryExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 3,
			},
			query:   nil,
			wantErr: true,
		},
		{
			name: "basic expansion without original",
			config: &MultiQueryExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 3,
				IncludeOriginal: false,
			},
			query:   mustCreateQuery(t, "What is machine learning?"),
			wantErr: false,
			validateResult: func(t *testing.T, result []*Query, original *Query) {
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
			config: &MultiQueryExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 3,
				IncludeOriginal: true,
			},
			query:   mustCreateQuery(t, "What is artificial intelligence?"),
			wantErr: false,
			validateResult: func(t *testing.T, result []*Query, original *Query) {
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
			config: &MultiQueryExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 5,
				IncludeOriginal: false,
			},
			query:   mustCreateQuery(t, "How does neural network work?"),
			wantErr: false,
			validateResult: func(t *testing.T, result []*Query, original *Query) {
				assert.NotEmpty(t, result)
				assert.LessOrEqual(t, len(result), 5)
			},
		},
		{
			name: "expansion with custom template",
			config: &MultiQueryExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 2,
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate(`Generate {{.Number}} alternative search queries for: {{.Query}}
Provide only the queries, one per line.`),
			},
			query:   mustCreateQuery(t, "deep learning applications"),
			wantErr: false,
			validateResult: func(t *testing.T, result []*Query, original *Query) {
				assert.NotEmpty(t, result)
				assert.LessOrEqual(t, len(result), 2)
			},
		},
		{
			name: "expansion preserves query metadata",
			config: &MultiQueryExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 2,
			},
			query:   mustCreateQueryWithMetadata(t, "What is NLP?", map[string]any{"source": "test"}),
			wantErr: false,
			validateResult: func(t *testing.T, result []*Query, original *Query) {
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
			config: &MultiQueryExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 3,
			},
			query:   mustCreateQuery(t, "AI"),
			wantErr: false,
			validateResult: func(t *testing.T, result []*Query, original *Query) {
				assert.NotEmpty(t, result)
			},
		},
		{
			name: "complex query expansion",
			config: &MultiQueryExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 3,
			},
			query:   mustCreateQuery(t, "How can transformer models be applied to natural language understanding tasks in production environments?"),
			wantErr: false,
			validateResult: func(t *testing.T, result []*Query, original *Query) {
				assert.NotEmpty(t, result)
				for _, q := range result {
					assert.NotEmpty(t, q.Text)
				}
			},
		},
		{
			name: "single query expansion",
			config: &MultiQueryExpanderConfig{
				ChatModel:       chatModel,
				NumberOfQueries: 1,
				IncludeOriginal: false,
			},
			query:   mustCreateQuery(t, "What is deep learning?"),
			wantErr: false,
			validateResult: func(t *testing.T, result []*Query, original *Query) {
				assert.NotEmpty(t, result)
				assert.LessOrEqual(t, len(result), 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expander, err := NewMultiQueryExpander(tt.config)
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

	config := &MultiQueryExpanderConfig{
		ChatModel:       chatModel,
		NumberOfQueries: 3,
	}

	expander, err := NewMultiQueryExpander(config)
	require.NoError(t, err)

	ctx := context.Background()
	query := mustCreateQuery(t, "test query")

	result, err := expander.Expand(ctx, query)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}

func TestMultiExpander_Expand_FilterEmptyVariants(t *testing.T) {
	chatModel := newTestChatModel(t)

	config := &MultiQueryExpanderConfig{
		ChatModel:       chatModel,
		NumberOfQueries: 5,
		IncludeOriginal: false,
	}

	expander, err := NewMultiQueryExpander(config)
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

	config := &MultiQueryExpanderConfig{
		ChatModel:       chatModel,
		NumberOfQueries: 2,
		IncludeOriginal: false,
	}

	expander, err := NewMultiQueryExpander(config)
	require.NoError(t, err)

	ctx := context.Background()
	query := mustCreateQuery(t, "Explain reinforcement learning")

	result, err := expander.Expand(ctx, query)
	require.NoError(t, err)

	assert.LessOrEqual(t, len(result), 2)
}

func TestMultiExpander_Expand_WithIncludeOriginalAndLimit(t *testing.T) {
	chatModel := newTestChatModel(t)

	config := &MultiQueryExpanderConfig{
		ChatModel:       chatModel,
		NumberOfQueries: 2,
		IncludeOriginal: true,
	}

	expander, err := NewMultiQueryExpander(config)
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

	config := &MultiQueryExpanderConfig{
		ChatModel:       chatModel,
		NumberOfQueries: 2,
	}

	expander, err := NewMultiQueryExpander(config)
	require.NoError(t, err)

	query := mustCreateQuery(t, "test query")

	result, err := expander.Expand(nil, query)
	require.Error(t, err)
	assert.Empty(t, result)
}
