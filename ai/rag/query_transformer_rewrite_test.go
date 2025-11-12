package rag

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/model/chat"
)

func TestRewriteTransformerConfig_validate(t *testing.T) {
	chatModel := newTestChatModel(t)

	tests := []struct {
		name    string
		config  *RewriteQueryTransformerConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "rewrite transformer config cannot be nil",
		},
		{
			name: "nil chat model",
			config: &RewriteQueryTransformerConfig{
				ChatModel: nil,
			},
			wantErr: true,
			errMsg:  "chat model is required",
		},
		{
			name: "valid config with defaults",
			config: &RewriteQueryTransformerConfig{
				ChatModel: chatModel,
			},
			wantErr: false,
		},
		{
			name: "valid config with custom target",
			config: &RewriteQueryTransformerConfig{
				ChatModel:          chatModel,
				TargetSearchSystem: "web search engine",
			},
			wantErr: false,
		},
		{
			name: "valid config with custom template",
			config: &RewriteQueryTransformerConfig{
				ChatModel:          chatModel,
				TargetSearchSystem: "database",
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate("Optimize for {{.Target}}: {{.Query}}"),
			},
			wantErr: false,
		},
		{
			name: "invalid template missing Target variable",
			config: &RewriteQueryTransformerConfig{
				ChatModel: chatModel,
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate("Query: {{.Query}}"),
			},
			wantErr: true,
		},
		{
			name: "invalid template missing Query variable",
			config: &RewriteQueryTransformerConfig{
				ChatModel: chatModel,
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate("Target: {{.Target}}"),
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
				if tt.config != nil {
					if tt.config.TargetSearchSystem == "" {
						assert.Equal(t, "vector store", tt.config.TargetSearchSystem)
					}
					if tt.config.PromptTemplate == nil {
						assert.NotNil(t, tt.config.PromptTemplate)
					}
				}
			}
		})
	}
}

func TestNewRewriteTransformer(t *testing.T) {
	chatModel := newTestChatModel(t)

	tests := []struct {
		name    string
		config  *RewriteQueryTransformerConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "valid config",
			config: &RewriteQueryTransformerConfig{
				ChatModel: chatModel,
			},
			wantErr: false,
		},
		{
			name: "valid config with custom settings",
			config: &RewriteQueryTransformerConfig{
				ChatModel:          chatModel,
				TargetSearchSystem: "elasticsearch",
			},
			wantErr: false,
		},
		{
			name: "invalid config",
			config: &RewriteQueryTransformerConfig{
				ChatModel: nil,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformer, err := NewRewriteQueryTransformer(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, transformer)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, transformer)
				assert.NotNil(t, transformer.chatClient)
				assert.NotNil(t, transformer.promptTemplate)
				assert.NotEmpty(t, transformer.targetSearchSystem)
				assert.Implements(t, (*QueryTransformer)(nil), transformer)
			}
		})
	}
}

func TestRewriteTransformer_Transform(t *testing.T) {
	chatModel := newTestChatModel(t)

	tests := []struct {
		name           string
		config         *RewriteQueryTransformerConfig
		query          *Query
		wantErr        bool
		validateResult func(t *testing.T, result *Query, original *Query)
	}{
		{
			name: "nil query",
			config: &RewriteQueryTransformerConfig{
				ChatModel: chatModel,
			},
			query:   nil,
			wantErr: true,
		},
		{
			name: "simple query rewrite",
			config: &RewriteQueryTransformerConfig{
				ChatModel: chatModel,
			},
			query:   mustCreateQuery(t, "What is machine learning?"),
			wantErr: false,
			validateResult: func(t *testing.T, result *Query, original *Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
			},
		},
		{
			name: "verbose query rewrite",
			config: &RewriteQueryTransformerConfig{
				ChatModel: chatModel,
			},
			query:   mustCreateQuery(t, "I'm curious about machine learning and I was wondering if you could tell me what it is and how it works?"),
			wantErr: false,
			validateResult: func(t *testing.T, result *Query, original *Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
				assert.Less(t, len(result.Text), len(original.Text))
			},
		},
		{
			name: "query with irrelevant information",
			config: &RewriteQueryTransformerConfig{
				ChatModel: chatModel,
			},
			query:   mustCreateQuery(t, "My professor mentioned deep learning yesterday in class, can you explain what it means?"),
			wantErr: false,
			validateResult: func(t *testing.T, result *Query, original *Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
				assert.NotContains(t, strings.ToLower(result.Text), "professor")
				assert.NotContains(t, strings.ToLower(result.Text), "yesterday")
			},
		},
		{
			name: "web search engine target",
			config: &RewriteQueryTransformerConfig{
				ChatModel:          chatModel,
				TargetSearchSystem: "web search engine",
			},
			query:   mustCreateQuery(t, "How to use transformers in NLP?"),
			wantErr: false,
			validateResult: func(t *testing.T, result *Query, original *Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
			},
		},
		{
			name: "database target",
			config: &RewriteQueryTransformerConfig{
				ChatModel:          chatModel,
				TargetSearchSystem: "database",
			},
			query:   mustCreateQuery(t, "Show me all users who signed up last month"),
			wantErr: false,
			validateResult: func(t *testing.T, result *Query, original *Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
			},
		},
		{
			name: "custom template",
			config: &RewriteQueryTransformerConfig{
				ChatModel:          chatModel,
				TargetSearchSystem: "vector store",
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate(`Rewrite this query for {{.Target}}:
{{.Query}}

Optimized:`),
			},
			query:   mustCreateQuery(t, "Tell me about neural networks"),
			wantErr: false,
			validateResult: func(t *testing.T, result *Query, original *Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
			},
		},
		{
			name: "query preserves metadata",
			config: &RewriteQueryTransformerConfig{
				ChatModel: chatModel,
			},
			query: func() *Query {
				q := mustCreateQuery(t, "What is AI?")
				q.Set("source", "test")
				q.Set("user_id", "123")
				return q
			}(),
			wantErr: false,
			validateResult: func(t *testing.T, result *Query, original *Query) {
				assert.NotNil(t, result)
				source, exists := result.Get("source")
				assert.True(t, exists)
				assert.Equal(t, "test", source)
				userId, exists := result.Get("user_id")
				assert.True(t, exists)
				assert.Equal(t, "123", userId)
			},
		},
		{
			name: "ambiguous query",
			config: &RewriteQueryTransformerConfig{
				ChatModel: chatModel,
			},
			query:   mustCreateQuery(t, "it"),
			wantErr: false,
			validateResult: func(t *testing.T, result *Query, original *Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
			},
		},
		{
			name: "query with multiple questions",
			config: &RewriteQueryTransformerConfig{
				ChatModel: chatModel,
			},
			query:   mustCreateQuery(t, "What is machine learning? How does it differ from deep learning? Can you give examples?"),
			wantErr: false,
			validateResult: func(t *testing.T, result *Query, original *Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
			},
		},
		{
			name: "technical query with jargon",
			config: &RewriteQueryTransformerConfig{
				ChatModel: chatModel,
			},
			query:   mustCreateQuery(t, "Explain backpropagation in CNNs for image classification tasks"),
			wantErr: false,
			validateResult: func(t *testing.T, result *Query, original *Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
			},
		},
		{
			name: "conversational query",
			config: &RewriteQueryTransformerConfig{
				ChatModel: chatModel,
			},
			query:   mustCreateQuery(t, "Hey, could you maybe help me understand what reinforcement learning is all about?"),
			wantErr: false,
			validateResult: func(t *testing.T, result *Query, original *Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
				assert.NotContains(t, strings.ToLower(result.Text), "hey")
				assert.NotContains(t, strings.ToLower(result.Text), "maybe")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformer, err := NewRewriteQueryTransformer(tt.config)
			require.NoError(t, err)

			ctx := context.Background()
			result, err := transformer.Transform(ctx, tt.query)

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

func TestRewriteTransformer_Transform_DefaultTarget(t *testing.T) {
	chatModel := newTestChatModel(t)

	config := &RewriteQueryTransformerConfig{
		ChatModel: chatModel,
	}

	transformer, err := NewRewriteQueryTransformer(config)
	require.NoError(t, err)

	assert.Equal(t, "vector store", transformer.targetSearchSystem)

	ctx := context.Background()
	query := mustCreateQuery(t, "test query")

	result, err := transformer.Transform(ctx, query)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestRewriteTransformer_Transform_EmptyResponse(t *testing.T) {
	chatModel := newTestChatModel(t)

	config := &RewriteQueryTransformerConfig{
		ChatModel: chatModel,
	}

	transformer, err := NewRewriteQueryTransformer(config)
	require.NoError(t, err)

	ctx := context.Background()
	query := mustCreateQuery(t, "test")

	result, err := transformer.Transform(ctx, query)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestRewriteTransformer_Transform_DifferentTargets(t *testing.T) {
	chatModel := newTestChatModel(t)

	targets := []string{
		"vector store",
		"web search engine",
		"database",
		"elasticsearch",
		"knowledge graph",
	}

	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			config := &RewriteQueryTransformerConfig{
				ChatModel:          chatModel,
				TargetSearchSystem: target,
			}

			transformer, err := NewRewriteQueryTransformer(config)
			require.NoError(t, err)

			assert.Equal(t, target, transformer.targetSearchSystem)

			ctx := context.Background()
			query := mustCreateQuery(t, "machine learning algorithms")

			result, err := transformer.Transform(ctx, query)
			require.NoError(t, err)
			assert.NotNil(t, result)
			assert.NotEmpty(t, result.Text)
		})
	}
}
