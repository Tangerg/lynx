package rag

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/media/document"
)

// Mock implementations for testing

type mockQueryTransformer struct {
	transformFunc func(ctx context.Context, query *Query) (*Query, error)
}

func (m *mockQueryTransformer) Transform(ctx context.Context, query *Query) (*Query, error) {
	if m.transformFunc != nil {
		return m.transformFunc(ctx, query)
	}
	cloned := query.Clone()
	cloned.Text = cloned.Text + " transformed"
	return cloned, nil
}

type mockQueryExpander struct {
	expandFunc func(ctx context.Context, query *Query) ([]*Query, error)
}

func (m *mockQueryExpander) Expand(ctx context.Context, query *Query) ([]*Query, error) {
	if m.expandFunc != nil {
		return m.expandFunc(ctx, query)
	}
	q1 := query.Clone()
	q1.Text = query.Text + " variant1"
	q2 := query.Clone()
	q2.Text = query.Text + " variant2"
	return []*Query{q1, q2}, nil
}

type mockDocumentRetriever struct {
	retrieveFunc func(ctx context.Context, query *Query) ([]*document.Document, error)
}

func (m *mockDocumentRetriever) Retrieve(ctx context.Context, query *Query) ([]*document.Document, error) {
	if m.retrieveFunc != nil {
		return m.retrieveFunc(ctx, query)
	}
	doc, _ := document.NewDocument("mock document for: "+query.Text, nil)
	return []*document.Document{doc}, nil
}

type mockDocumentRefiner struct {
	refineFunc func(ctx context.Context, query *Query, docs []*document.Document) ([]*document.Document, error)
}

func (m *mockDocumentRefiner) Refine(ctx context.Context, query *Query, docs []*document.Document) ([]*document.Document, error) {
	if m.refineFunc != nil {
		return m.refineFunc(ctx, query, docs)
	}
	// Filter to keep only half of the documents
	half := len(docs) / 2
	if half == 0 && len(docs) > 0 {
		half = 1
	}
	return docs[:half], nil
}

type mockQueryAugmenter struct {
	augmentFunc func(ctx context.Context, query *Query, docs []*document.Document) (*Query, error)
}

func (m *mockQueryAugmenter) Augment(ctx context.Context, query *Query, docs []*document.Document) (*Query, error) {
	if m.augmentFunc != nil {
		return m.augmentFunc(ctx, query, docs)
	}
	cloned := query.Clone()
	cloned.Text = cloned.Text + " augmented"
	return cloned, nil
}

func TestPipelineConfig_validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *PipelineConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "pipeline config cannot be nil",
		},
		{
			name: "no document retrievers",
			config: &PipelineConfig{
				DocumentRetrievers: []DocumentRetriever{},
			},
			wantErr: true,
			errMsg:  "at least one document retriever is required",
		},
		{
			name: "valid minimal config",
			config: &PipelineConfig{
				DocumentRetrievers: []DocumentRetriever{&mockDocumentRetriever{}},
			},
			wantErr: false,
		},
		{
			name: "valid full config",
			config: &PipelineConfig{
				QueryTransformers:  []QueryTransformer{&mockQueryTransformer{}},
				QueryExpander:      &mockQueryExpander{},
				DocumentRetrievers: []DocumentRetriever{&mockDocumentRetriever{}},
				DocumentRefiners:   []DocumentRefiner{&mockDocumentRefiner{}},
				QueryAugmenter:     &mockQueryAugmenter{},
			},
			wantErr: false,
		},
		{
			name: "nil query expander defaults to Nop",
			config: &PipelineConfig{
				DocumentRetrievers: []DocumentRetriever{&mockDocumentRetriever{}},
				QueryExpander:      nil,
			},
			wantErr: false,
		},
		{
			name: "nil query augmenter defaults to Nop",
			config: &PipelineConfig{
				DocumentRetrievers: []DocumentRetriever{&mockDocumentRetriever{}},
				QueryAugmenter:     nil,
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
					assert.NotNil(t, tt.config.QueryExpander)
					assert.NotNil(t, tt.config.QueryAugmenter)
				}
			}
		})
	}
}

func TestNewPipeline(t *testing.T) {
	tests := []struct {
		name    string
		config  *PipelineConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "invalid config",
			config: &PipelineConfig{
				DocumentRetrievers: []DocumentRetriever{},
			},
			wantErr: true,
		},
		{
			name: "valid config",
			config: &PipelineConfig{
				DocumentRetrievers: []DocumentRetriever{&mockDocumentRetriever{}},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline, err := NewPipeline(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, pipeline)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, pipeline)
				assert.NotNil(t, pipeline.queryExpander)
				assert.NotNil(t, pipeline.queryAugmenter)
			}
		})
	}
}

func TestPipeline_transformQuery(t *testing.T) {
	tests := []struct {
		name         string
		transformers []QueryTransformer
		query        *Query
		wantErr      bool
		validate     func(t *testing.T, result *Query)
	}{
		{
			name:         "no transformers",
			transformers: []QueryTransformer{},
			query:        mustCreateQuery(t, "test query"),
			wantErr:      false,
			validate: func(t *testing.T, result *Query) {
				assert.Equal(t, "test query", result.Text)
			},
		},
		{
			name:         "single transformer",
			transformers: []QueryTransformer{&mockQueryTransformer{}},
			query:        mustCreateQuery(t, "test"),
			wantErr:      false,
			validate: func(t *testing.T, result *Query) {
				assert.Equal(t, "test transformed", result.Text)
			},
		},
		{
			name: "multiple transformers",
			transformers: []QueryTransformer{
				&mockQueryTransformer{},
				&mockQueryTransformer{},
			},
			query:   mustCreateQuery(t, "test"),
			wantErr: false,
			validate: func(t *testing.T, result *Query) {
				assert.Equal(t, "test transformed transformed", result.Text)
			},
		},
		{
			name: "transformer returns error",
			transformers: []QueryTransformer{
				&mockQueryTransformer{
					transformFunc: func(ctx context.Context, query *Query) (*Query, error) {
						return nil, errors.New("transform error")
					},
				},
			},
			query:   mustCreateQuery(t, "test"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline := &Pipeline{
				queryTransformers: tt.transformers,
			}

			ctx := context.Background()
			result, err := pipeline.transformQuery(ctx, tt.query)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, result)
				}
			}
		})
	}
}

func TestPipeline_expandQuery(t *testing.T) {
	tests := []struct {
		name     string
		expander QueryExpander
		query    *Query
		wantErr  bool
		validate func(t *testing.T, results []*Query)
	}{
		{
			name:     "successful expansion",
			expander: &mockQueryExpander{},
			query:    mustCreateQuery(t, "test"),
			wantErr:  false,
			validate: func(t *testing.T, results []*Query) {
				assert.Len(t, results, 2)
				assert.Contains(t, results[0].Text, "variant1")
				assert.Contains(t, results[1].Text, "variant2")
			},
		},
		{
			name: "expander returns error",
			expander: &mockQueryExpander{
				expandFunc: func(ctx context.Context, query *Query) ([]*Query, error) {
					return nil, errors.New("expand error")
				},
			},
			query:   mustCreateQuery(t, "test"),
			wantErr: true,
		},
		{
			name:     "nop expander",
			expander: NewNop(),
			query:    mustCreateQuery(t, "test"),
			wantErr:  false,
			validate: func(t *testing.T, results []*Query) {
				assert.Len(t, results, 1)
				assert.Equal(t, "test", results[0].Text)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline := &Pipeline{
				queryExpander: tt.expander,
			}

			ctx := context.Background()
			results, err := pipeline.expandQuery(ctx, tt.query)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, results)
				}
			}
		})
	}
}

func TestPipeline_retrieveByQuery(t *testing.T) {
	tests := []struct {
		name       string
		retrievers []DocumentRetriever
		query      *Query
		wantErr    bool
		validate   func(t *testing.T, docs []*document.Document)
	}{
		{
			name:       "single retriever",
			retrievers: []DocumentRetriever{&mockDocumentRetriever{}},
			query:      mustCreateQuery(t, "test"),
			wantErr:    false,
			validate: func(t *testing.T, docs []*document.Document) {
				assert.Len(t, docs, 1)
			},
		},
		{
			name: "multiple retrievers",
			retrievers: []DocumentRetriever{
				&mockDocumentRetriever{},
				&mockDocumentRetriever{},
			},
			query:   mustCreateQuery(t, "test"),
			wantErr: false,
			validate: func(t *testing.T, docs []*document.Document) {
				assert.Len(t, docs, 2)
			},
		},
		{
			name: "one retriever fails",
			retrievers: []DocumentRetriever{
				&mockDocumentRetriever{},
				&mockDocumentRetriever{
					retrieveFunc: func(ctx context.Context, query *Query) ([]*document.Document, error) {
						return nil, errors.New("retrieve error")
					},
				},
			},
			query:   mustCreateQuery(t, "test"),
			wantErr: false, // Partial failure is acceptable
			validate: func(t *testing.T, docs []*document.Document) {
				assert.NotEmpty(t, docs)
			},
		},
		{
			name: "all retrievers fail",
			retrievers: []DocumentRetriever{
				&mockDocumentRetriever{
					retrieveFunc: func(ctx context.Context, query *Query) ([]*document.Document, error) {
						return nil, errors.New("retrieve error")
					},
				},
			},
			query:   mustCreateQuery(t, "test"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline := &Pipeline{
				documentRetrievers: tt.retrievers,
			}

			ctx := context.Background()
			docs, err := pipeline.retrieveByQuery(ctx, tt.query)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, docs)
				}
			}
		})
	}
}

func TestPipeline_retrieveByQueries(t *testing.T) {
	tests := []struct {
		name       string
		retrievers []DocumentRetriever
		queries    []*Query
		wantErr    bool
		validate   func(t *testing.T, docs []*document.Document)
	}{
		{
			name:       "single query",
			retrievers: []DocumentRetriever{&mockDocumentRetriever{}},
			queries:    []*Query{mustCreateQuery(t, "test1")},
			wantErr:    false,
			validate: func(t *testing.T, docs []*document.Document) {
				assert.Len(t, docs, 1)
			},
		},
		{
			name:       "multiple queries",
			retrievers: []DocumentRetriever{&mockDocumentRetriever{}},
			queries: []*Query{
				mustCreateQuery(t, "test1"),
				mustCreateQuery(t, "test2"),
			},
			wantErr: false,
			validate: func(t *testing.T, docs []*document.Document) {
				assert.Len(t, docs, 2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline := &Pipeline{
				documentRetrievers: tt.retrievers,
			}

			ctx := context.Background()
			docs, err := pipeline.retrieveByQueries(ctx, tt.queries)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, docs)
				}
			}
		})
	}
}

func TestPipeline_refineDocuments(t *testing.T) {
	tests := []struct {
		name     string
		refiners []DocumentRefiner
		query    *Query
		docs     []*document.Document
		wantErr  bool
		validate func(t *testing.T, docs []*document.Document)
	}{
		{
			name:     "no refiners",
			refiners: []DocumentRefiner{},
			query:    mustCreateQuery(t, "test"),
			docs:     createMockDocuments(4),
			wantErr:  false,
			validate: func(t *testing.T, docs []*document.Document) {
				assert.Len(t, docs, 4)
			},
		},
		{
			name:     "single refiner",
			refiners: []DocumentRefiner{&mockDocumentRefiner{}},
			query:    mustCreateQuery(t, "test"),
			docs:     createMockDocuments(4),
			wantErr:  false,
			validate: func(t *testing.T, docs []*document.Document) {
				assert.Len(t, docs, 2)
			},
		},
		{
			name: "refiner returns error",
			refiners: []DocumentRefiner{
				&mockDocumentRefiner{
					refineFunc: func(ctx context.Context, query *Query, docs []*document.Document) ([]*document.Document, error) {
						return nil, errors.New("refine error")
					},
				},
			},
			query:   mustCreateQuery(t, "test"),
			docs:    createMockDocuments(4),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline := &Pipeline{
				documentRefiners: tt.refiners,
			}

			ctx := context.Background()
			result, err := pipeline.refineDocuments(ctx, tt.query, tt.docs)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, result)
				}
			}
		})
	}
}

func TestPipeline_augmentQuery(t *testing.T) {
	tests := []struct {
		name      string
		augmenter QueryAugmenter
		query     *Query
		docs      []*document.Document
		wantErr   bool
		validate  func(t *testing.T, result *Query)
	}{
		{
			name:      "successful augmentation",
			augmenter: &mockQueryAugmenter{},
			query:     mustCreateQuery(t, "test"),
			docs:      createMockDocuments(2),
			wantErr:   false,
			validate: func(t *testing.T, result *Query) {
				assert.Contains(t, result.Text, "augmented")
			},
		},
		{
			name: "augmenter returns error",
			augmenter: &mockQueryAugmenter{
				augmentFunc: func(ctx context.Context, query *Query, docs []*document.Document) (*Query, error) {
					return nil, errors.New("augment error")
				},
			},
			query:   mustCreateQuery(t, "test"),
			docs:    createMockDocuments(2),
			wantErr: true,
		},
		{
			name:      "nop augmenter",
			augmenter: NewNop(),
			query:     mustCreateQuery(t, "test"),
			docs:      createMockDocuments(2),
			wantErr:   false,
			validate: func(t *testing.T, result *Query) {
				assert.Equal(t, "test", result.Text)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline := &Pipeline{
				queryAugmenter: tt.augmenter,
			}

			ctx := context.Background()
			result, err := pipeline.augmentQuery(ctx, tt.query, tt.docs)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, result)
				}
			}
		})
	}
}

func TestPipeline_Execute(t *testing.T) {
	tests := []struct {
		name     string
		config   *PipelineConfig
		query    *Query
		wantErr  bool
		validate func(t *testing.T, query *Query, docs []*document.Document)
	}{
		{
			name: "minimal pipeline",
			config: &PipelineConfig{
				DocumentRetrievers: []DocumentRetriever{&mockDocumentRetriever{}},
			},
			query:   mustCreateQuery(t, "test"),
			wantErr: false,
			validate: func(t *testing.T, query *Query, docs []*document.Document) {
				assert.NotNil(t, query)
				assert.NotEmpty(t, docs)
			},
		},
		{
			name: "full pipeline",
			config: &PipelineConfig{
				QueryTransformers:  []QueryTransformer{&mockQueryTransformer{}},
				QueryExpander:      &mockQueryExpander{},
				DocumentRetrievers: []DocumentRetriever{&mockDocumentRetriever{}},
				DocumentRefiners:   []DocumentRefiner{&mockDocumentRefiner{}},
				QueryAugmenter:     &mockQueryAugmenter{},
			},
			query:   mustCreateQuery(t, "test"),
			wantErr: false,
			validate: func(t *testing.T, query *Query, docs []*document.Document) {
				assert.NotNil(t, query)
				assert.Contains(t, query.Text, "augmented")
				assert.NotEmpty(t, docs)
			},
		},
		{
			name: "transformer fails",
			config: &PipelineConfig{
				QueryTransformers: []QueryTransformer{
					&mockQueryTransformer{
						transformFunc: func(ctx context.Context, query *Query) (*Query, error) {
							return nil, errors.New("transform error")
						},
					},
				},
				DocumentRetrievers: []DocumentRetriever{&mockDocumentRetriever{}},
			},
			query:   mustCreateQuery(t, "test"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline, err := NewPipeline(tt.config)
			require.NoError(t, err)

			ctx := context.Background()
			query, docs, err := pipeline.Execute(ctx, tt.query)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, query, docs)
				}
			}
		})
	}
}

func TestPipeline_Run(t *testing.T) {
	config := &PipelineConfig{
		DocumentRetrievers: []DocumentRetriever{&mockDocumentRetriever{}},
	}

	pipeline, err := NewPipeline(config)
	require.NoError(t, err)

	ctx := context.Background()
	query, docs, err := pipeline.Run(ctx, "test query")

	require.NoError(t, err)
	assert.NotNil(t, query)
	assert.Equal(t, "test query", query.Text)
	assert.NotEmpty(t, docs)
}

func TestPipeline_Run_InvalidQuery(t *testing.T) {
	config := &PipelineConfig{
		DocumentRetrievers: []DocumentRetriever{&mockDocumentRetriever{}},
	}

	pipeline, err := NewPipeline(config)
	require.NoError(t, err)

	ctx := context.Background()
	_, _, err = pipeline.Run(ctx, "")

	assert.Error(t, err)
}

// Helper functions

func mustCreateQuery(t *testing.T, text string) *Query {
	query, err := NewQuery(text)
	require.NoError(t, err)
	return query
}

func createMockDocuments(count int) []*document.Document {
	docs := make([]*document.Document, count)
	for i := 0; i < count; i++ {
		docs[i], _ = document.NewDocument("mock document", nil)
	}
	return docs
}
