package transformers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/rag"
)

func TestCompressionTransformerConfig_validate(t *testing.T) {
	chatModel := newTestChatModel(t)

	tests := []struct {
		name    string
		config  *CompressionTransformerConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "compression transformer config cannot be nil",
		},
		{
			name: "nil chat model",
			config: &CompressionTransformerConfig{
				ChatModel: nil,
			},
			wantErr: true,
			errMsg:  "chat model is required",
		},
		{
			name: "valid config with defaults",
			config: &CompressionTransformerConfig{
				ChatModel: chatModel,
			},
			wantErr: false,
		},
		{
			name: "valid config with custom template",
			config: &CompressionTransformerConfig{
				ChatModel: chatModel,
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate("History: {{.History}}\nQuery: {{.Query}}\nStandalone:"),
			},
			wantErr: false,
		},
		{
			name: "invalid template missing History variable",
			config: &CompressionTransformerConfig{
				ChatModel: chatModel,
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate("Query: {{.Query}}\nStandalone:"),
			},
			wantErr: true,
		},
		{
			name: "invalid template missing Query variable",
			config: &CompressionTransformerConfig{
				ChatModel: chatModel,
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate("History: {{.History}}\nStandalone:"),
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
				if tt.config != nil && tt.config.PromptTemplate == nil {
					assert.NotNil(t, tt.config.PromptTemplate)
				}
			}
		})
	}
}

func TestNewCompressionTransformer(t *testing.T) {
	chatModel := newTestChatModel(t)

	tests := []struct {
		name    string
		config  *CompressionTransformerConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "valid config",
			config: &CompressionTransformerConfig{
				ChatModel: chatModel,
			},
			wantErr: false,
		},
		{
			name: "valid config with custom template",
			config: &CompressionTransformerConfig{
				ChatModel: chatModel,
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate("History: {{.History}}\nQuery: {{.Query}}\nResult:"),
			},
			wantErr: false,
		},
		{
			name: "invalid config",
			config: &CompressionTransformerConfig{
				ChatModel: nil,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformer, err := NewCompressionTransformer(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, transformer)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, transformer)
				assert.NotNil(t, transformer.chatClient)
				assert.NotNil(t, transformer.promptTemplate)
				assert.Implements(t, (*rag.QueryTransformer)(nil), transformer)
			}
		})
	}
}

func TestCompressionTransformer_Transform(t *testing.T) {
	chatModel := newTestChatModel(t)

	tests := []struct {
		name           string
		config         *CompressionTransformerConfig
		query          *rag.Query
		wantErr        bool
		validateResult func(t *testing.T, result *rag.Query, original *rag.Query)
	}{
		{
			name: "nil query",
			config: &CompressionTransformerConfig{
				ChatModel: chatModel,
			},
			query:   nil,
			wantErr: true,
		},
		{
			name: "query without history",
			config: &CompressionTransformerConfig{
				ChatModel: chatModel,
			},
			query:   mustCreateQuery(t, "What is machine learning?"),
			wantErr: false,
			validateResult: func(t *testing.T, result *rag.Query, original *rag.Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
			},
		},
		{
			name: "query with simple history",
			config: &CompressionTransformerConfig{
				ChatModel: chatModel,
			},
			query: mustCreateQueryWithHistory(t, "Tell me more about it", []chat.Message{
				chat.NewUserMessage(chat.MessageParams{Text: "What is machine learning?"}),
				chat.NewAssistantMessage("Machine learning is a subset of AI that enables systems to learn from data."),
			}),
			wantErr: false,
			validateResult: func(t *testing.T, result *rag.Query, original *rag.Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
				assert.NotEqual(t, original.Text, result.Text)
			},
		},
		{
			name: "query with complex history",
			config: &CompressionTransformerConfig{
				ChatModel: chatModel,
			},
			query: mustCreateQueryWithHistory(t, "How does it compare?", []chat.Message{
				chat.NewUserMessage(chat.MessageParams{Text: "What is deep learning?"}),
				chat.NewAssistantMessage("Deep learning is a subset of machine learning using neural networks."),
				chat.NewUserMessage(chat.MessageParams{Text: "What about supervised learning?"}),
				chat.NewAssistantMessage("Supervised learning uses labeled data to train models."),
			}),
			wantErr: false,
			validateResult: func(t *testing.T, result *rag.Query, original *rag.Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
				assert.NotEqual(t, "How does it compare?", result.Text)
			},
		},
		{
			name: "query with custom template",
			config: &CompressionTransformerConfig{
				ChatModel: chatModel,
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate(`Combine the following conversation and question into a single query:

Conversation:
{{.History}}

Question: {{.Query}}

Combined query:`),
			},
			query: mustCreateQueryWithHistory(t, "What are the applications?", []chat.Message{
				chat.NewUserMessage(chat.MessageParams{Text: "Tell me about neural networks"}),
				chat.NewAssistantMessage("Neural networks are computing systems inspired by biological networks."),
			}),
			wantErr: false,
			validateResult: func(t *testing.T, result *rag.Query, original *rag.Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
			},
		},
		{
			name: "query preserves metadata",
			config: &CompressionTransformerConfig{
				ChatModel: chatModel,
			},
			query: mustCreateQueryWithHistoryAndMetadata(t, "What else?", []chat.Message{
				chat.NewUserMessage(chat.MessageParams{Text: "What is AI?"}),
				chat.NewAssistantMessage("AI stands for Artificial Intelligence."),
			}, map[string]any{
				"source":  "test",
				"user_id": "123",
			}),
			wantErr: false,
			validateResult: func(t *testing.T, result *rag.Query, original *rag.Query) {
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
			name: "long conversation history",
			config: &CompressionTransformerConfig{
				ChatModel: chatModel,
			},
			query: mustCreateQueryWithHistory(t, "Can you summarize?", []chat.Message{
				chat.NewUserMessage(chat.MessageParams{Text: "What is machine learning?"}),
				chat.NewAssistantMessage("Machine learning is a method of data analysis."),
				chat.NewUserMessage(chat.MessageParams{Text: "What are the types?"}),
				chat.NewAssistantMessage("There are supervised, unsupervised, and reinforcement learning."),
				chat.NewUserMessage(chat.MessageParams{Text: "Explain supervised learning"}),
				chat.NewAssistantMessage("Supervised learning uses labeled training data."),
			}),
			wantErr: false,
			validateResult: func(t *testing.T, result *rag.Query, original *rag.Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
			},
		},
		{
			name: "query with pronoun reference",
			config: &CompressionTransformerConfig{
				ChatModel: chatModel,
			},
			query: mustCreateQueryWithHistory(t, "How does it work?", []chat.Message{
				chat.NewUserMessage("Tell me about transformers in NLP."),
				chat.NewAssistantMessage("Transformers are a type of neural network architecture."),
			}),
			wantErr: false,
			validateResult: func(t *testing.T, result *rag.Query, original *rag.Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformer, err := NewCompressionTransformer(tt.config)
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

func TestCompressionTransformer_Transform_EmptyHistory(t *testing.T) {
	chatModel := newTestChatModel(t)

	config := &CompressionTransformerConfig{
		ChatModel: chatModel,
	}

	transformer, err := NewCompressionTransformer(config)
	require.NoError(t, err)

	query := mustCreateQueryWithHistory(t, "What is AI?", []chat.Message{})

	ctx := context.Background()
	result, err := transformer.Transform(ctx, query)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.Text)
}

func TestCompressionTransformer_extractConversationHistory(t *testing.T) {
	chatModel := newTestChatModel(t)

	config := &CompressionTransformerConfig{
		ChatModel: chatModel,
	}

	transformer, err := NewCompressionTransformer(config)
	require.NoError(t, err)

	tests := []struct {
		name     string
		query    *rag.Query
		expected string
	}{
		{
			name:     "no history",
			query:    mustCreateQuery(t, "test"),
			expected: "",
		},
		{
			name: "with history",
			query: mustCreateQueryWithHistory(t, "follow up", []chat.Message{
				chat.NewUserMessage(chat.MessageParams{Text: "Hello"}),
				chat.NewAssistantMessage("Hi there!"),
			}),
			expected: "user: Hello\n\nassistant: Hi there!",
		},
		{
			name: "invalid history type",
			query: func() *rag.Query {
				q := mustCreateQuery(t, "test")
				q.Set(rag.ChatHistoryKey, "invalid")
				return q
			}(),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := transformer.extractConversationHistory(tt.query)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCompressionTransformer_Transform_EmptyResponse(t *testing.T) {
	chatModel := newTestChatModel(t)

	config := &CompressionTransformerConfig{
		ChatModel: chatModel,
	}

	transformer, err := NewCompressionTransformer(config)
	require.NoError(t, err)

	query := mustCreateQuery(t, "test")

	ctx := context.Background()
	result, err := transformer.Transform(ctx, query)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func mustCreateQuery(t *testing.T, text string) *rag.Query {
	query, err := rag.NewQuery(text)
	require.NoError(t, err)
	return query
}

func mustCreateQueryWithHistory(t *testing.T, text string, history []chat.Message) *rag.Query {
	query := mustCreateQuery(t, text)
	query.Set(rag.ChatHistoryKey, history)
	return query
}

func mustCreateQueryWithHistoryAndMetadata(t *testing.T, text string, history []chat.Message, metadata map[string]any) *rag.Query {
	query := mustCreateQueryWithHistory(t, text, history)
	for k, v := range metadata {
		query.Set(k, v)
	}
	return query
}
