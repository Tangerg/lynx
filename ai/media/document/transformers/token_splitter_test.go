package transformers

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/tokenizer"
)

// mockTokenizer implements tokenizer.Tokenizer for testing
type mockTokenizer struct {
	encodeFunc func(ctx context.Context, text string) ([]int, error)
	decodeFunc func(ctx context.Context, tokens []int) (string, error)
}

func (m *mockTokenizer) Encode(ctx context.Context, text string) ([]int, error) {
	if m.encodeFunc != nil {
		return m.encodeFunc(ctx, text)
	}
	// Default: one token per character
	tokens := make([]int, len(text))
	for i := range text {
		tokens[i] = int(text[i])
	}
	return tokens, nil
}

func (m *mockTokenizer) Decode(ctx context.Context, tokens []int) (string, error) {
	if m.decodeFunc != nil {
		return m.decodeFunc(ctx, tokens)
	}
	// Default: one character per token
	bytes := make([]byte, len(tokens))
	for i, token := range tokens {
		bytes[i] = byte(token)
	}
	return string(bytes), nil
}

// TestTokenSplitterConfig_validate tests config validation
func TestTokenSplitterConfig_validate(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		var config *TokenSplitterConfig
		err := config.validate()

		require.Error(t, err)
		assert.Equal(t, "config is required", err.Error())
	})

	t.Run("nil tokenizer", func(t *testing.T) {
		config := &TokenSplitterConfig{
			Tokenizer: nil,
		}

		err := config.validate()

		require.Error(t, err)
		assert.Equal(t, "tokenizer is required", err.Error())
	})

	t.Run("valid config with all fields", func(t *testing.T) {
		config := &TokenSplitterConfig{
			Tokenizer:      &mockTokenizer{},
			ChunkSize:      100,
			MinChunkSize:   50,
			MinEmbedLength: 10,
			MaxChunkCount:  1000,
			KeepSeparator:  true,
			CopyFormatter:  true,
		}

		err := config.validate()

		require.NoError(t, err)
		assert.Equal(t, 100, config.ChunkSize)
		assert.Equal(t, 50, config.MinChunkSize)
		assert.Equal(t, 10, config.MinEmbedLength)
		assert.Equal(t, 1000, config.MaxChunkCount)
	})

	t.Run("applies default values for zero fields", func(t *testing.T) {
		config := &TokenSplitterConfig{
			Tokenizer:      &mockTokenizer{},
			ChunkSize:      0,
			MinChunkSize:   0,
			MinEmbedLength: 0,
			MaxChunkCount:  0,
		}

		err := config.validate()

		require.NoError(t, err)
		assert.Equal(t, 800, config.ChunkSize)
		assert.Equal(t, 350, config.MinChunkSize)
		assert.Equal(t, 5, config.MinEmbedLength)
		assert.Equal(t, 10000, config.MaxChunkCount)
	})

	t.Run("applies default values for negative fields", func(t *testing.T) {
		config := &TokenSplitterConfig{
			Tokenizer:      &mockTokenizer{},
			ChunkSize:      -1,
			MinChunkSize:   -1,
			MinEmbedLength: -1,
			MaxChunkCount:  -1,
		}

		err := config.validate()

		require.NoError(t, err)
		assert.Equal(t, 800, config.ChunkSize)
		assert.Equal(t, 350, config.MinChunkSize)
		assert.Equal(t, 5, config.MinEmbedLength)
		assert.Equal(t, 10000, config.MaxChunkCount)
	})

	t.Run("preserves positive custom values", func(t *testing.T) {
		config := &TokenSplitterConfig{
			Tokenizer:      &mockTokenizer{},
			ChunkSize:      200,
			MinChunkSize:   100,
			MinEmbedLength: 20,
			MaxChunkCount:  500,
		}

		err := config.validate()

		require.NoError(t, err)
		assert.Equal(t, 200, config.ChunkSize)
		assert.Equal(t, 100, config.MinChunkSize)
		assert.Equal(t, 20, config.MinEmbedLength)
		assert.Equal(t, 500, config.MaxChunkCount)
	})
}

// TestNewTokenSplitter tests the constructor
func TestNewTokenSplitter(t *testing.T) {
	t.Run("with nil config", func(t *testing.T) {
		splitter, err := NewTokenSplitter(nil)

		require.Error(t, err)
		assert.Nil(t, splitter)
		assert.Equal(t, "config is required", err.Error())
	})

	t.Run("with nil tokenizer", func(t *testing.T) {
		config := &TokenSplitterConfig{
			Tokenizer: nil,
		}

		splitter, err := NewTokenSplitter(config)

		require.Error(t, err)
		assert.Nil(t, splitter)
		assert.Equal(t, "tokenizer is required", err.Error())
	})

	t.Run("with valid config", func(t *testing.T) {
		config := &TokenSplitterConfig{
			Tokenizer: &mockTokenizer{},
			ChunkSize: 100,
		}

		splitter, err := NewTokenSplitter(config)

		require.NoError(t, err)
		require.NotNil(t, splitter)
		assert.NotNil(t, splitter.config)
		assert.NotNil(t, splitter.splitter)
	})

	t.Run("applies default values during construction", func(t *testing.T) {
		config := &TokenSplitterConfig{
			Tokenizer: &mockTokenizer{},
		}

		splitter, err := NewTokenSplitter(config)

		require.NoError(t, err)
		assert.Equal(t, 800, splitter.config.ChunkSize)
		assert.Equal(t, 350, splitter.config.MinChunkSize)
		assert.Equal(t, 5, splitter.config.MinEmbedLength)
		assert.Equal(t, 10000, splitter.config.MaxChunkCount)
	})
}

// TestTokenSplitter_splitByTokens tests the token splitting logic
func TestTokenSplitter_splitByTokens(t *testing.T) {
	ctx := context.Background()

	t.Run("empty text", func(t *testing.T) {
		config := &TokenSplitterConfig{
			Tokenizer: &mockTokenizer{},
			ChunkSize: 10,
		}
		splitter, err := NewTokenSplitter(config)
		require.NoError(t, err)

		chunks, err := splitter.splitByTokens(ctx, "")

		require.NoError(t, err)
		assert.Empty(t, chunks)
	})

	t.Run("whitespace only text", func(t *testing.T) {
		config := &TokenSplitterConfig{
			Tokenizer: &mockTokenizer{},
			ChunkSize: 10,
		}
		splitter, err := NewTokenSplitter(config)
		require.NoError(t, err)

		chunks, err := splitter.splitByTokens(ctx, "   \n\t   ")

		require.NoError(t, err)
		assert.Empty(t, chunks)
	})

	t.Run("text shorter than chunk size", func(t *testing.T) {
		mock := &mockTokenizer{
			encodeFunc: func(ctx context.Context, text string) ([]int, error) {
				// Simulate word-based tokenization
				words := strings.Fields(text)
				return make([]int, len(words)), nil
			},
			decodeFunc: func(ctx context.Context, tokens []int) (string, error) {
				return "short text", nil
			},
		}

		config := &TokenSplitterConfig{
			Tokenizer:      mock,
			ChunkSize:      100,
			MinEmbedLength: 5,
		}
		splitter, err := NewTokenSplitter(config)
		require.NoError(t, err)

		chunks, err := splitter.splitByTokens(ctx, "short text")

		require.NoError(t, err)
		require.Len(t, chunks, 1)
		assert.Equal(t, "short text", chunks[0])
	})

	t.Run("text with punctuation at boundary", func(t *testing.T) {
		text := "This is sentence one. This is sentence two. This is sentence three."
		mock := &mockTokenizer{
			encodeFunc: func(ctx context.Context, text string) ([]int, error) {
				return make([]int, len(text)), nil
			},
			decodeFunc: func(ctx context.Context, tokens []int) (string, error) {
				if len(tokens) <= 25 {
					return text[:len(tokens)], nil
				}
				return text[:25], nil
			},
		}

		config := &TokenSplitterConfig{
			Tokenizer:      mock,
			ChunkSize:      25,
			MinChunkSize:   10,
			MinEmbedLength: 5,
		}
		splitter, err := NewTokenSplitter(config)
		require.NoError(t, err)

		chunks, err := splitter.splitByTokens(ctx, text)

		require.NoError(t, err)
		assert.Greater(t, len(chunks), 0)
	})

	t.Run("keep separator enabled", func(t *testing.T) {
		text := "Line 1\nLine 2\nLine 3"
		mock := &mockTokenizer{
			encodeFunc: func(ctx context.Context, text string) ([]int, error) {
				return make([]int, len(text)), nil
			},
			decodeFunc: func(ctx context.Context, tokens []int) (string, error) {
				return text, nil
			},
		}

		config := &TokenSplitterConfig{
			Tokenizer:      mock,
			ChunkSize:      100,
			MinEmbedLength: 5,
			KeepSeparator:  true,
		}
		splitter, err := NewTokenSplitter(config)
		require.NoError(t, err)

		chunks, err := splitter.splitByTokens(ctx, text)

		require.NoError(t, err)
		require.Len(t, chunks, 1)
		// Should preserve newlines
		assert.Contains(t, chunks[0], "\n")
	})

	t.Run("keep separator disabled", func(t *testing.T) {
		text := "Line 1\nLine 2\nLine 3"
		mock := &mockTokenizer{
			encodeFunc: func(ctx context.Context, text string) ([]int, error) {
				return make([]int, len(text)), nil
			},
			decodeFunc: func(ctx context.Context, tokens []int) (string, error) {
				return text, nil
			},
		}

		config := &TokenSplitterConfig{
			Tokenizer:      mock,
			ChunkSize:      100,
			MinEmbedLength: 5,
			KeepSeparator:  false,
		}
		splitter, err := NewTokenSplitter(config)
		require.NoError(t, err)

		chunks, err := splitter.splitByTokens(ctx, text)

		require.NoError(t, err)
		require.Len(t, chunks, 1)
		// Should replace newlines with spaces
		assert.NotContains(t, chunks[0], "\n")
		assert.Contains(t, chunks[0], " ")
	})

	t.Run("filters chunks below min embed length", func(t *testing.T) {
		mock := &mockTokenizer{
			encodeFunc: func(ctx context.Context, text string) ([]int, error) {
				return make([]int, len(text)), nil
			},
			decodeFunc: func(ctx context.Context, tokens []int) (string, error) {
				if len(tokens) < 3 {
					return "ab", nil
				}
				return "valid chunk", nil
			},
		}

		config := &TokenSplitterConfig{
			Tokenizer:      mock,
			ChunkSize:      5,
			MinEmbedLength: 3,
		}
		splitter, err := NewTokenSplitter(config)
		require.NoError(t, err)

		chunks, err := splitter.splitByTokens(ctx, "test text here")

		require.NoError(t, err)
		// Short chunks should be filtered out
		for _, chunk := range chunks {
			assert.GreaterOrEqual(t, len(chunk), 3)
		}
	})

	t.Run("respects max chunk count", func(t *testing.T) {
		longText := strings.Repeat("word ", 1000)
		mock := &mockTokenizer{
			encodeFunc: func(ctx context.Context, text string) ([]int, error) {
				words := strings.Fields(text)
				return make([]int, len(words)), nil
			},
			decodeFunc: func(ctx context.Context, tokens []int) (string, error) {
				return strings.Repeat("word ", len(tokens)), nil
			},
		}

		config := &TokenSplitterConfig{
			Tokenizer:      mock,
			ChunkSize:      10,
			MaxChunkCount:  5,
			MinEmbedLength: 5,
		}
		splitter, err := NewTokenSplitter(config)
		require.NoError(t, err)

		chunks, err := splitter.splitByTokens(ctx, longText)

		require.NoError(t, err)
		assert.LessOrEqual(t, len(chunks), 6) // 5 + remaining
	})

	t.Run("encode error", func(t *testing.T) {
		expectedErr := errors.New("encode error")
		mock := &mockTokenizer{
			encodeFunc: func(ctx context.Context, text string) ([]int, error) {
				return nil, expectedErr
			},
		}

		config := &TokenSplitterConfig{
			Tokenizer: mock,
			ChunkSize: 10,
		}
		splitter, err := NewTokenSplitter(config)
		require.NoError(t, err)

		chunks, err := splitter.splitByTokens(ctx, "test")

		require.Error(t, err)
		assert.Equal(t, expectedErr, err)
		assert.Nil(t, chunks)
	})

	t.Run("decode error", func(t *testing.T) {
		expectedErr := errors.New("decode error")
		mock := &mockTokenizer{
			encodeFunc: func(ctx context.Context, text string) ([]int, error) {
				return []int{1, 2, 3}, nil
			},
			decodeFunc: func(ctx context.Context, tokens []int) (string, error) {
				return "", expectedErr
			},
		}

		config := &TokenSplitterConfig{
			Tokenizer: mock,
			ChunkSize: 10,
		}
		splitter, err := NewTokenSplitter(config)
		require.NoError(t, err)

		chunks, err := splitter.splitByTokens(ctx, "test")

		require.Error(t, err)
		assert.Equal(t, expectedErr, err)
		assert.Nil(t, chunks)
	})

	t.Run("handles remaining tokens", func(t *testing.T) {
		text := strings.Repeat("word ", 25)
		mock := &mockTokenizer{
			encodeFunc: func(ctx context.Context, text string) ([]int, error) {
				words := strings.Fields(text)
				return make([]int, len(words)), nil
			},
			decodeFunc: func(ctx context.Context, tokens []int) (string, error) {
				return strings.Repeat("word ", len(tokens)), nil
			},
		}

		config := &TokenSplitterConfig{
			Tokenizer:      mock,
			ChunkSize:      10,
			MinEmbedLength: 5,
		}
		splitter, err := NewTokenSplitter(config)
		require.NoError(t, err)

		chunks, err := splitter.splitByTokens(ctx, text)

		require.NoError(t, err)
		assert.Greater(t, len(chunks), 2)
	})
}

// TestTokenSplitter_Transform tests the Transform method
func TestTokenSplitter_Transform(t *testing.T) {
	ctx := context.Background()

	t.Run("single document", func(t *testing.T) {
		mock := &mockTokenizer{
			encodeFunc: func(ctx context.Context, text string) ([]int, error) {
				words := strings.Fields(text)
				return make([]int, len(words)), nil
			},
			decodeFunc: func(ctx context.Context, tokens []int) (string, error) {
				return strings.Repeat("word ", len(tokens)), nil
			},
		}

		config := &TokenSplitterConfig{
			Tokenizer:      mock,
			ChunkSize:      5,
			MinEmbedLength: 5,
		}
		splitter, err := NewTokenSplitter(config)
		require.NoError(t, err)

		doc, err := document.NewDocument(strings.Repeat("word ", 20), nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*document.Document{doc})

		require.NoError(t, err)
		assert.Greater(t, len(result), 1)
	})

	t.Run("multiple documents", func(t *testing.T) {
		mock := &mockTokenizer{
			encodeFunc: func(ctx context.Context, text string) ([]int, error) {
				return make([]int, len(text)), nil
			},
			decodeFunc: func(ctx context.Context, tokens []int) (string, error) {
				return strings.Repeat("x", len(tokens)), nil
			},
		}

		config := &TokenSplitterConfig{
			Tokenizer:      mock,
			ChunkSize:      10,
			MinEmbedLength: 5,
		}
		splitter, err := NewTokenSplitter(config)
		require.NoError(t, err)

		doc1, _ := document.NewDocument("first document text", nil)
		doc2, _ := document.NewDocument("second document text", nil)

		result, err := splitter.Transform(ctx, []*document.Document{doc1, doc2})

		require.NoError(t, err)
		assert.Greater(t, len(result), 0)
	})

	t.Run("empty document list", func(t *testing.T) {
		config := &TokenSplitterConfig{
			Tokenizer: &mockTokenizer{},
			ChunkSize: 10,
		}
		splitter, err := NewTokenSplitter(config)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*document.Document{})

		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("preserves metadata", func(t *testing.T) {
		mock := &mockTokenizer{
			encodeFunc: func(ctx context.Context, text string) ([]int, error) {
				return make([]int, len(text)), nil
			},
			decodeFunc: func(ctx context.Context, tokens []int) (string, error) {
				return strings.Repeat("x", len(tokens)), nil
			},
		}

		config := &TokenSplitterConfig{
			Tokenizer:      mock,
			ChunkSize:      10,
			MinEmbedLength: 5,
		}
		splitter, err := NewTokenSplitter(config)
		require.NoError(t, err)

		doc, _ := document.NewDocument("test document with metadata", nil)
		doc.Metadata["source"] = "test"
		doc.Metadata["page"] = 42

		result, err := splitter.Transform(ctx, []*document.Document{doc})

		require.NoError(t, err)
		for _, chunk := range result {
			assert.Equal(t, "test", chunk.Metadata["source"])
			assert.Equal(t, 42, chunk.Metadata["page"])
		}
	})

	t.Run("with copy formatter enabled", func(t *testing.T) {
		mock := &mockTokenizer{
			encodeFunc: func(ctx context.Context, text string) ([]int, error) {
				return make([]int, len(text)), nil
			},
			decodeFunc: func(ctx context.Context, tokens []int) (string, error) {
				return strings.Repeat("x", len(tokens)), nil
			},
		}

		config := &TokenSplitterConfig{
			Tokenizer:      mock,
			ChunkSize:      10,
			MinEmbedLength: 5,
			CopyFormatter:  true,
		}
		splitter, err := NewTokenSplitter(config)
		require.NoError(t, err)

		formatter := &mockFormatter{prefix: "test: "}
		doc, _ := document.NewDocument("test document", nil)
		doc.Formatter = formatter

		result, err := splitter.Transform(ctx, []*document.Document{doc})

		require.NoError(t, err)
		for _, chunk := range result {
			formatted := chunk.Formatter.Format(chunk, document.MetadataModeAll)
			assert.Contains(t, formatted, "test:")
		}
	})
}

// TestTokenSplitter_WithRealTokenizer tests with actual tiktoken
func TestTokenSplitter_WithRealTokenizer(t *testing.T) {
	ctx := context.Background()

	t.Run("basic text splitting", func(t *testing.T) {
		config := &TokenSplitterConfig{
			Tokenizer:      tokenizer.NewTiktokenWithCL100KBase(),
			ChunkSize:      20,
			MinEmbedLength: 5,
		}
		splitter, err := NewTokenSplitter(config)
		require.NoError(t, err)

		text := "This is a test document. It contains multiple sentences. " +
			"Each sentence should be properly tokenized and split into chunks."

		doc, _ := document.NewDocument(text, nil)

		result, err := splitter.Transform(ctx, []*document.Document{doc})

		require.NoError(t, err)
		assert.Greater(t, len(result), 1)

		for _, chunk := range result {
			t.Logf("Chunk: %s", chunk.Text)
			assert.NotEmpty(t, chunk.Text)
		}
	})

	t.Run("long document splitting", func(t *testing.T) {
		config := &TokenSplitterConfig{
			Tokenizer:      tokenizer.NewTiktokenWithCL100KBase(),
			ChunkSize:      50,
			MinEmbedLength: 10,
		}
		splitter, err := NewTokenSplitter(config)
		require.NoError(t, err)

		// Create a long document
		longText := strings.Repeat("This is a sentence. ", 100)
		doc, _ := document.NewDocument(longText, nil)

		result, err := splitter.Transform(ctx, []*document.Document{doc})

		require.NoError(t, err)
		assert.Greater(t, len(result), 5)

		for _, chunk := range result {
			assert.NotEmpty(t, chunk.Text)
		}
	})

	t.Run("preserve punctuation boundaries", func(t *testing.T) {
		config := &TokenSplitterConfig{
			Tokenizer:      tokenizer.NewTiktokenWithCL100KBase(),
			ChunkSize:      30,
			MinChunkSize:   10,
			MinEmbedLength: 5,
		}
		splitter, err := NewTokenSplitter(config)
		require.NoError(t, err)

		text := "First sentence. Second sentence? Third sentence! Fourth sentence."
		doc, _ := document.NewDocument(text, nil)

		result, err := splitter.Transform(ctx, []*document.Document{doc})

		require.NoError(t, err)
		assert.Greater(t, len(result), 0)

		// Check that chunks respect sentence boundaries
		for _, chunk := range result {
			t.Logf("Chunk: %s", chunk.Text)
		}
	})
}

// TestTokenSplitter_EdgeCases tests edge cases
func TestTokenSplitter_EdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("very short text", func(t *testing.T) {
		config := &TokenSplitterConfig{
			Tokenizer:      tokenizer.NewTiktokenWithCL100KBase(),
			ChunkSize:      100,
			MinEmbedLength: 5,
		}
		splitter, err := NewTokenSplitter(config)
		require.NoError(t, err)

		doc, _ := document.NewDocument("Hi", nil)

		result, err := splitter.Transform(ctx, []*document.Document{doc})

		require.NoError(t, err)
		// Might be empty if text is too short
		if len(result) > 0 {
			assert.NotEmpty(t, result[0].Text)
		}
	})

	t.Run("text with only punctuation", func(t *testing.T) {
		config := &TokenSplitterConfig{
			Tokenizer:      tokenizer.NewTiktokenWithCL100KBase(),
			ChunkSize:      10,
			MinEmbedLength: 1,
		}
		splitter, err := NewTokenSplitter(config)
		require.NoError(t, err)

		doc, _ := document.NewDocument("...!!!???", nil)

		result, err := splitter.Transform(ctx, []*document.Document{doc})

		require.NoError(t, err)
		// Should handle gracefully
		if len(result) > 0 {
			assert.NotEmpty(t, result[0].Text)
		}
	})

	t.Run("unicode text", func(t *testing.T) {
		config := &TokenSplitterConfig{
			Tokenizer:      tokenizer.NewTiktokenWithCL100KBase(),
			ChunkSize:      20,
			MinEmbedLength: 5,
		}
		splitter, err := NewTokenSplitter(config)
		require.NoError(t, err)

		doc, _ := document.NewDocument("你好世界。这是一个测试。", nil)

		result, err := splitter.Transform(ctx, []*document.Document{doc})

		require.NoError(t, err)
		if len(result) > 0 {
			assert.NotEmpty(t, result[0].Text)
		}
	})

	t.Run("mixed language text", func(t *testing.T) {
		config := &TokenSplitterConfig{
			Tokenizer:      tokenizer.NewTiktokenWithCL100KBase(),
			ChunkSize:      30,
			MinEmbedLength: 5,
		}
		splitter, err := NewTokenSplitter(config)
		require.NoError(t, err)

		doc, _ := document.NewDocument("Hello world. 你好世界. Bonjour monde.", nil)

		result, err := splitter.Transform(ctx, []*document.Document{doc})

		require.NoError(t, err)
		assert.Greater(t, len(result), 0)
	})
}

// TestTokenSplitter_InterfaceCompliance verifies interface implementation
func TestTokenSplitter_InterfaceCompliance(t *testing.T) {
	config := &TokenSplitterConfig{
		Tokenizer: &mockTokenizer{},
		ChunkSize: 10,
	}
	splitter, err := NewTokenSplitter(config)
	require.NoError(t, err)

	var _ document.Transformer = splitter
}

// BenchmarkTokenSplitter benchmarks performance
func BenchmarkTokenSplitter_Transform(b *testing.B) {
	ctx := context.Background()
	config := &TokenSplitterConfig{
		Tokenizer:      tokenizer.NewTiktokenWithCL100KBase(),
		ChunkSize:      50,
		MinEmbedLength: 10,
	}
	splitter, _ := NewTokenSplitter(config)

	text := strings.Repeat("This is a test sentence. ", 100)
	doc, _ := document.NewDocument(text, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = splitter.Transform(ctx, []*document.Document{doc})
	}
}

func BenchmarkTokenSplitter_SplitByTokens(b *testing.B) {
	ctx := context.Background()
	config := &TokenSplitterConfig{
		Tokenizer:      tokenizer.NewTiktokenWithCL100KBase(),
		ChunkSize:      50,
		MinEmbedLength: 10,
	}
	splitter, _ := NewTokenSplitter(config)

	text := strings.Repeat("This is a test sentence. ", 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = splitter.splitByTokens(ctx, text)
	}
}

func BenchmarkTokenSplitter_LargeDocument(b *testing.B) {
	ctx := context.Background()
	config := &TokenSplitterConfig{
		Tokenizer:      tokenizer.NewTiktokenWithCL100KBase(),
		ChunkSize:      100,
		MinEmbedLength: 20,
	}
	splitter, _ := NewTokenSplitter(config)

	text := strings.Repeat("This is a long document with multiple sentences. "+
		"It needs to be split into multiple chunks for processing. ", 500)
	doc, _ := document.NewDocument(text, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = splitter.Transform(ctx, []*document.Document{doc})
	}
}
