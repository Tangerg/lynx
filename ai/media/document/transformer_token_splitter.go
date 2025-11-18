package document

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/lynx/ai/tokenizer"
)

// TokenSplitterConfig holds the configuration for TokenSplitter.
type TokenSplitterConfig struct {
	// Tokenizer is used to encode text into tokens and decode tokens back to text.
	// Required. Must not be nil.
	// The tokenizer should match the model that will process the chunks
	// (e.g., use the same tokenizer as your embedding model).
	Tokenizer tokenizer.Tokenizer

	// ChunkSize specifies the target number of tokens per chunk.
	// Optional. Defaults to 800 tokens if not provided or <= 0.
	// This should be set based on your embedding model's token limit,
	// typically leaving some buffer for metadata and special tokens.
	ChunkSize int

	// MinChunkSize sets the minimum size in characters for attempting to split at punctuation.
	// Optional. Defaults to 350 characters if not provided or <= 0.
	// Chunks smaller than this may not split at sentence boundaries.
	// This helps ensure chunks are semantically meaningful.
	MinChunkSize int

	// MinEmbedLength specifies the minimum character length for a chunk to be included.
	// Optional. Defaults to 5 characters if not provided or <= 0.
	// Chunks shorter than this are filtered out as they typically don't
	// provide meaningful semantic information.
	MinEmbedLength int

	// MaxChunkCount limits the maximum number of chunks that can be created from a single document.
	// Optional. Defaults to 10000 if not provided or <= 0.
	// This prevents potential infinite loops or memory issues with extremely large documents.
	MaxChunkCount int

	// KeepSeparator determines whether to preserve newline characters in the output chunks.
	// Optional. Defaults to false.
	// If false, newlines are replaced with spaces for cleaner text.
	// If true, original line structure is maintained.
	KeepSeparator bool

	// CopyFormatter determines whether to copy the formatter from the original document
	// to each split chunk.
	// Optional. Defaults to false.
	// Set to true if you want split chunks to inherit the parent document's
	// formatting behavior.
	CopyFormatter bool
}

func (c *TokenSplitterConfig) validate() error {
	const (
		chunkSize      = 800
		minChunkSize   = 350
		minEmbedLength = 5
		maxChunkCount  = 10000
	)

	if c == nil {
		return errors.New("config is required")
	}
	if c.Tokenizer == nil {
		return errors.New("tokenizer is required")
	}
	if c.ChunkSize <= 0 {
		c.ChunkSize = chunkSize
	}
	if c.MinChunkSize <= 0 {
		c.MinChunkSize = minChunkSize
	}
	if c.MinEmbedLength <= 0 {
		c.MinEmbedLength = minEmbedLength
	}
	if c.MaxChunkCount <= 0 {
		c.MaxChunkCount = maxChunkCount
	}
	return nil
}

var _ Transformer = (*TokenSplitter)(nil)

// TokenSplitter transforms documents by splitting them into token-based chunks with
// intelligent boundary detection.
//
// This transformer is useful for:
//   - Creating chunks that respect embedding model token limits
//   - Splitting at natural sentence boundaries when possible
//   - Ensuring consistent chunk sizes for efficient batch processing
//   - Preventing token truncation during embedding generation
//
// The splitter uses a sophisticated algorithm that:
//  1. Splits text into token-based chunks of approximately ChunkSize tokens
//  2. Attempts to split at sentence boundaries (., ?, !, \n) when chunks exceed MinChunkSize
//  3. Filters out chunks shorter than MinEmbedLength to avoid meaningless embeddings
//  4. Limits total chunks per document to MaxChunkCount for safety
type TokenSplitter struct {
	tokenizer      tokenizer.Tokenizer
	chunkSize      int
	minChunkSize   int
	minEmbedLength int
	maxChunkCount  int
	keepSeparator  bool
	copyFormatter  bool
	splitter       *Splitter
}

func NewTokenSplitter(config *TokenSplitterConfig) (*TokenSplitter, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	ts := &TokenSplitter{
		tokenizer:      config.Tokenizer,
		chunkSize:      config.ChunkSize,
		minChunkSize:   config.MinChunkSize,
		minEmbedLength: config.MinEmbedLength,
		maxChunkCount:  config.MaxChunkCount,
		keepSeparator:  config.KeepSeparator,
		copyFormatter:  config.CopyFormatter,
	}
	ts.splitter, _ = NewSplitter(&SplitterConfig{
		CopyFormatter: config.CopyFormatter,
		SplitFunc:     ts.splitByTokens,
	})
	return ts, nil
}

func (t *TokenSplitter) splitByTokens(ctx context.Context, text string) ([]string, error) {
	if strings.TrimSpace(text) == "" {
		return []string{}, nil
	}

	tokens, err := t.tokenizer.Encode(ctx, text)
	if err != nil {
		return nil, err
	}

	textChunks := make([]string, 0, t.chunkSize)
	processedCount := 0

	for len(tokens) > 0 && processedCount < t.maxChunkCount {
		chunkEnd := min(t.chunkSize, len(tokens))
		currentTokens := tokens[:chunkEnd]

		chunkText, err := t.tokenizer.Decode(ctx, currentTokens)
		if err != nil {
			return nil, err
		}

		if strings.TrimSpace(chunkText) == "" {
			tokens = tokens[len(currentTokens):]
			continue
		}

		lastPunctuation := max(
			strings.LastIndex(chunkText, "."),
			max(strings.LastIndex(chunkText, "?"),
				max(strings.LastIndex(chunkText, "!"),
					strings.LastIndex(chunkText, "\n"))),
		)

		if lastPunctuation != -1 && lastPunctuation > t.minChunkSize {
			chunkText = chunkText[:lastPunctuation+1]
		}

		var finalChunk string
		if t.keepSeparator {
			finalChunk = strings.TrimSpace(chunkText)
		} else {
			finalChunk = strings.TrimSpace(
				strings.ReplaceAll(chunkText, "\n", " "),
			)
		}

		if len(finalChunk) > t.minEmbedLength {
			textChunks = append(textChunks, finalChunk)
		}

		actualTokens, err := t.tokenizer.Encode(ctx, chunkText)
		if err != nil {
			return nil, err
		}
		tokens = tokens[min(len(actualTokens), len(tokens)):]

		processedCount++
	}

	if len(tokens) > 0 {
		remainingText, err := t.tokenizer.Decode(ctx, tokens)
		if err != nil {
			return nil, err
		}

		cleanedText := strings.TrimSpace(
			strings.ReplaceAll(remainingText, "\n", " "),
		)

		if len(cleanedText) > t.minEmbedLength {
			textChunks = append(textChunks, cleanedText)
		}
	}

	return textChunks, nil
}

func (t *TokenSplitter) Transform(ctx context.Context, docs []*Document) ([]*Document, error) {
	return t.splitter.Transform(ctx, docs)
}
