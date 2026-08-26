package documentpipeline

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/tokenizer"
)

const (
	defaultMaxTokensPerChunk     = 800
	defaultMinTokensPerChunk     = 350
	defaultMinCharactersPerChunk = 5
	defaultMaxChunks             = 10_000
)

// ErrChunkLimitExceeded reports that splitting would exceed the configured
// output bound. The splitter fails instead of emitting one oversized tail.
var ErrChunkLimitExceeded = errors.New("document pipeline: chunk limit exceeded")

// TokenSplitterConfig configures token-aware chunking. Zero sizing values use
// documented defaults; negative values are rejected.
type TokenSplitterConfig struct {
	Tokenizer tokenizer.Tokenizer

	MaxTokensPerChunk     int
	MinTokensPerChunk     int
	MinCharactersPerChunk int
	MaxChunks             int
	PreserveNewlines      bool
	IDGenerator           IDGenerator
}

// TokenSplitter splits document text into token-bounded chunks and prefers a
// sentence boundary once the configured minimum token count has been reached.
type TokenSplitter struct {
	tokenizer             tokenizer.Tokenizer
	maxTokensPerChunk     int
	minTokensPerChunk     int
	minCharactersPerChunk int
	maxChunks             int
	preserveNewlines      bool
	splitter              *Splitter
}

func NewTokenSplitter(config TokenSplitterConfig) (*TokenSplitter, error) {
	if isNil(config.Tokenizer) {
		return nil, errors.New("document pipeline: tokenizer is required")
	}
	if config.MaxTokensPerChunk < 0 || config.MinTokensPerChunk < 0 ||
		config.MinCharactersPerChunk < 0 || config.MaxChunks < 0 {
		return nil, errors.New("document pipeline: token splitter limits must not be negative")
	}
	if config.MaxTokensPerChunk == 0 {
		config.MaxTokensPerChunk = defaultMaxTokensPerChunk
	}
	if config.MinTokensPerChunk == 0 {
		config.MinTokensPerChunk = min(defaultMinTokensPerChunk, config.MaxTokensPerChunk)
	}
	if config.MinCharactersPerChunk == 0 {
		config.MinCharactersPerChunk = defaultMinCharactersPerChunk
	}
	if config.MaxChunks == 0 {
		config.MaxChunks = defaultMaxChunks
	}
	if config.MinTokensPerChunk > config.MaxTokensPerChunk {
		return nil, fmt.Errorf(
			"document pipeline: minimum chunk tokens %d exceed maximum %d",
			config.MinTokensPerChunk,
			config.MaxTokensPerChunk,
		)
	}

	splitter := &TokenSplitter{
		tokenizer:             config.Tokenizer,
		maxTokensPerChunk:     config.MaxTokensPerChunk,
		minTokensPerChunk:     config.MinTokensPerChunk,
		minCharactersPerChunk: config.MinCharactersPerChunk,
		maxChunks:             config.MaxChunks,
		preserveNewlines:      config.PreserveNewlines,
	}
	base, err := NewSplitter(SplitterConfig{
		SplitFunc:   splitter.SplitText,
		IDGenerator: config.IDGenerator,
	})
	if err != nil {
		return nil, err
	}
	splitter.splitter = base
	return splitter, nil
}

// SplitText tokenizes text and emits chunks within the configured bounds.
func (s *TokenSplitter) SplitText(ctx context.Context, text string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}

	tokens, err := s.tokenizer.Encode(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("document pipeline: tokenize text: %w", err)
	}

	chunks := make([]string, 0, min(len(tokens)/s.maxTokensPerChunk+1, s.maxChunks))
	for len(tokens) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		windowTokens := tokens[:min(s.maxTokensPerChunk, len(tokens))]
		windowText, err := s.tokenizer.Decode(ctx, windowTokens)
		if err != nil {
			return nil, fmt.Errorf("document pipeline: decode token window: %w", err)
		}
		if strings.TrimSpace(windowText) == "" {
			tokens = tokens[len(windowTokens):]
			continue
		}

		selected := windowText
		consumedCount := len(windowTokens)
		if boundary := s.lastSentenceBoundary(windowText); boundary > 0 && boundary < len(windowText) {
			prefix := windowText[:boundary]
			prefixTokens, err := s.tokenizer.Encode(ctx, prefix)
			if err != nil {
				return nil, fmt.Errorf("document pipeline: measure sentence boundary: %w", err)
			}
			if len(prefixTokens) >= s.minTokensPerChunk && len(prefixTokens) < len(windowTokens) {
				originalPrefix, err := s.tokenizer.Decode(ctx, windowTokens[:len(prefixTokens)])
				if err != nil {
					return nil, fmt.Errorf("document pipeline: verify sentence boundary: %w", err)
				}
				if originalPrefix == prefix {
					selected = prefix
					consumedCount = len(prefixTokens)
				}
			}
		}
		tokens = tokens[consumedCount:]

		chunk := s.clean(selected)
		if utf8.RuneCountInString(chunk) < s.minCharactersPerChunk {
			continue
		}
		if len(chunks) == s.maxChunks {
			return nil, fmt.Errorf("%w: maximum is %d", ErrChunkLimitExceeded, s.maxChunks)
		}
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

func (s *TokenSplitter) clean(text string) string {
	if !s.preserveNewlines {
		text = strings.ReplaceAll(text, "\n", " ")
	}
	return strings.TrimSpace(text)
}

func (*TokenSplitter) lastSentenceBoundary(text string) int {
	boundary := -1
	for _, punctuation := range []string{".", "?", "!", "\n", "。", "？", "！"} {
		if index := strings.LastIndex(text, punctuation); index >= 0 {
			boundary = max(boundary, index+len(punctuation))
		}
	}
	return boundary
}

// Split emits token-bounded document chunks with cloned metadata and lineage.
func (s *TokenSplitter) Split(ctx context.Context, docs []*document.Document) ([]*document.Document, error) {
	return s.splitter.Split(ctx, docs)
}
