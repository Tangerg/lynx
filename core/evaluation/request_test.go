package evaluation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/media/document"
)

func TestRequest(t *testing.T) {
	t.Run("create valid request", func(t *testing.T) {
		doc, err := document.NewDocument("test content", nil)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "What is AI?",
			Generation: "AI is artificial intelligence",
			Documents:  []*document.Document{doc},
		}

		assert.Equal(t, "What is AI?", req.Prompt)
		assert.Equal(t, "AI is artificial intelligence", req.Generation)
		assert.Len(t, req.Documents, 1)
	})

	t.Run("create request with empty documents", func(t *testing.T) {
		req := &Request{
			Prompt:     "test prompt",
			Generation: "test generation",
			Documents:  []*document.Document{},
		}

		assert.Empty(t, req.Documents)
	})

	t.Run("create request with nil documents", func(t *testing.T) {
		req := &Request{
			Prompt:     "test prompt",
			Generation: "test generation",
			Documents:  nil,
		}

		assert.Nil(t, req.Documents)
	})
}

func TestExtractDocuments(t *testing.T) {
	t.Run("extract single document", func(t *testing.T) {
		doc, err := document.NewDocument("This is document content", nil)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "test",
			Generation: "test",
			Documents:  []*document.Document{doc},
		}

		result := extractDocuments(req)
		assert.Equal(t, "This is document content", result)
	})

	t.Run("extract multiple documents", func(t *testing.T) {
		doc1, err := document.NewDocument("First document", nil)
		require.NoError(t, err)

		doc2, err := document.NewDocument("Second document", nil)
		require.NoError(t, err)

		doc3, err := document.NewDocument("Third document", nil)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "test",
			Generation: "test",
			Documents:  []*document.Document{doc1, doc2, doc3},
		}

		result := extractDocuments(req)
		expected := "First document\nSecond document\nThird document"
		assert.Equal(t, expected, result)
	})

	t.Run("extract with empty text documents", func(t *testing.T) {
		doc1, err := document.NewDocument("Valid content", nil)
		require.NoError(t, err)

		doc2, err := document.NewDocument("placeholder", nil)
		require.NoError(t, err)
		doc2.Text = ""

		doc3, err := document.NewDocument("Another valid content", nil)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "test",
			Generation: "test",
			Documents:  []*document.Document{doc1, doc2, doc3},
		}

		result := extractDocuments(req)
		expected := "Valid content\nAnother valid content"
		assert.Equal(t, expected, result)
	})

	t.Run("extract with all empty documents", func(t *testing.T) {
		doc1, err := document.NewDocument("placeholder1", nil)
		require.NoError(t, err)
		doc1.Text = ""

		doc2, err := document.NewDocument("placeholder2", nil)
		require.NoError(t, err)
		doc2.Text = ""

		req := &Request{
			Prompt:     "test",
			Generation: "test",
			Documents:  []*document.Document{doc1, doc2},
		}

		result := extractDocuments(req)
		assert.Equal(t, "", result)
	})

	t.Run("extract with nil documents slice", func(t *testing.T) {
		req := &Request{
			Prompt:     "test",
			Generation: "test",
			Documents:  nil,
		}

		result := extractDocuments(req)
		assert.Equal(t, "", result)
	})

	t.Run("extract with empty documents slice", func(t *testing.T) {
		req := &Request{
			Prompt:     "test",
			Generation: "test",
			Documents:  []*document.Document{},
		}

		result := extractDocuments(req)
		assert.Equal(t, "", result)
	})

	t.Run("extract with mixed content", func(t *testing.T) {
		doc1, err := document.NewDocument("First line", nil)
		require.NoError(t, err)

		doc2, err := document.NewDocument("placeholder", nil)
		require.NoError(t, err)
		doc2.Text = ""

		doc3, err := document.NewDocument("Second line", nil)
		require.NoError(t, err)

		doc4, err := document.NewDocument("placeholder", nil)
		require.NoError(t, err)
		doc4.Text = ""

		doc5, err := document.NewDocument("Third line", nil)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "test",
			Generation: "test",
			Documents:  []*document.Document{doc1, doc2, doc3, doc4, doc5},
		}

		result := extractDocuments(req)
		expected := "First line\nSecond line\nThird line"
		assert.Equal(t, expected, result)
	})
}
