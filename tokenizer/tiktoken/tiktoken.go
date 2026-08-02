// Package tiktoken implements tokenizer capabilities with OpenAI's tiktoken
// vocabularies.
package tiktoken

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tiktokenlib "github.com/pkoukk/tiktoken-go"

	"github.com/Tangerg/lynx/tokenizer"
)

var (
	// ErrInvalidEncoding reports a blank or unknown tiktoken vocabulary name.
	ErrInvalidEncoding = errors.New("tiktoken: invalid encoding")
	// ErrUninitialized reports use of a zero-value or nil Tokenizer.
	ErrUninitialized = errors.New("tiktoken: tokenizer is not initialized")
)

// Supported vocabulary names. Callers choose explicitly because token counts
// are vocabulary-specific; there is no model-independent default.
const (
	O200KBase  = tiktokenlib.MODEL_O200K_BASE
	CL100KBase = tiktokenlib.MODEL_CL100K_BASE
	P50KBase   = tiktokenlib.MODEL_P50K_BASE
	P50KEdit   = tiktokenlib.MODEL_P50K_EDIT
	R50KBase   = tiktokenlib.MODEL_R50K_BASE
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

// New loads encodingName and returns an error when the vocabulary is unknown.
func New(encodingName string) (*Tokenizer, error) {
	if strings.TrimSpace(encodingName) == "" {
		return nil, fmt.Errorf("%w: name must not be blank", ErrInvalidEncoding)
	}
	encoding, err := tiktokenlib.GetEncoding(encodingName)
	if err != nil {
		return nil, fmt.Errorf("%w: load %q: %w", ErrInvalidEncoding, encodingName, err)
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
	if err := t.validate(); err != nil {
		return nil, err
	}
	return t.encoding.Encode(text, nil, nil), nil
}

// Decode converts token IDs to text.
func (t *Tokenizer) Decode(ctx context.Context, tokens []int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := t.validate(); err != nil {
		return "", err
	}
	return t.encoding.Decode(tokens), nil
}

func (t *Tokenizer) validate() error {
	if t == nil || t.encoding == nil {
		return ErrUninitialized
	}
	return nil
}
