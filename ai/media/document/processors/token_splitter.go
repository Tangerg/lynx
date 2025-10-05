package processors

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/tokenizer"
)

const (
	DefaultChunkSize      = 800
	DefaultMinChunkSize   = 350
	DefaultMinEmbedLength = 5
	DefaultMaxChunkCount  = 10000
)

type TokenSplitter struct {
	once           sync.Once
	splitter       *Splitter
	Tokenizer      tokenizer.Tokenizer
	ChunkSize      int
	MinChunkSize   int
	MinEmbedLength int
	MaxChunkCount  int
	KeepSeparator  bool
	CopyFormatter  bool
}

func (t *TokenSplitter) initializeConfig() {
	if t.ChunkSize <= 0 {
		t.ChunkSize = DefaultChunkSize
	}
	if t.MinChunkSize <= 0 {
		t.MinChunkSize = DefaultMinChunkSize
	}
	if t.MinEmbedLength <= 0 {
		t.MinEmbedLength = DefaultMinEmbedLength
	}
	if t.MaxChunkCount <= 0 {
		t.MaxChunkCount = DefaultMaxChunkCount
	}
}

func (t *TokenSplitter) initializeSplitter() {
	t.once.Do(func() {
		t.initializeConfig()

		t.splitter = &Splitter{
			CopyFormatter: t.CopyFormatter,
			SplitFunc:     t.splitByTokens,
		}
	})
}

func (t *TokenSplitter) splitByTokens(ctx context.Context, text string) ([]string, error) {
	if t.Tokenizer == nil {
		return nil, errors.New("tokenizer is required")
	}

	if strings.TrimSpace(text) == "" {
		return []string{}, nil
	}

	tokens, err := t.Tokenizer.Encode(ctx, text)
	if err != nil {
		return nil, err
	}

	textChunks := make([]string, 0, t.ChunkSize)
	processedCount := 0

	for len(tokens) > 0 && processedCount < t.MaxChunkCount {
		chunkEnd := min(t.ChunkSize, len(tokens))
		currentTokens := tokens[:chunkEnd]

		chunkText, err := t.Tokenizer.Decode(ctx, currentTokens)
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

		if lastPunctuation != -1 && lastPunctuation > t.MinChunkSize {
			chunkText = chunkText[:lastPunctuation+1]
		}

		var finalChunk string
		if t.KeepSeparator {
			finalChunk = strings.TrimSpace(chunkText)
		} else {
			finalChunk = strings.TrimSpace(
				strings.ReplaceAll(chunkText, "\n", " "),
			)
		}

		if len(finalChunk) > t.MinEmbedLength {
			textChunks = append(textChunks, finalChunk)
		}

		actualTokens, err := t.Tokenizer.Encode(ctx, chunkText)
		if err != nil {
			return nil, err
		}
		tokens = tokens[len(actualTokens):]

		processedCount++
	}

	if len(tokens) > 0 {
		remainingText, err := t.Tokenizer.Decode(ctx, tokens)
		if err != nil {
			return nil, err
		}

		cleanedText := strings.TrimSpace(
			strings.ReplaceAll(remainingText, "\n", " "),
		)

		if len(cleanedText) > t.MinEmbedLength {
			textChunks = append(textChunks, cleanedText)
		}
	}

	return textChunks, nil
}

func (t *TokenSplitter) Process(ctx context.Context, docs []*document.Document) ([]*document.Document, error) {
	t.initializeSplitter()

	return t.splitter.Process(ctx, docs)
}
