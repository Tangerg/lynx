package document

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/lynx/core/tokenizer"
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

// TokenSplitterConfig configures a [TokenSplitter]. The Tokenizer is
// required; the rest fall back to sensible defaults.
type TokenSplitterConfig struct {
	// Tokenizer encodes/decodes text. Use the same vocabulary as the
	// embedding model that will consume the chunks. Required.
	Tokenizer tokenizer.Tokenizer

	// ChunkSize targets a max tokens-per-chunk. Defaults to 800.
	ChunkSize int

	// MinChunkSize is the minimum size in characters at which the
	// splitter will attempt to break at a sentence boundary.
	// Defaults to 350.
	MinChunkSize int

	// MinEmbedLength filters out chunks shorter than this many
	// characters. Defaults to 5.
	MinEmbedLength int

	// MaxChunkCount caps how many chunks one document may produce —
	// guard against pathological inputs. Defaults to 10000.
	MaxChunkCount int

	// KeepSeparator preserves newlines instead of collapsing them to
	// spaces. Defaults to false (cleaner one-line chunks).
	KeepSeparator bool

	// CopyFormatter copies the source document's [Formatter] to each
	// chunk. Defaults to false.
	CopyFormatter bool
}

// validate fills in defaults for non-positive numeric fields and
// returns an error when required fields are missing.
func (c *TokenSplitterConfig) validate() error {
	if c == nil {
		return errors.New("document.TokenSplitterConfig: config must not be nil")
	}
	if c.Tokenizer == nil {
		return errors.New("document.TokenSplitterConfig: Tokenizer is required")
	}
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
	return nil
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
	copyFormatter  bool
	splitter       *Splitter
}

// NewTokenSplitter builds a [TokenSplitter]. Returns an error when
// config is invalid.
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

// splitByTokens implements the algorithm documented on [TokenSplitter].
func (t *TokenSplitter) splitByTokens(ctx context.Context, text string) ([]string, error) {
	if strings.TrimSpace(text) == "" {
		return []string{}, nil
	}

	tokens, err := t.tokenizer.Encode(ctx, text)
	if err != nil {
		return nil, err
	}

	chunks := make([]string, 0, t.chunkSize)
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

		// Try to end at the last sentence boundary inside the window.
		lastPunct := lastSentenceEnd(windowText)
		if lastPunct != -1 && lastPunct > t.minChunkSize {
			windowText = windowText[:lastPunct+1]
		}

		final := strings.TrimSpace(windowText)
		if !t.keepSeparator {
			final = strings.TrimSpace(strings.ReplaceAll(windowText, "\n", " "))
		}
		if len(final) > t.minEmbedLength {
			chunks = append(chunks, final)
		}

		// Re-encode the (possibly trimmed) chunk to know how many tokens
		// to consume.
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
		final := strings.TrimSpace(strings.ReplaceAll(tail, "\n", " "))
		if len(final) > t.minEmbedLength {
			chunks = append(chunks, final)
		}
	}

	return chunks, nil
}

// lastSentenceEnd returns the highest byte index of any of ., ?, !, \n
// in s, or -1 if none are present.
func lastSentenceEnd(s string) int {
	return max(
		strings.LastIndex(s, "."),
		max(strings.LastIndex(s, "?"),
			max(strings.LastIndex(s, "!"),
				strings.LastIndex(s, "\n"))),
	)
}

// Transform delegates to the wrapped [Splitter].
func (t *TokenSplitter) Transform(ctx context.Context, docs []*Document) ([]*Document, error) {
	return t.splitter.Transform(ctx, docs)
}
