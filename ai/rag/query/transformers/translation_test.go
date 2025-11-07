package transformers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/rag"
)

func TestTranslationTransformerConfig_validate(t *testing.T) {
	chatModel := newTestChatModel(t)

	tests := []struct {
		name    string
		config  *TranslationTransformerConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "translation transformer config cannot be nil",
		},
		{
			name: "nil chat model",
			config: &TranslationTransformerConfig{
				ChatModel: nil,
			},
			wantErr: true,
			errMsg:  "chat model is required",
		},
		{
			name: "empty target language",
			config: &TranslationTransformerConfig{
				ChatModel:      chatModel,
				TargetLanguage: "",
			},
			wantErr: true,
			errMsg:  "target language is required",
		},
		{
			name: "valid config",
			config: &TranslationTransformerConfig{
				ChatModel:      chatModel,
				TargetLanguage: "English",
			},
			wantErr: false,
		},
		{
			name: "valid config with Chinese",
			config: &TranslationTransformerConfig{
				ChatModel:      chatModel,
				TargetLanguage: "Chinese",
			},
			wantErr: false,
		},
		{
			name: "valid config with custom template",
			config: &TranslationTransformerConfig{
				ChatModel:      chatModel,
				TargetLanguage: "Spanish",
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate("Translate to {{.Target}}: {{.Query}}"),
			},
			wantErr: false,
		},
		{
			name: "invalid template missing Target variable",
			config: &TranslationTransformerConfig{
				ChatModel:      chatModel,
				TargetLanguage: "English",
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate("Translate: {{.Query}}"),
			},
			wantErr: true,
		},
		{
			name: "invalid template missing Query variable",
			config: &TranslationTransformerConfig{
				ChatModel:      chatModel,
				TargetLanguage: "English",
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate("Translate to {{.Target}}"),
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

func TestNewTranslationTransformer(t *testing.T) {
	chatModel := newTestChatModel(t)

	tests := []struct {
		name    string
		config  *TranslationTransformerConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "valid config",
			config: &TranslationTransformerConfig{
				ChatModel:      chatModel,
				TargetLanguage: "English",
			},
			wantErr: false,
		},
		{
			name: "valid config with custom template",
			config: &TranslationTransformerConfig{
				ChatModel:      chatModel,
				TargetLanguage: "French",
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate("Convert to {{.Target}}: {{.Query}}"),
			},
			wantErr: false,
		},
		{
			name: "invalid config",
			config: &TranslationTransformerConfig{
				ChatModel:      nil,
				TargetLanguage: "English",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformer, err := NewTranslationTransformer(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, transformer)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, transformer)
				assert.NotNil(t, transformer.chatClient)
				assert.NotNil(t, transformer.promptTemplate)
				assert.NotEmpty(t, transformer.targetLanguage)
				assert.Implements(t, (*rag.QueryTransformer)(nil), transformer)
			}
		})
	}
}

func TestTranslationTransformer_Transform(t *testing.T) {
	chatModel := newTestChatModel(t)

	tests := []struct {
		name           string
		config         *TranslationTransformerConfig
		query          *rag.Query
		wantErr        bool
		validateResult func(t *testing.T, result *rag.Query, original *rag.Query)
	}{
		{
			name: "nil query",
			config: &TranslationTransformerConfig{
				ChatModel:      chatModel,
				TargetLanguage: "English",
			},
			query:   nil,
			wantErr: true,
		},
		{
			name: "English query stays in English",
			config: &TranslationTransformerConfig{
				ChatModel:      chatModel,
				TargetLanguage: "English",
			},
			query:   mustCreateQuery(t, "What is machine learning?"),
			wantErr: false,
			validateResult: func(t *testing.T, result *rag.Query, original *rag.Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
			},
		},
		{
			name: "Chinese to English translation",
			config: &TranslationTransformerConfig{
				ChatModel:      chatModel,
				TargetLanguage: "English",
			},
			query:   mustCreateQuery(t, "什么是机器学习？"),
			wantErr: false,
			validateResult: func(t *testing.T, result *rag.Query, original *rag.Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
				assert.NotEqual(t, original.Text, result.Text)
			},
		},
		{
			name: "Spanish to English translation",
			config: &TranslationTransformerConfig{
				ChatModel:      chatModel,
				TargetLanguage: "English",
			},
			query:   mustCreateQuery(t, "¿Qué es el aprendizaje automático?"),
			wantErr: false,
			validateResult: func(t *testing.T, result *rag.Query, original *rag.Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
				assert.NotEqual(t, original.Text, result.Text)
			},
		},
		{
			name: "English to Chinese translation",
			config: &TranslationTransformerConfig{
				ChatModel:      chatModel,
				TargetLanguage: "Chinese",
			},
			query:   mustCreateQuery(t, "What is artificial intelligence?"),
			wantErr: false,
			validateResult: func(t *testing.T, result *rag.Query, original *rag.Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
				assert.NotEqual(t, original.Text, result.Text)
			},
		},
		{
			name: "Chinese query stays in Chinese",
			config: &TranslationTransformerConfig{
				ChatModel:      chatModel,
				TargetLanguage: "Chinese",
			},
			query:   mustCreateQuery(t, "什么是深度学习？"),
			wantErr: false,
			validateResult: func(t *testing.T, result *rag.Query, original *rag.Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
			},
		},
		{
			name: "French to English translation",
			config: &TranslationTransformerConfig{
				ChatModel:      chatModel,
				TargetLanguage: "English",
			},
			query:   mustCreateQuery(t, "Qu'est-ce que l'apprentissage profond?"),
			wantErr: false,
			validateResult: func(t *testing.T, result *rag.Query, original *rag.Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
				assert.NotEqual(t, original.Text, result.Text)
			},
		},
		{
			name: "custom template",
			config: &TranslationTransformerConfig{
				ChatModel:      chatModel,
				TargetLanguage: "English",
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate(`Translate the following to {{.Target}}. If already in {{.Target}}, keep unchanged:

{{.Query}}

Translation:`),
			},
			query:   mustCreateQuery(t, "什么是神经网络？"),
			wantErr: false,
			validateResult: func(t *testing.T, result *rag.Query, original *rag.Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
			},
		},
		{
			name: "query preserves metadata",
			config: &TranslationTransformerConfig{
				ChatModel:      chatModel,
				TargetLanguage: "English",
			},
			query: mustCreateQueryWithMetadata(t, "什么是AI？", map[string]any{
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
			name: "German to English translation",
			config: &TranslationTransformerConfig{
				ChatModel:      chatModel,
				TargetLanguage: "English",
			},
			query:   mustCreateQuery(t, "Was ist maschinelles Lernen?"),
			wantErr: false,
			validateResult: func(t *testing.T, result *rag.Query, original *rag.Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
				assert.NotEqual(t, original.Text, result.Text)
			},
		},
		{
			name: "Japanese to English translation",
			config: &TranslationTransformerConfig{
				ChatModel:      chatModel,
				TargetLanguage: "English",
			},
			query:   mustCreateQuery(t, "機械学習とは何ですか？"),
			wantErr: false,
			validateResult: func(t *testing.T, result *rag.Query, original *rag.Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
				assert.NotEqual(t, original.Text, result.Text)
			},
		},
		{
			name: "Korean to English translation",
			config: &TranslationTransformerConfig{
				ChatModel:      chatModel,
				TargetLanguage: "English",
			},
			query:   mustCreateQuery(t, "머신러닝이 무엇인가요?"),
			wantErr: false,
			validateResult: func(t *testing.T, result *rag.Query, original *rag.Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
				assert.NotEqual(t, original.Text, result.Text)
			},
		},
		{
			name: "complex technical query translation",
			config: &TranslationTransformerConfig{
				ChatModel:      chatModel,
				TargetLanguage: "English",
			},
			query:   mustCreateQuery(t, "解释卷积神经网络中的反向传播算法"),
			wantErr: false,
			validateResult: func(t *testing.T, result *rag.Query, original *rag.Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
			},
		},
		{
			name: "short query translation",
			config: &TranslationTransformerConfig{
				ChatModel:      chatModel,
				TargetLanguage: "English",
			},
			query:   mustCreateQuery(t, "人工智能"),
			wantErr: false,
			validateResult: func(t *testing.T, result *rag.Query, original *rag.Query) {
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.Text)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformer, err := NewTranslationTransformer(tt.config)
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

func TestTranslationTransformer_Transform_MultipleLanguages(t *testing.T) {
	chatModel := newTestChatModel(t)

	languages := []struct {
		target string
		query  string
	}{
		{"English", "什么是深度学习？"},
		{"Chinese", "What is deep learning?"},
		{"Spanish", "What is machine learning?"},
		{"French", "What is neural network?"},
		{"German", "What is AI?"},
	}

	for _, lang := range languages {
		t.Run(lang.target, func(t *testing.T) {
			config := &TranslationTransformerConfig{
				ChatModel:      chatModel,
				TargetLanguage: lang.target,
			}

			transformer, err := NewTranslationTransformer(config)
			require.NoError(t, err)

			ctx := context.Background()
			query := mustCreateQuery(t, lang.query)

			result, err := transformer.Transform(ctx, query)
			require.NoError(t, err)
			assert.NotNil(t, result)
			assert.NotEmpty(t, result.Text)
		})
	}
}

func TestTranslationTransformer_Transform_EmptyResponse(t *testing.T) {
	chatModel := newTestChatModel(t)

	config := &TranslationTransformerConfig{
		ChatModel:      chatModel,
		TargetLanguage: "English",
	}

	transformer, err := NewTranslationTransformer(config)
	require.NoError(t, err)

	ctx := context.Background()
	query := mustCreateQuery(t, "test")

	result, err := transformer.Transform(ctx, query)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func mustCreateQueryWithMetadata(t *testing.T, text string, metadata map[string]any) *rag.Query {
	query := mustCreateQuery(t, text)
	for k, v := range metadata {
		query.Set(k, v)
	}
	return query
}
