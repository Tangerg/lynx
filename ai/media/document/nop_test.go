package document

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMetadataMode tests the MetadataMode constants
func TestMetadataMode(t *testing.T) {
	tests := []struct {
		name     string
		mode     MetadataMode
		expected string
	}{
		{
			name:     "metadata mode all",
			mode:     MetadataModeAll,
			expected: "all",
		},
		{
			name:     "metadata mode embed",
			mode:     MetadataModeEmbed,
			expected: "embed",
		},
		{
			name:     "metadata mode inference",
			mode:     MetadataModeInference,
			expected: "inference",
		},
		{
			name:     "metadata mode none",
			mode:     MetadataModeNone,
			expected: "none",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.mode))
		})
	}
}

// TestNewNop tests the NewNop constructor
func TestNewNop(t *testing.T) {
	t.Run("returns singleton instance", func(t *testing.T) {
		nop1 := NewNop()
		nop2 := NewNop()

		require.NotNil(t, nop1)
		require.NotNil(t, nop2)
		// Verify it's the same instance (singleton pattern)
		assert.Same(t, nop1, nop2)
	})

	t.Run("returns non-nil instance", func(t *testing.T) {
		nop := NewNop()
		assert.NotNil(t, nop)
	})
}

// TestNop_InterfaceCompliance verifies Nop implements all required interfaces
func TestNop_InterfaceCompliance(t *testing.T) {
	nop := NewNop()

	t.Run("implements Reader", func(t *testing.T) {
		var _ Reader = nop
	})

	t.Run("implements Writer", func(t *testing.T) {
		var _ Writer = nop
	})

	t.Run("implements Transformer", func(t *testing.T) {
		var _ Transformer = nop
	})

	t.Run("implements Formatter", func(t *testing.T) {
		var _ Formatter = nop
	})

	t.Run("implements Batcher", func(t *testing.T) {
		var _ Batcher = nop
	})
}

// TestNop_Read tests the Read method
func TestNop_Read(t *testing.T) {
	nop := NewNop()

	tests := []struct {
		name string
		ctx  context.Context
	}{
		{
			name: "with background context",
			ctx:  context.Background(),
		},
		{
			name: "with todo context",
			ctx:  context.TODO(),
		},
		{
			name: "with timeout context",
			ctx: func() context.Context {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				return ctx
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docs, err := nop.Read(tt.ctx)

			require.NoError(t, err)
			assert.Nil(t, docs)
		})
	}
}

// TestNop_Write tests the Write method
func TestNop_Write(t *testing.T) {
	nop := NewNop()

	tests := []struct {
		name string
		ctx  context.Context
		docs []*Document
	}{
		{
			name: "nil documents",
			ctx:  context.Background(),
			docs: nil,
		},
		{
			name: "empty documents",
			ctx:  context.Background(),
			docs: []*Document{},
		},
		{
			name: "single document",
			ctx:  context.Background(),
			docs: []*Document{
				{
					ID:   "doc1",
					Text: "test content",
				},
			},
		},
		{
			name: "multiple documents",
			ctx:  context.Background(),
			docs: []*Document{
				{ID: "doc1", Text: "content 1"},
				{ID: "doc2", Text: "content 2"},
				{ID: "doc3", Text: "content 3"},
			},
		},
		{
			name: "with cancelled context",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			docs: []*Document{{ID: "doc1"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := nop.Write(tt.ctx, tt.docs)
			require.NoError(t, err)
		})
	}
}

// TestNop_Format tests the Format method
func TestNop_Format(t *testing.T) {
	nop := NewNop()

	tests := []struct {
		name     string
		doc      *Document
		mode     MetadataMode
		expected string
	}{
		{
			name: "simple document with mode all",
			doc: &Document{
				ID:   "doc1",
				Text: "hello world",
			},
			mode:     MetadataModeAll,
			expected: "hello world",
		},
		{
			name: "simple document with mode embed",
			doc: &Document{
				ID:   "doc2",
				Text: "test content",
			},
			mode:     MetadataModeEmbed,
			expected: "test content",
		},
		{
			name: "simple document with mode inference",
			doc: &Document{
				ID:   "doc3",
				Text: "inference text",
			},
			mode:     MetadataModeInference,
			expected: "inference text",
		},
		{
			name: "simple document with mode none",
			doc: &Document{
				ID:   "doc4",
				Text: "plain text",
			},
			mode:     MetadataModeNone,
			expected: "plain text",
		},
		{
			name: "document with metadata ignored",
			doc: &Document{
				ID:   "doc5",
				Text: "main text",
				Metadata: map[string]any{
					"author": "test",
					"date":   "2025-01-01",
				},
			},
			mode:     MetadataModeAll,
			expected: "main text",
		},
		{
			name: "empty text document",
			doc: &Document{
				ID:   "doc6",
				Text: "",
			},
			mode:     MetadataModeAll,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := nop.Format(tt.doc, tt.mode)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestNop_Transform tests the Transform method
func TestNop_Transform(t *testing.T) {
	nop := NewNop()

	t.Run("nil documents", func(t *testing.T) {
		ctx := context.Background()
		var docs []*Document = nil

		result, err := nop.Transform(ctx, docs)

		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("empty documents", func(t *testing.T) {
		ctx := context.Background()
		docs := []*Document{}

		result, err := nop.Transform(ctx, docs)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result, 0)
		// Verify same slice reference
		assert.True(t, reflect.DeepEqual(result, docs))
	})

	t.Run("single document unchanged", func(t *testing.T) {
		ctx := context.Background()
		docs := []*Document{
			{ID: "doc1", Text: "content 1"},
		}

		result, err := nop.Transform(ctx, docs)

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Len(t, result, 1)
		// Verify same slice reference
		assert.True(t, reflect.DeepEqual(result, docs))
		// Verify same document reference
		assert.Same(t, docs[0], result[0])
	})

	t.Run("multiple documents unchanged", func(t *testing.T) {
		ctx := context.Background()
		docs := []*Document{
			{ID: "doc1", Text: "content 1"},
			{ID: "doc2", Text: "content 2"},
			{ID: "doc3", Text: "content 3"},
		}

		result, err := nop.Transform(ctx, docs)

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Len(t, result, 3)
		// Verify same slice reference
		assert.True(t, reflect.DeepEqual(result, docs))
		// Verify same document references
		for i := range docs {
			assert.Same(t, docs[i], result[i])
		}
	})

	t.Run("documents with metadata preserved", func(t *testing.T) {
		ctx := context.Background()
		docs := []*Document{
			{
				ID:   "doc1",
				Text: "text",
				Metadata: map[string]any{
					"key": "value",
				},
			},
		}

		result, err := nop.Transform(ctx, docs)

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Len(t, result, 1)
		// Verify same slice reference
		assert.True(t, reflect.DeepEqual(result, docs))
		// Verify same document and metadata
		assert.Same(t, docs[0], result[0])
		assert.Equal(t, "value", result[0].Metadata["key"])
	})
}

// TestNop_Batch tests the Batch method
func TestNop_Batch(t *testing.T) {
	nop := NewNop()

	t.Run("nil documents", func(t *testing.T) {
		ctx := context.Background()
		var docs []*Document = nil

		result, err := nop.Batch(ctx, docs)

		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Nil(t, result[0])
	})

	t.Run("empty documents", func(t *testing.T) {
		ctx := context.Background()
		docs := []*Document{}

		result, err := nop.Batch(ctx, docs)

		require.NoError(t, err)
		require.Len(t, result, 1)
		require.NotNil(t, result[0])
		assert.Len(t, result[0], 0)
		// Verify same slice reference in batch
		assert.True(t, reflect.DeepEqual(result[0], docs))
	})

	t.Run("single document in one batch", func(t *testing.T) {
		ctx := context.Background()
		docs := []*Document{
			{ID: "doc1", Text: "content 1"},
		}

		result, err := nop.Batch(ctx, docs)

		require.NoError(t, err)
		require.Len(t, result, 1, "should return single batch")
		require.NotNil(t, result[0])
		require.Len(t, result[0], 1)
		// Verify same slice reference
		assert.True(t, reflect.DeepEqual(result[0], docs))
		// Verify same document reference
		assert.Same(t, docs[0], result[0][0])
	})

	t.Run("multiple documents in one batch", func(t *testing.T) {
		ctx := context.Background()
		docs := []*Document{
			{ID: "doc1", Text: "content 1"},
			{ID: "doc2", Text: "content 2"},
			{ID: "doc3", Text: "content 3"},
		}

		result, err := nop.Batch(ctx, docs)

		require.NoError(t, err)
		require.Len(t, result, 1, "should return single batch")
		require.NotNil(t, result[0])
		require.Len(t, result[0], 3)
		// Verify same slice reference
		assert.True(t, reflect.DeepEqual(result[0], docs))
		// Verify same document references
		for i := range docs {
			assert.Same(t, docs[i], result[0][i])
		}
	})

	t.Run("large number of documents in one batch", func(t *testing.T) {
		ctx := context.Background()
		docs := make([]*Document, 100)
		for i := 0; i < 100; i++ {
			docs[i] = &Document{
				ID:   string(rune(i)),
				Text: "content",
			}
		}

		result, err := nop.Batch(ctx, docs)

		require.NoError(t, err)
		require.Len(t, result, 1, "should return single batch")
		require.NotNil(t, result[0])
		require.Len(t, result[0], 100)
		// Verify same slice reference
		assert.True(t, reflect.DeepEqual(result[0], docs))
		// Spot check some document references
		assert.Same(t, docs[0], result[0][0])
		assert.Same(t, docs[50], result[0][50])
		assert.Same(t, docs[99], result[0][99])
	})
}

// TestNop_ContextCancellation tests behavior with cancelled contexts
func TestNop_ContextCancellation(t *testing.T) {
	nop := NewNop()

	t.Run("Read with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		docs, err := nop.Read(ctx)
		require.NoError(t, err)
		assert.Nil(t, docs)
	})

	t.Run("Write with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := nop.Write(ctx, []*Document{{ID: "doc1"}})
		require.NoError(t, err)
	})

	t.Run("Transform with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		docs := []*Document{{ID: "doc1"}}
		result, err := nop.Transform(ctx, docs)

		require.NoError(t, err)
		// Verify same slice reference
		assert.True(t, reflect.DeepEqual(result, docs))
	})

	t.Run("Batch with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		docs := []*Document{{ID: "doc1"}}
		result, err := nop.Batch(ctx, docs)

		require.NoError(t, err)
		require.Len(t, result, 1)
		// Verify same slice reference in batch
		assert.True(t, reflect.DeepEqual(result[0], docs))
	})
}

// TestNop_ConcurrentAccess tests thread-safety of singleton
func TestNop_ConcurrentAccess(t *testing.T) {
	const goroutines = 100
	done := make(chan *Nop, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			done <- NewNop()
		}()
	}

	first := <-done
	for i := 1; i < goroutines; i++ {
		instance := <-done
		assert.Same(t, first, instance, "All instances should be the same")
	}
}

// TestNop_Integration tests complete workflow
func TestNop_Integration(t *testing.T) {
	t.Run("complete pipeline workflow", func(t *testing.T) {
		nop := NewNop()
		ctx := context.Background()

		// Read (returns nil)
		readDocs, err := nop.Read(ctx)
		require.NoError(t, err)
		assert.Nil(t, readDocs)

		// Create test documents
		docs := []*Document{
			{ID: "doc1", Text: "content 1"},
			{ID: "doc2", Text: "content 2"},
		}

		// Transform (returns same slice)
		transformed, err := nop.Transform(ctx, docs)
		require.NoError(t, err)
		assert.True(t, reflect.DeepEqual(docs, transformed))

		// Batch (returns single batch with same slice)
		batches, err := nop.Batch(ctx, transformed)
		require.NoError(t, err)
		require.Len(t, batches, 1)
		assert.True(t, reflect.DeepEqual(batches[0], transformed))

		// Format (returns text only)
		formatted := nop.Format(docs[0], MetadataModeAll)
		assert.Equal(t, "content 1", formatted)

		// Write (succeeds silently)
		err = nop.Write(ctx, docs)
		require.NoError(t, err)
	})
}
