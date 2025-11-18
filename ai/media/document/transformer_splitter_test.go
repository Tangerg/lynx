package document

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/media"
	"github.com/Tangerg/lynx/pkg/mime"
)

// TestSplitterConfig_validate tests config validation
func TestSplitterConfig_validate(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		var config *SplitterConfig
		err := config.validate()

		require.Error(t, err)
		assert.Equal(t, "config is required", err.Error())
	})

	t.Run("nil split func", func(t *testing.T) {
		config := &SplitterConfig{
			CopyFormatter: true,
			SplitFunc:     nil,
		}

		err := config.validate()

		require.Error(t, err)
		assert.Equal(t, "config split func is required", err.Error())
	})

	t.Run("valid config with split func", func(t *testing.T) {
		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				return []string{text}, nil
			},
		}

		err := config.validate()

		require.NoError(t, err)
	})

	t.Run("valid config with copy formatter", func(t *testing.T) {
		config := &SplitterConfig{
			CopyFormatter: true,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				return []string{text}, nil
			},
		}

		err := config.validate()

		require.NoError(t, err)
	})
}

// TestNewSplitter tests the constructor
func TestNewSplitter(t *testing.T) {
	t.Run("with nil config", func(t *testing.T) {
		splitter, err := NewSplitter(nil)

		require.Error(t, err)
		assert.Nil(t, splitter)
		assert.Equal(t, "config is required", err.Error())
	})

	t.Run("with nil split func", func(t *testing.T) {
		config := &SplitterConfig{
			CopyFormatter: true,
			SplitFunc:     nil,
		}

		splitter, err := NewSplitter(config)

		require.Error(t, err)
		assert.Nil(t, splitter)
		assert.Equal(t, "config split func is required", err.Error())
	})

	t.Run("with valid config", func(t *testing.T) {
		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				return []string{text}, nil
			},
		}

		splitter, err := NewSplitter(config)

		require.NoError(t, err)
		require.NotNil(t, splitter)
	})

	t.Run("with copy formatter enabled", func(t *testing.T) {
		config := &SplitterConfig{
			CopyFormatter: true,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				return []string{text}, nil
			},
		}

		splitter, err := NewSplitter(config)

		require.NoError(t, err)
		require.NotNil(t, splitter)
		assert.True(t, splitter.copyFormatter)
	})
}

// TestSplitter_splitSingleDocument tests splitting a single document
func TestSplitter_splitSingleDocument(t *testing.T) {
	ctx := context.Background()

	t.Run("split into multiple chunks", func(t *testing.T) {
		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				return strings.Split(text, " "), nil
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		doc, err := NewDocument("hello world test", nil)
		require.NoError(t, err)
		doc.Metadata["source"] = "test"

		chunks, err := splitter.splitSingleDocument(ctx, doc)

		require.NoError(t, err)
		require.Len(t, chunks, 3)
		assert.Equal(t, "hello", chunks[0].Text)
		assert.Equal(t, "world", chunks[1].Text)
		assert.Equal(t, "test", chunks[2].Text)

		// Check metadata is cloned
		for _, chunk := range chunks {
			assert.Equal(t, "test", chunk.Metadata["source"])
		}
	})

	t.Run("split with empty chunks filtered", func(t *testing.T) {
		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				return []string{"chunk1", "", "chunk2", "", "chunk3"}, nil
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		doc, err := NewDocument("test", nil)
		require.NoError(t, err)

		chunks, err := splitter.splitSingleDocument(ctx, doc)

		require.NoError(t, err)
		require.Len(t, chunks, 3)
		assert.Equal(t, "chunk1", chunks[0].Text)
		assert.Equal(t, "chunk2", chunks[1].Text)
		assert.Equal(t, "chunk3", chunks[2].Text)
	})

	t.Run("split with copy formatter enabled", func(t *testing.T) {
		formatter := &mockFormatter{prefix: "formatted: "}

		config := &SplitterConfig{
			CopyFormatter: true,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				return []string{"chunk1", "chunk2"}, nil
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		doc, err := NewDocument("test", nil)
		require.NoError(t, err)
		doc.Formatter = formatter

		chunks, err := splitter.splitSingleDocument(ctx, doc)

		require.NoError(t, err)
		require.Len(t, chunks, 2)

		// Check formatter is copied
		for _, chunk := range chunks {
			assert.NotNil(t, chunk.Formatter)
			formatted := chunk.Formatter.Format(chunk, MetadataModeAll)
			assert.Contains(t, formatted, "formatted:")
		}
	})

	t.Run("split without copy formatter", func(t *testing.T) {
		formatter := &mockFormatter{prefix: "formatted: "}

		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				return []string{"chunk1", "chunk2"}, nil
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		doc, err := NewDocument("test", nil)
		require.NoError(t, err)
		doc.Formatter = formatter

		chunks, err := splitter.splitSingleDocument(ctx, doc)

		require.NoError(t, err)
		require.Len(t, chunks, 2)

		// Check formatter is not copied (uses default Nop formatter)
		for _, chunk := range chunks {
			assert.NotNil(t, chunk.Formatter) // Default formatter exists
			// Should not be the same formatter
			formatted := chunk.Formatter.Format(chunk, MetadataModeAll)
			assert.NotContains(t, formatted, "formatted:")
		}
	})

	t.Run("split func returns error", func(t *testing.T) {
		expectedErr := errors.New("split error")
		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				return nil, expectedErr
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		doc, err := NewDocument("test", nil)
		require.NoError(t, err)

		chunks, err := splitter.splitSingleDocument(ctx, doc)

		require.Error(t, err)
		assert.Equal(t, expectedErr, err)
		assert.Nil(t, chunks)
	})

	t.Run("split returns empty array", func(t *testing.T) {
		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				return []string{}, nil
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		doc, err := NewDocument("test", nil)
		require.NoError(t, err)

		chunks, err := splitter.splitSingleDocument(ctx, doc)

		require.NoError(t, err)
		assert.Empty(t, chunks)
	})

	t.Run("split returns only empty strings", func(t *testing.T) {
		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				return []string{"", "", ""}, nil
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		doc, err := NewDocument("test", nil)
		require.NoError(t, err)

		chunks, err := splitter.splitSingleDocument(ctx, doc)

		require.NoError(t, err)
		assert.Empty(t, chunks)
	})

	t.Run("metadata is cloned not shared", func(t *testing.T) {
		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				return []string{"chunk1", "chunk2"}, nil
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		doc, err := NewDocument("test", nil)
		require.NoError(t, err)
		doc.Metadata["key"] = "value"

		chunks, err := splitter.splitSingleDocument(ctx, doc)

		require.NoError(t, err)
		require.Len(t, chunks, 2)

		// Modify first chunk's metadata
		chunks[0].Metadata["key"] = "modified"

		// Check second chunk's metadata is not affected
		assert.Equal(t, "value", chunks[1].Metadata["key"])
		// Check original document's metadata is not affected
		assert.Equal(t, "value", doc.Metadata["key"])
	})

	t.Run("empty metadata handling", func(t *testing.T) {
		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				return []string{"chunk1", "chunk2"}, nil
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		doc, err := NewDocument("test", nil)
		require.NoError(t, err)

		chunks, err := splitter.splitSingleDocument(ctx, doc)

		require.NoError(t, err)
		require.Len(t, chunks, 2)

		// Should handle empty metadata gracefully
		for _, chunk := range chunks {
			assert.NotNil(t, chunk.Metadata)
		}
	})

	t.Run("document with media", func(t *testing.T) {
		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				return []string{"chunk1", "chunk2"}, nil
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		testMedia := &media.Media{
			MimeType: mime.MustNew("text", "plain"),
		}
		doc, err := NewDocument("test", testMedia)
		require.NoError(t, err)

		chunks, err := splitter.splitSingleDocument(ctx, doc)

		require.NoError(t, err)
		require.Len(t, chunks, 2)

		// Media should not be copied to chunks (only text is split)
		for _, chunk := range chunks {
			assert.Nil(t, chunk.Media)
		}
	})
}

// TestSplitter_Transform tests the Transform method
func TestSplitter_Transform(t *testing.T) {
	ctx := context.Background()

	t.Run("transform single document", func(t *testing.T) {
		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				return strings.Split(text, " "), nil
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		doc, err := NewDocument("hello world", nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		require.Len(t, result, 2)
		assert.Equal(t, "hello", result[0].Text)
		assert.Equal(t, "world", result[1].Text)
	})

	t.Run("transform multiple documents", func(t *testing.T) {
		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				return strings.Split(text, " "), nil
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		doc1, err := NewDocument("hello world", nil)
		require.NoError(t, err)

		doc2, err := NewDocument("foo bar", nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc1, doc2})

		require.NoError(t, err)
		require.Len(t, result, 4)
		assert.Equal(t, "hello", result[0].Text)
		assert.Equal(t, "world", result[1].Text)
		assert.Equal(t, "foo", result[2].Text)
		assert.Equal(t, "bar", result[3].Text)
	})

	t.Run("transform empty document list", func(t *testing.T) {
		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				return strings.Split(text, " "), nil
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{})

		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("transform with split error", func(t *testing.T) {
		expectedErr := errors.New("split error")
		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				return nil, expectedErr
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		doc, err := NewDocument("test", nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.Error(t, err)
		assert.Equal(t, expectedErr, err)
		assert.Nil(t, result)
	})

	t.Run("transform stops on first error", func(t *testing.T) {
		callCount := 0
		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				callCount++
				if callCount == 2 {
					return nil, errors.New("second doc error")
				}
				return []string{text}, nil
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		doc1, err := NewDocument("first", nil)
		require.NoError(t, err)

		doc2, err := NewDocument("second", nil)
		require.NoError(t, err)

		doc3, err := NewDocument("third", nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc1, doc2, doc3})

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, 2, callCount)
	})

	t.Run("transform with metadata preservation", func(t *testing.T) {
		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				return strings.Split(text, " "), nil
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		doc1, err := NewDocument("hello world", nil)
		require.NoError(t, err)
		doc1.Metadata["source"] = "doc1"

		doc2, err := NewDocument("foo bar", nil)
		require.NoError(t, err)
		doc2.Metadata["source"] = "doc2"

		result, err := splitter.Transform(ctx, []*Document{doc1, doc2})

		require.NoError(t, err)
		require.Len(t, result, 4)

		// Check metadata preservation
		assert.Equal(t, "doc1", result[0].Metadata["source"])
		assert.Equal(t, "doc1", result[1].Metadata["source"])
		assert.Equal(t, "doc2", result[2].Metadata["source"])
		assert.Equal(t, "doc2", result[3].Metadata["source"])
	})

	t.Run("transform with formatter copying", func(t *testing.T) {
		formatter1 := &mockFormatter{prefix: "fmt1: "}
		formatter2 := &mockFormatter{prefix: "fmt2: "}

		config := &SplitterConfig{
			CopyFormatter: true,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				return strings.Split(text, " "), nil
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		doc1, err := NewDocument("hello world", nil)
		require.NoError(t, err)
		doc1.Formatter = formatter1

		doc2, err := NewDocument("foo bar", nil)
		require.NoError(t, err)
		doc2.Formatter = formatter2

		result, err := splitter.Transform(ctx, []*Document{doc1, doc2})

		require.NoError(t, err)
		require.Len(t, result, 4)

		// Check formatter copying
		formatted := result[0].Formatter.Format(result[0], MetadataModeAll)
		assert.Contains(t, formatted, "fmt1:")

		formatted = result[2].Formatter.Format(result[2], MetadataModeAll)
		assert.Contains(t, formatted, "fmt2:")
	})

	t.Run("transform with context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
					return strings.Split(text, " "), nil
				}
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		doc, err := NewDocument("hello world", nil)
		require.NoError(t, err)

		// Cancel context before transform
		cancel()

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.Error(t, err)
		assert.Equal(t, context.Canceled, err)
		assert.Nil(t, result)
	})

	t.Run("transform documents with media", func(t *testing.T) {
		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				return strings.Split(text, " "), nil
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		testMedia := &media.Media{
			MimeType: mime.MustNew("text", "plain"),
		}
		doc, err := NewDocument("hello world", testMedia)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		require.Len(t, result, 2)

		// Media should not be in chunks
		for _, chunk := range result {
			assert.Nil(t, chunk.Media)
		}
	})
}

// TestSplitter_InterfaceCompliance verifies interface implementation
func TestSplitter_InterfaceCompliance(t *testing.T) {
	config := &SplitterConfig{
		CopyFormatter: false,
		SplitFunc: func(ctx context.Context, text string) ([]string, error) {
			return []string{text}, nil
		},
	}

	splitter, err := NewSplitter(config)
	require.NoError(t, err)

	var _ Transformer = splitter
}

// TestSplitter_EdgeCases tests edge cases
func TestSplitter_EdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("very long text split", func(t *testing.T) {
		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				chunks := make([]string, 0)
				for i := 0; i < len(text); i += 100 {
					end := i + 100
					if end > len(text) {
						end = len(text)
					}
					chunks = append(chunks, text[i:end])
				}
				return chunks, nil
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		longText := strings.Repeat("a", 1000)
		doc, err := NewDocument(longText, nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		assert.Len(t, result, 10)
	})

	t.Run("unicode text split", func(t *testing.T) {
		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				return strings.Split(text, " "), nil
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		doc, err := NewDocument("你好 世界 🌍", nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		require.Len(t, result, 3)
		assert.Equal(t, "你好", result[0].Text)
		assert.Equal(t, "世界", result[1].Text)
		assert.Equal(t, "🌍", result[2].Text)
	})

	t.Run("split with newlines", func(t *testing.T) {
		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				return strings.Split(text, "\n"), nil
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		doc, err := NewDocument("line1\nline2\nline3", nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		require.Len(t, result, 3)
		assert.Equal(t, "line1", result[0].Text)
		assert.Equal(t, "line2", result[1].Text)
		assert.Equal(t, "line3", result[2].Text)
	})

	t.Run("complex metadata types", func(t *testing.T) {
		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				return []string{"chunk1", "chunk2"}, nil
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		doc, err := NewDocument("test", nil)
		require.NoError(t, err)
		doc.Metadata = map[string]any{
			"string": "value",
			"int":    42,
			"float":  3.14,
			"bool":   true,
			"slice":  []string{"a", "b"},
			"map":    map[string]string{"key": "val"},
			"nested": map[string]any{"deep": "value"},
		}

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		require.Len(t, result, 2)

		// Check all metadata types are preserved
		for _, chunk := range result {
			assert.Equal(t, "value", chunk.Metadata["string"])
			assert.Equal(t, 42, chunk.Metadata["int"])
			assert.Equal(t, 3.14, chunk.Metadata["float"])
			assert.Equal(t, true, chunk.Metadata["bool"])
			assert.NotNil(t, chunk.Metadata["slice"])
			assert.NotNil(t, chunk.Metadata["map"])
			assert.NotNil(t, chunk.Metadata["nested"])
		}
	})

	t.Run("document with ID and Score", func(t *testing.T) {
		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				return []string{"chunk1", "chunk2"}, nil
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		doc, err := NewDocument("test", nil)
		require.NoError(t, err)
		doc.ID = "doc-123"
		doc.Score = 0.95

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		require.Len(t, result, 2)

		// ID and Score are not copied to chunks
		for _, chunk := range result {
			assert.Empty(t, chunk.ID)
			assert.Equal(t, 0.0, chunk.Score)
		}
	})
}

// TestSplitter_Integration tests complete workflows
func TestSplitter_Integration(t *testing.T) {
	ctx := context.Background()

	t.Run("sentence splitter workflow", func(t *testing.T) {
		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				sentences := strings.Split(text, ". ")
				for i := range sentences {
					if i < len(sentences)-1 {
						sentences[i] += "."
					}
				}
				return sentences, nil
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		doc, err := NewDocument(
			"First sentence. Second sentence. Third sentence.",
			nil,
		)
		require.NoError(t, err)
		doc.Metadata["source"] = "article"

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		require.Len(t, result, 3)
		assert.Equal(t, "First sentence.", result[0].Text)
		assert.Equal(t, "Second sentence.", result[1].Text)
		assert.Equal(t, "Third sentence.", result[2].Text)

		for _, chunk := range result {
			assert.Equal(t, "article", chunk.Metadata["source"])
		}
	})

	t.Run("paragraph splitter workflow", func(t *testing.T) {
		config := &SplitterConfig{
			CopyFormatter: true,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				paragraphs := strings.Split(text, "\n\n")
				result := make([]string, 0, len(paragraphs))
				for _, p := range paragraphs {
					p = strings.TrimSpace(p)
					if p != "" {
						result = append(result, p)
					}
				}
				return result, nil
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		text := `Paragraph one.
More text.

Paragraph two.
More text.

Paragraph three.`

		doc, err := NewDocument(text, nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		require.Len(t, result, 3)
		assert.Contains(t, result[0].Text, "Paragraph one")
		assert.Contains(t, result[1].Text, "Paragraph two")
		assert.Contains(t, result[2].Text, "Paragraph three")
	})

	t.Run("fixed size chunker workflow", func(t *testing.T) {
		chunkSize := 50
		config := &SplitterConfig{
			CopyFormatter: false,
			SplitFunc: func(ctx context.Context, text string) ([]string, error) {
				chunks := make([]string, 0)
				for i := 0; i < len(text); i += chunkSize {
					end := i + chunkSize
					if end > len(text) {
						end = len(text)
					}
					chunks = append(chunks, text[i:end])
				}
				return chunks, nil
			},
		}

		splitter, err := NewSplitter(config)
		require.NoError(t, err)

		longText := strings.Repeat("This is a test. ", 50)
		doc, err := NewDocument(longText, nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		assert.Greater(t, len(result), 10)

		for i := 0; i < len(result)-1; i++ {
			assert.Equal(t, chunkSize, len(result[i].Text))
		}
	})
}

// BenchmarkSplitter benchmarks splitter performance
func BenchmarkSplitter_TransformSmall(b *testing.B) {
	ctx := context.Background()
	config := &SplitterConfig{
		CopyFormatter: false,
		SplitFunc: func(ctx context.Context, text string) ([]string, error) {
			return strings.Split(text, " "), nil
		},
	}

	splitter, _ := NewSplitter(config)
	doc, _ := NewDocument("hello world test", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = splitter.Transform(ctx, []*Document{doc})
	}
}

func BenchmarkSplitter_TransformLarge(b *testing.B) {
	ctx := context.Background()
	config := &SplitterConfig{
		CopyFormatter: false,
		SplitFunc: func(ctx context.Context, text string) ([]string, error) {
			return strings.Split(text, " "), nil
		},
	}

	splitter, _ := NewSplitter(config)
	longText := strings.Repeat("word ", 1000)
	doc, _ := NewDocument(longText, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = splitter.Transform(ctx, []*Document{doc})
	}
}

func BenchmarkSplitter_TransformMultipleDocs(b *testing.B) {
	ctx := context.Background()
	config := &SplitterConfig{
		CopyFormatter: false,
		SplitFunc: func(ctx context.Context, text string) ([]string, error) {
			return strings.Split(text, " "), nil
		},
	}

	splitter, _ := NewSplitter(config)
	docs := make([]*Document, 100)
	for i := range docs {
		docs[i], _ = NewDocument("hello world test", nil)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = splitter.Transform(ctx, docs)
	}
}

func BenchmarkSplitter_WithFormatterCopy(b *testing.B) {
	ctx := context.Background()
	config := &SplitterConfig{
		CopyFormatter: true,
		SplitFunc: func(ctx context.Context, text string) ([]string, error) {
			return strings.Split(text, " "), nil
		},
	}

	splitter, _ := NewSplitter(config)
	doc, _ := NewDocument("hello world test", nil)
	doc.Formatter = &mockFormatter{prefix: "test: "}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = splitter.Transform(ctx, []*Document{doc})
	}
}
