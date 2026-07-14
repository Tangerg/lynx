// Package tiktoken implements tokenizer capabilities with OpenAI's tiktoken
// vocabularies.
package tiktoken

import (
	"context"
	"fmt"

	tiktokenlib "github.com/pkoukk/tiktoken-go"

	"github.com/Tangerg/lynx/tokenizer"
)

var (
	_ tokenizer.TextEstimator = (*Tokenizer)(nil)
	_ tokenizer.Tokenizer     = (*Tokenizer)(nil)
)

// Tokenizer encodes, decodes, and counts text with one tiktoken vocabulary.
// It is safe for concurrent use.
type Tokenizer struct {
	encoding *tiktokenlib.Tiktoken
}

// NewDefault returns a tokenizer using the cl100k_base vocabulary.
func NewDefault() (*Tokenizer, error) {
	return New(tiktokenlib.MODEL_CL100K_BASE)
}

// New loads encodingName and returns an error when the vocabulary is unknown.
func New(encodingName string) (*Tokenizer, error) {
	encoding, err := tiktokenlib.GetEncoding(encodingName)
	if err != nil {
		return nil, fmt.Errorf("tiktoken.New: load %q: %w", encodingName, err)
	}
	return &Tokenizer{encoding: encoding}, nil
}

// EstimateText returns the exact token count for the configured vocabulary.
func (t *Tokenizer) EstimateText(ctx context.Context, text string) (int, error) {
	tokens, err := t.Encode(ctx, text)
	if err != nil {
		return 0, err
	}
	return len(tokens), nil
}

// Encode converts text to token IDs.
func (t *Tokenizer) Encode(ctx context.Context, text string) ([]int, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return t.encoding.Encode(text, nil, nil), nil
}

// Decode converts token IDs to text.
func (t *Tokenizer) Decode(ctx context.Context, tokens []int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return t.encoding.Decode(tokens), nil
}
