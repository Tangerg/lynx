package rag

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/media/document"
)

func TestNewNop(t *testing.T) {
	t.Run("returns singleton instance", func(t *testing.T) {
		nop1 := NewNop()
		nop2 := NewNop()

		assert.NotNil(t, nop1)
		assert.NotNil(t, nop2)
		assert.Same(t, nop1, nop2, "NewNop should return the same singleton instance")
	})
}

func TestNop_Expand(t *testing.T) {
	ctx := context.Background()
	nop := NewNop()

	t.Run("returns query as single-element slice", func(t *testing.T) {
		query := &Query{Text: "test query"}

		result, err := nop.Expand(ctx, query)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result, 1)
		assert.Same(t, query, result[0], "should return the same query instance")
	})

	t.Run("handles nil context gracefully", func(t *testing.T) {
		query := &Query{Text: "test query"}

		result, err := nop.Expand(nil, query)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result, 1)
	})

	t.Run("handles empty query", func(t *testing.T) {
		query := &Query{}

		result, err := nop.Expand(ctx, query)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result, 1)
		assert.Same(t, query, result[0])
	})
}

func TestNop_Retrieve(t *testing.T) {
	ctx := context.Background()
	nop := NewNop()

	t.Run("returns empty document list", func(t *testing.T) {
		query := &Query{Text: "test query"}

		result, err := nop.Retrieve(ctx, query)

		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("handles nil context gracefully", func(t *testing.T) {
		query := &Query{Text: "test query"}

		result, err := nop.Retrieve(nil, query)

		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("handles nil query", func(t *testing.T) {
		result, err := nop.Retrieve(ctx, nil)

		require.NoError(t, err)
		assert.Nil(t, result)
	})
}

func TestNop_Transform(t *testing.T) {
	ctx := context.Background()
	nop := NewNop()

	t.Run("returns query without modification", func(t *testing.T) {
		query := &Query{Text: "test query"}

		result, err := nop.Transform(ctx, query)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Same(t, query, result, "should return the same query instance")
		assert.Equal(t, "test query", result.Text)
	})

	t.Run("handles nil context gracefully", func(t *testing.T) {
		query := &Query{Text: "test query"}

		result, err := nop.Transform(nil, query)

		require.NoError(t, err)
		assert.Same(t, query, result)
	})

	t.Run("handles empty query", func(t *testing.T) {
		query := &Query{}

		result, err := nop.Transform(ctx, query)

		require.NoError(t, err)
		assert.Same(t, query, result)
	})
}

func TestNop_Augment(t *testing.T) {
	ctx := context.Background()
	nop := NewNop()

	t.Run("returns query without augmentation", func(t *testing.T) {
		query := &Query{Text: "test query"}
		docs := []*document.Document{
			{Text: "doc1"},
			{Text: "doc2"},
		}

		result, err := nop.Augment(ctx, query, docs)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Same(t, query, result, "should return the same query instance")
		assert.Equal(t, "test query", result.Text)
	})

	t.Run("handles nil context gracefully", func(t *testing.T) {
		query := &Query{Text: "test query"}
		docs := []*document.Document{{Text: "doc1"}}

		result, err := nop.Augment(nil, query, docs)

		require.NoError(t, err)
		assert.Same(t, query, result)
	})

	t.Run("handles nil documents", func(t *testing.T) {
		query := &Query{Text: "test query"}

		result, err := nop.Augment(ctx, query, nil)

		require.NoError(t, err)
		assert.Same(t, query, result)
	})

	t.Run("handles empty document list", func(t *testing.T) {
		query := &Query{Text: "test query"}
		docs := []*document.Document{}

		result, err := nop.Augment(ctx, query, docs)

		require.NoError(t, err)
		assert.Same(t, query, result)
	})
}

func TestNop_Refine(t *testing.T) {
	ctx := context.Background()
	nop := NewNop()

	t.Run("returns documents without refinement", func(t *testing.T) {
		query := &Query{Text: "test query"}
		docs := []*document.Document{
			{Text: "doc1"},
			{Text: "doc2"},
			{Text: "doc3"},
		}

		result, err := nop.Refine(ctx, query, docs)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result, 3)
		assert.True(t, reflect.DeepEqual(docs, result), "should return the same document slice")
	})

	t.Run("handles nil context gracefully", func(t *testing.T) {
		query := &Query{Text: "test query"}
		docs := []*document.Document{{Text: "doc1"}}

		result, err := nop.Refine(nil, query, docs)

		require.NoError(t, err)
		assert.True(t, reflect.DeepEqual(docs, result))
	})

	t.Run("handles nil query", func(t *testing.T) {
		docs := []*document.Document{{Text: "doc1"}}

		result, err := nop.Refine(ctx, nil, docs)

		require.NoError(t, err)

		assert.True(t, reflect.DeepEqual(docs, result))
	})

	t.Run("handles nil documents", func(t *testing.T) {
		query := &Query{Text: "test query"}

		result, err := nop.Refine(ctx, query, nil)

		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("handles empty document list", func(t *testing.T) {
		query := &Query{Text: "test query"}
		docs := []*document.Document{}

		result, err := nop.Refine(ctx, query, docs)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result, 0)

		assert.True(t, reflect.DeepEqual(docs, result))
	})
}

func TestNop_InterfaceCompliance(t *testing.T) {
	t.Run("implements QueryExpander", func(t *testing.T) {
		var _ QueryExpander = (*Nop)(nil)
	})

	t.Run("implements QueryTransformer", func(t *testing.T) {
		var _ QueryTransformer = (*Nop)(nil)
	})

	t.Run("implements QueryAugmenter", func(t *testing.T) {
		var _ QueryAugmenter = (*Nop)(nil)
	})

	t.Run("implements DocumentRetriever", func(t *testing.T) {
		var _ DocumentRetriever = (*Nop)(nil)
	})

	t.Run("implements DocumentRefiner", func(t *testing.T) {
		var _ DocumentRefiner = (*Nop)(nil)
	})
}
