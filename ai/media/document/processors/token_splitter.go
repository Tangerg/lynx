package processors

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/tokenizer"
)

type TokenSplitterConfig struct {
	Tokenizer      tokenizer.Tokenizer
	ChunkSize      int
	MinChunkSize   int
	MinEmbedLength int
	MaxChunkCount  int
	KeepSeparator  bool
	CopyFormatter  bool
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

var _ document.Processor = (*TokenSplitter)(nil)

type TokenSplitter struct {
	config   *TokenSplitterConfig
	splitter *Splitter
}

func NewTokenSplitter(config *TokenSplitterConfig) (*TokenSplitter, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	ts := &TokenSplitter{
		config: config,
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

	tokens, err := t.config.Tokenizer.Encode(ctx, text)
	if err != nil {
		return nil, err
	}

	textChunks := make([]string, 0, t.config.ChunkSize)
	processedCount := 0

	for len(tokens) > 0 && processedCount < t.config.MaxChunkCount {
		chunkEnd := min(t.config.ChunkSize, len(tokens))
		currentTokens := tokens[:chunkEnd]

		chunkText, err := t.config.Tokenizer.Decode(ctx, currentTokens)
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

		if lastPunctuation != -1 && lastPunctuation > t.config.MinChunkSize {
			chunkText = chunkText[:lastPunctuation+1]
		}

		var finalChunk string
		if t.config.KeepSeparator {
			finalChunk = strings.TrimSpace(chunkText)
		} else {
			finalChunk = strings.TrimSpace(
				strings.ReplaceAll(chunkText, "\n", " "),
			)
		}

		if len(finalChunk) > t.config.MinEmbedLength {
			textChunks = append(textChunks, finalChunk)
		}

		actualTokens, err := t.config.Tokenizer.Encode(ctx, chunkText)
		if err != nil {
			return nil, err
		}
		tokens = tokens[len(actualTokens):]

		processedCount++
	}

	if len(tokens) > 0 {
		remainingText, err := t.config.Tokenizer.Decode(ctx, tokens)
		if err != nil {
			return nil, err
		}

		cleanedText := strings.TrimSpace(
			strings.ReplaceAll(remainingText, "\n", " "),
		)

		if len(cleanedText) > t.config.MinEmbedLength {
			textChunks = append(textChunks, cleanedText)
		}
	}

	return textChunks, nil
}

func (t *TokenSplitter) Process(ctx context.Context, docs []*document.Document) ([]*document.Document, error) {
	return t.splitter.Process(ctx, docs)
}
