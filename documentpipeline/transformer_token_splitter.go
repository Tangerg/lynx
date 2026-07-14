package documentpipeline

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/documentpipeline/id"
	"github.com/Tangerg/lynx/tokenizer"
)

// Default sizing for [TokenSplitter]. The numbers come from common
// embedding-model token limits and a reasonable upper bound on chunks
// per document — non-positive values fall back to these.
const (
	defaultTokenChunkSize      = 800
	defaultTokenMinChunkSize   = 350
	defaultTokenMinEmbedLength = 5
	defaultTokenMaxChunkCount  = 10_000
)

type TokenSplitterConfig struct {
	Tokenizer tokenizer.Tokenizer

	ChunkSize      int
	MinChunkSize   int
	MinEmbedLength int
	MaxChunkCount  int
	KeepSeparator  bool
	IDGenerator    id.Generator
}

func (c *TokenSplitterConfig) Validate() error {
	if c.Tokenizer == nil {
		return errors.New("documentpipeline.TokenSplitterConfig: Tokenizer is required")
	}
	return nil
}

func (c *TokenSplitterConfig) ApplyDefaults() {
	if c.ChunkSize <= 0 {
		c.ChunkSize = defaultTokenChunkSize
	}
	if c.MinChunkSize <= 0 {
		c.MinChunkSize = defaultTokenMinChunkSize
	}
	if c.MinEmbedLength <= 0 {
		c.MinEmbedLength = defaultTokenMinEmbedLength
	}
	if c.MaxChunkCount <= 0 {
		c.MaxChunkCount = defaultTokenMaxChunkCount
	}
}

var _ Transformer = (*TokenSplitter)(nil)

// TokenSplitter is a token-aware [Transformer] that splits documents
// into chunks bounded by the configured token count, preferring
// sentence-boundary cuts when possible. See [TokenSplitter.splitByTokens]
// for the per-chunk algorithm.
type TokenSplitter struct {
	tokenizer      tokenizer.Tokenizer
	chunkSize      int
	minChunkSize   int
	minEmbedLength int
	maxChunkCount  int
	keepSeparator  bool
	splitter       *Splitter
}

func NewTokenSplitter(config TokenSplitterConfig) (*TokenSplitter, error) {
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	ts := &TokenSplitter{
		tokenizer:      config.Tokenizer,
		chunkSize:      config.ChunkSize,
		minChunkSize:   config.MinChunkSize,
		minEmbedLength: config.MinEmbedLength,
		maxChunkCount:  config.MaxChunkCount,
		keepSeparator:  config.KeepSeparator,
	}
	ts.splitter, _ = NewSplitter(SplitterConfig{
		SplitFunc:   ts.splitByTokens,
		IDGenerator: config.IDGenerator,
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

	var chunks []string
	processed := 0

	for len(tokens) > 0 && processed < t.maxChunkCount {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := min(t.chunkSize, len(tokens))
		windowTokens := tokens[:end]

		windowText, err := t.tokenizer.Decode(ctx, windowTokens)
		if err != nil {
			return nil, err
		}

		if strings.TrimSpace(windowText) == "" {
			tokens = tokens[end:]
			continue
		}

		lastPunct := lastSentenceEnd(windowText)
		if lastPunct != -1 && lastPunct > t.minChunkSize {
			windowText = windowText[:lastPunct+1]
		}

		final := t.cleanChunk(windowText)
		if len(final) > t.minEmbedLength {
			chunks = append(chunks, final)
		}

		consumedTokens, err := t.tokenizer.Encode(ctx, windowText)
		if err != nil {
			return nil, err
		}
		tokens = tokens[min(len(consumedTokens), len(tokens)):]

		processed++
	}

	if len(tokens) > 0 {
		tail, err := t.tokenizer.Decode(ctx, tokens)
		if err != nil {
			return nil, err
		}
		final := t.cleanChunk(tail)
		if len(final) > t.minEmbedLength {
			chunks = append(chunks, final)
		}
	}

	return chunks, nil
}

func (t *TokenSplitter) cleanChunk(s string) string {
	if !t.keepSeparator {
		s = strings.ReplaceAll(s, "\n", " ")
	}
	return strings.TrimSpace(s)
}

func lastSentenceEnd(s string) int {
	return max(
		strings.LastIndex(s, "."),
		strings.LastIndex(s, "?"),
		strings.LastIndex(s, "!"),
		strings.LastIndex(s, "\n"),
	)
}

func (t *TokenSplitter) Transform(ctx context.Context, docs []*document.Document) ([]*document.Document, error) {
	return t.splitter.Transform(ctx, docs)
}
