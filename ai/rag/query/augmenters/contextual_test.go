package augmenters

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/rag"
)

func TestContextualAugmenterConfig_validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *ContextualAugmenterConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "contextual augmenter config cannot be nil",
		},
		{
			name:    "valid config with defaults",
			config:  &ContextualAugmenterConfig{},
			wantErr: false,
		},
		{
			name: "valid config with custom templates",
			config: &ContextualAugmenterConfig{
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate("Context: {{.Context}}\nQuery: {{.Query}}"),
				EmptyContextPromptTemplate: chat.NewPromptTemplate().
					WithTemplate("No context available"),
			},
			wantErr: false,
		},
		{
			name: "invalid prompt template missing Context variable",
			config: &ContextualAugmenterConfig{
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate("Query: {{.Query}}"),
			},
			wantErr: true,
		},
		{
			name: "invalid prompt template missing Query variable",
			config: &ContextualAugmenterConfig{
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate("Context: {{.Context}}"),
			},
			wantErr: true,
		},
		{
			name: "valid config with AllowEmptyContext true",
			config: &ContextualAugmenterConfig{
				AllowEmptyContext: true,
			},
			wantErr: false,
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
					assert.NotNil(t, tt.config.PromptTemplate)
					assert.NotNil(t, tt.config.EmptyContextPromptTemplate)
				}
			}
		})
	}
}

func TestNewContextualAugmenter(t *testing.T) {
	tests := []struct {
		name    string
		config  *ContextualAugmenterConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name:    "valid config",
			config:  &ContextualAugmenterConfig{},
			wantErr: false,
		},
		{
			name: "valid config with custom settings",
			config: &ContextualAugmenterConfig{
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate("Context: {{.Context}}\nQuery: {{.Query}}"),
				EmptyContextPromptTemplate: chat.NewPromptTemplate().
					WithTemplate("No context"),
				AllowEmptyContext: true,
			},
			wantErr: false,
		},
		{
			name: "invalid config",
			config: &ContextualAugmenterConfig{
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate("Invalid template"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			augmenter, err := NewContextualAugmenter(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, augmenter)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, augmenter)
				assert.NotNil(t, augmenter.promptTemplate)
				assert.NotNil(t, augmenter.emptyContextPromptTemplate)
				assert.Implements(t, (*rag.QueryAugmenter)(nil), augmenter)
			}
		})
	}
}

func TestContextualAugmenter_Augment(t *testing.T) {
	tests := []struct {
		name           string
		config         *ContextualAugmenterConfig
		query          *rag.Query
		documents      []*document.Document
		wantErr        bool
		validateResult func(t *testing.T, result *rag.Query)
	}{
		{
			name:    "nil query",
			config:  &ContextualAugmenterConfig{},
			query:   nil,
			wantErr: true,
		},
		{
			name: "empty documents with AllowEmptyContext false",
			config: &ContextualAugmenterConfig{
				AllowEmptyContext: false,
			},
			query:     mustCreateQuery(t, "What is AI?"),
			documents: []*document.Document{},
			wantErr:   false,
			validateResult: func(t *testing.T, result *rag.Query) {
				assert.NotNil(t, result)
				assert.NotEqual(t, "What is AI?", result.Text)
			},
		},
		{
			name: "empty documents with AllowEmptyContext true",
			config: &ContextualAugmenterConfig{
				AllowEmptyContext: true,
			},
			query:     mustCreateQuery(t, "What is AI?"),
			documents: []*document.Document{},
			wantErr:   false,
			validateResult: func(t *testing.T, result *rag.Query) {
				assert.NotNil(t, result)
				assert.Equal(t, "What is AI?", result.Text)
			},
		},
		{
			name:   "single document",
			config: &ContextualAugmenterConfig{},
			query:  mustCreateQuery(t, "What is machine learning?"),
			documents: []*document.Document{
				createDoc(t, "doc1", "Machine learning is a subset of AI", 0.9),
			},
			wantErr: false,
			validateResult: func(t *testing.T, result *rag.Query) {
				assert.NotNil(t, result)
				assert.Contains(t, result.Text, "Machine learning is a subset of AI")
				assert.Contains(t, result.Text, "What is machine learning?")
			},
		},
		{
			name:   "multiple documents",
			config: &ContextualAugmenterConfig{},
			query:  mustCreateQuery(t, "Explain neural networks"),
			documents: []*document.Document{
				createDoc(t, "doc1", "Neural networks are computing systems", 0.9),
				createDoc(t, "doc2", "They are inspired by biological networks", 0.8),
				createDoc(t, "doc3", "Used for pattern recognition", 0.7),
			},
			wantErr: false,
			validateResult: func(t *testing.T, result *rag.Query) {
				assert.NotNil(t, result)
				assert.Contains(t, result.Text, "Neural networks are computing systems")
				assert.Contains(t, result.Text, "They are inspired by biological networks")
				assert.Contains(t, result.Text, "Used for pattern recognition")
				assert.Contains(t, result.Text, "Explain neural networks")
			},
		},
		{
			name: "custom template",
			config: &ContextualAugmenterConfig{
				PromptTemplate: chat.NewPromptTemplate().
					WithTemplate("CONTEXT:\n{{.Context}}\n\nQUESTION: {{.Query}}"),
			},
			query: mustCreateQuery(t, "What is deep learning?"),
			documents: []*document.Document{
				createDoc(t, "doc1", "Deep learning uses neural networks", 0.9),
			},
			wantErr: false,
			validateResult: func(t *testing.T, result *rag.Query) {
				assert.NotNil(t, result)
				assert.Contains(t, result.Text, "CONTEXT:")
				assert.Contains(t, result.Text, "QUESTION:")
				assert.Contains(t, result.Text, "Deep learning uses neural networks")
				assert.Contains(t, result.Text, "What is deep learning?")
			},
		},
		{
			name: "custom empty context template",
			config: &ContextualAugmenterConfig{
				AllowEmptyContext: false,
				EmptyContextPromptTemplate: chat.NewPromptTemplate().
					WithTemplate("Sorry, I cannot answer that question."),
			},
			query:     mustCreateQuery(t, "Random question"),
			documents: []*document.Document{},
			wantErr:   false,
			validateResult: func(t *testing.T, result *rag.Query) {
				assert.NotNil(t, result)
				assert.Contains(t, result.Text, "Sorry, I cannot answer that question.")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			augmenter, err := NewContextualAugmenter(tt.config)
			require.NoError(t, err)

			ctx := context.Background()
			result, err := augmenter.Augment(ctx, tt.query, tt.documents)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tt.validateResult != nil {
				tt.validateResult(t, result)
			}
		})
	}
}

func TestContextualAugmenter_handleEmptyContext(t *testing.T) {
	tests := []struct {
		name              string
		allowEmptyContext bool
		originalQuery     string
		validateResult    func(t *testing.T, original, result *rag.Query)
	}{
		{
			name:              "allow empty context returns original query",
			allowEmptyContext: true,
			originalQuery:     "Original query text",
			validateResult: func(t *testing.T, original, result *rag.Query) {
				assert.Equal(t, original.Text, result.Text)
			},
		},
		{
			name:              "disallow empty context returns template result",
			allowEmptyContext: false,
			originalQuery:     "Original query text",
			validateResult: func(t *testing.T, original, result *rag.Query) {
				assert.NotEqual(t, original.Text, result.Text)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &ContextualAugmenterConfig{
				AllowEmptyContext: tt.allowEmptyContext,
			}
			augmenter, err := NewContextualAugmenter(config)
			require.NoError(t, err)

			query := mustCreateQuery(t, tt.originalQuery)
			result, err := augmenter.handleEmptyContext(query)
			require.NoError(t, err)

			tt.validateResult(t, query, result)
		})
	}
}

func TestContextualAugmenter_Augment_DocumentFormatting(t *testing.T) {
	config := &ContextualAugmenterConfig{}
	augmenter, err := NewContextualAugmenter(config)
	require.NoError(t, err)

	query := mustCreateQuery(t, "Test query")
	documents := []*document.Document{
		createDoc(t, "doc1", "First document", 0.9),
		createDoc(t, "doc2", "Second document", 0.8),
	}

	ctx := context.Background()
	result, err := augmenter.Augment(ctx, query, documents)
	require.NoError(t, err)

	assert.Contains(t, result.Text, "First document")
	assert.Contains(t, result.Text, "Second document")
}

func mustCreateQuery(t *testing.T, text string) *rag.Query {
	query, err := rag.NewQuery(text)
	require.NoError(t, err)
	return query
}

func createDoc(t *testing.T, id, text string, score float64) *document.Document {
	doc, err := document.NewDocument(text, nil)
	require.NoError(t, err)
	doc.ID = id
	doc.Score = score
	return doc
}
