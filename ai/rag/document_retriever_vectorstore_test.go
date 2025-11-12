package rag

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/vectorstore"
	"github.com/Tangerg/lynx/ai/vectorstore/filter"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
)

type MockVectorStore struct {
	mock.Mock
}

func (m *MockVectorStore) Retrieve(ctx context.Context, request *vectorstore.RetrievalRequest) ([]*document.Document, error) {
	args := m.Called(ctx, request)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*document.Document), args.Error(1)
}

func TestVectorStoreRetrieverConfig_validate(t *testing.T) {
	mockStore := new(MockVectorStore)

	tests := []struct {
		name    string
		config  *VectorStoreDocumentRetrieverConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "vector store retriever config cannot be nil",
		},
		{
			name: "nil vector store",
			config: &VectorStoreDocumentRetrieverConfig{
				VectorStore: nil,
			},
			wantErr: true,
			errMsg:  "vector store is required",
		},
		{
			name: "negative topK",
			config: &VectorStoreDocumentRetrieverConfig{
				VectorStore: mockStore,
				TopK:        -5,
			},
			wantErr: true,
			errMsg:  "top k must be positive",
		},
		{
			name: "zero topK defaults to DefaultTopK",
			config: &VectorStoreDocumentRetrieverConfig{
				VectorStore: mockStore,
				TopK:        0,
			},
			wantErr: false,
		},
		{
			name: "minScore below minimum",
			config: &VectorStoreDocumentRetrieverConfig{
				VectorStore: mockStore,
				TopK:        5,
				MinScore:    -0.1,
			},
			wantErr: true,
			errMsg:  "min score must be between min and max similarity score",
		},
		{
			name: "minScore above maximum",
			config: &VectorStoreDocumentRetrieverConfig{
				VectorStore: mockStore,
				TopK:        5,
				MinScore:    1.1,
			},
			wantErr: true,
			errMsg:  "min score must be between min and max similarity score",
		},
		{
			name: "valid config",
			config: &VectorStoreDocumentRetrieverConfig{
				VectorStore: mockStore,
				TopK:        10,
				MinScore:    0.7,
			},
			wantErr: false,
		},
		{
			name: "valid config with filter func",
			config: &VectorStoreDocumentRetrieverConfig{
				VectorStore: mockStore,
				TopK:        5,
				MinScore:    0.5,
				FilterFunc: func(ctx context.Context, params map[string]any) (ast.Expr, error) {
					return filter.EQ("field", "value"), nil
				},
			},
			wantErr: false,
		},
		{
			name: "minScore at minimum boundary",
			config: &VectorStoreDocumentRetrieverConfig{
				VectorStore: mockStore,
				TopK:        5,
				MinScore:    vectorstore.MinSimilarityScore,
			},
			wantErr: false,
		},
		{
			name: "minScore at maximum boundary",
			config: &VectorStoreDocumentRetrieverConfig{
				VectorStore: mockStore,
				TopK:        5,
				MinScore:    vectorstore.MaxSimilarityScore,
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
				if tt.config != nil && tt.config.TopK == 0 {
					assert.Equal(t, vectorstore.DefaultTopK, tt.config.TopK)
				}
			}
		})
	}
}

func TestNewVectorStoreRetriever(t *testing.T) {
	mockStore := new(MockVectorStore)

	tests := []struct {
		name    string
		config  *VectorStoreDocumentRetrieverConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "valid config",
			config: &VectorStoreDocumentRetrieverConfig{
				VectorStore: mockStore,
				TopK:        5,
				MinScore:    0.7,
			},
			wantErr: false,
		},
		{
			name: "invalid config",
			config: &VectorStoreDocumentRetrieverConfig{
				VectorStore: nil,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retriever, err := NewVectorStoreDocumentRetriever(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, retriever)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, retriever)
				assert.Implements(t, (*DocumentRetriever)(nil), retriever)
				assert.Equal(t, tt.config.VectorStore, retriever.vectorStore)
				assert.Equal(t, tt.config.TopK, retriever.topK)
				assert.Equal(t, tt.config.MinScore, retriever.minScore)
			}
		})
	}
}

func TestVectorStoreRetriever_Retrieve(t *testing.T) {
	tests := []struct {
		name           string
		config         *VectorStoreDocumentRetrieverConfig
		query          *Query
		setupMock      func(*MockVectorStore)
		wantErr        bool
		validateResult func(t *testing.T, result []*document.Document)
	}{
		{
			name: "nil query",
			config: &VectorStoreDocumentRetrieverConfig{
				VectorStore: new(MockVectorStore),
				TopK:        5,
			},
			query:   nil,
			wantErr: true,
		},
		{
			name: "successful retrieval",
			config: &VectorStoreDocumentRetrieverConfig{
				VectorStore: new(MockVectorStore),
				TopK:        3,
				MinScore:    0.7,
			},
			query: mustCreateQuery(t, "test query"),
			setupMock: func(m *MockVectorStore) {
				expectedDocs := []*document.Document{
					createDoc(t, "doc1", "Content 1", 0.9),
					createDoc(t, "doc2", "Content 2", 0.8),
				}
				m.On("Retrieve", mock.Anything, mock.MatchedBy(func(req *vectorstore.RetrievalRequest) bool {
					return req.Query == "test query" && req.TopK == 3 && req.MinScore == 0.7
				})).Return(expectedDocs, nil)
			},
			wantErr: false,
			validateResult: func(t *testing.T, result []*document.Document) {
				assert.Len(t, result, 2)
				assert.Equal(t, "doc1", result[0].ID)
				assert.Equal(t, "doc2", result[1].ID)
			},
		},
		{
			name: "vector store returns error",
			config: &VectorStoreDocumentRetrieverConfig{
				VectorStore: new(MockVectorStore),
				TopK:        5,
			},
			query: mustCreateQuery(t, "test query"),
			setupMock: func(m *MockVectorStore) {
				m.On("Retrieve", mock.Anything, mock.Anything).
					Return(nil, errors.New("vector store error"))
			},
			wantErr: true,
		},
		{
			name: "empty results",
			config: &VectorStoreDocumentRetrieverConfig{
				VectorStore: new(MockVectorStore),
				TopK:        5,
			},
			query: mustCreateQuery(t, "test query"),
			setupMock: func(m *MockVectorStore) {
				m.On("Retrieve", mock.Anything, mock.Anything).
					Return([]*document.Document{}, nil)
			},
			wantErr: false,
			validateResult: func(t *testing.T, result []*document.Document) {
				assert.Empty(t, result)
			},
		},
		{
			name: "with filter expression from query",
			config: &VectorStoreDocumentRetrieverConfig{
				VectorStore: new(MockVectorStore),
				TopK:        5,
			},
			query: mustCreateQueryWithFilter(t, "test query", filter.EQ("category", "tech")),
			setupMock: func(m *MockVectorStore) {
				expectedDocs := []*document.Document{
					createDoc(t, "doc1", "Tech content", 0.9),
				}
				m.On("Retrieve", mock.Anything, mock.MatchedBy(func(req *vectorstore.RetrievalRequest) bool {
					return req.Filter != nil
				})).Return(expectedDocs, nil)
			},
			wantErr: false,
			validateResult: func(t *testing.T, result []*document.Document) {
				assert.Len(t, result, 1)
			},
		},
		{
			name: "with filter string from query",
			config: &VectorStoreDocumentRetrieverConfig{
				VectorStore: new(MockVectorStore),
				TopK:        5,
			},
			query: mustCreateQueryWithFilterString(t, "test query", "category == 'tech'"),
			setupMock: func(m *MockVectorStore) {
				expectedDocs := []*document.Document{
					createDoc(t, "doc1", "Tech content", 0.9),
				}
				m.On("Retrieve", mock.Anything, mock.MatchedBy(func(req *vectorstore.RetrievalRequest) bool {
					return req.Filter != nil
				})).Return(expectedDocs, nil)
			},
			wantErr: false,
			validateResult: func(t *testing.T, result []*document.Document) {
				assert.Len(t, result, 1)
			},
		},
		{
			name: "with filter func",
			config: &VectorStoreDocumentRetrieverConfig{
				VectorStore: new(MockVectorStore),
				TopK:        5,
				FilterFunc: func(ctx context.Context, params map[string]any) (ast.Expr, error) {
					return filter.EQ("status", "active"), nil
				},
			},
			query: mustCreateQuery(t, "test query"),
			setupMock: func(m *MockVectorStore) {
				expectedDocs := []*document.Document{
					createDoc(t, "doc1", "Active content", 0.9),
				}
				m.On("Retrieve", mock.Anything, mock.MatchedBy(func(req *vectorstore.RetrievalRequest) bool {
					return req.Filter != nil
				})).Return(expectedDocs, nil)
			},
			wantErr: false,
			validateResult: func(t *testing.T, result []*document.Document) {
				assert.Len(t, result, 1)
			},
		},
		{
			name: "filter func returns error",
			config: &VectorStoreDocumentRetrieverConfig{
				VectorStore: new(MockVectorStore),
				TopK:        5,
				FilterFunc: func(ctx context.Context, params map[string]any) (ast.Expr, error) {
					return nil, errors.New("filter func error")
				},
			},
			query:   mustCreateQuery(t, "test query"),
			wantErr: true,
		},
		{
			name: "invalid filter string in query",
			config: &VectorStoreDocumentRetrieverConfig{
				VectorStore: new(MockVectorStore),
				TopK:        5,
			},
			query:   mustCreateQueryWithFilterString(t, "test query", "invalid filter syntax"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := tt.config.VectorStore.(*MockVectorStore)
			if tt.setupMock != nil {
				tt.setupMock(mockStore)
			}

			retriever, err := NewVectorStoreDocumentRetriever(tt.config)
			require.NoError(t, err)

			ctx := context.Background()
			result, err := retriever.Retrieve(ctx, tt.query)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tt.validateResult != nil {
				tt.validateResult(t, result)
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestVectorStoreRetriever_buildFilterExpression(t *testing.T) {
	tests := []struct {
		name           string
		config         *VectorStoreDocumentRetrieverConfig
		query          *Query
		wantErr        bool
		validateResult func(t *testing.T, result ast.Expr)
	}{
		{
			name: "no filter",
			config: &VectorStoreDocumentRetrieverConfig{
				VectorStore: new(MockVectorStore),
			},
			query:   mustCreateQuery(t, "test query"),
			wantErr: false,
			validateResult: func(t *testing.T, result ast.Expr) {
				assert.Nil(t, result)
			},
		},
		{
			name: "filter expr from query",
			config: &VectorStoreDocumentRetrieverConfig{
				VectorStore: new(MockVectorStore),
			},
			query:   mustCreateQueryWithFilter(t, "test query", filter.EQ("field", "value")),
			wantErr: false,
			validateResult: func(t *testing.T, result ast.Expr) {
				assert.NotNil(t, result)
			},
		},
		{
			name: "filter string from query",
			config: &VectorStoreDocumentRetrieverConfig{
				VectorStore: new(MockVectorStore),
			},
			query:   mustCreateQueryWithFilterString(t, "test query", "field == 'value'"),
			wantErr: false,
			validateResult: func(t *testing.T, result ast.Expr) {
				assert.NotNil(t, result)
			},
		},
		{
			name: "filter func",
			config: &VectorStoreDocumentRetrieverConfig{
				VectorStore: new(MockVectorStore),
				FilterFunc: func(ctx context.Context, params map[string]any) (ast.Expr, error) {
					return filter.EQ("dynamic", "field"), nil
				},
			},
			query:   mustCreateQuery(t, "test query"),
			wantErr: false,
			validateResult: func(t *testing.T, result ast.Expr) {
				assert.NotNil(t, result)
			},
		},
		{
			name: "query filter takes precedence over filter func",
			config: &VectorStoreDocumentRetrieverConfig{
				VectorStore: new(MockVectorStore),
				FilterFunc: func(ctx context.Context, params map[string]any) (ast.Expr, error) {
					return filter.EQ("func", "filter"), nil
				},
			},
			query:   mustCreateQueryWithFilter(t, "test query", filter.EQ("query", "filter")),
			wantErr: false,
			validateResult: func(t *testing.T, result ast.Expr) {
				assert.NotNil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retriever, err := NewVectorStoreDocumentRetriever(tt.config)
			require.NoError(t, err)

			ctx := context.Background()
			result, err := retriever.buildFilterExpression(ctx, tt.query)

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
