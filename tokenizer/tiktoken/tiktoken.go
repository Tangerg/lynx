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
	// ErrUninitialized reports use of a zero-value Tokenizer.
	ErrUninitialized = errors.New("tiktoken: tokenizer is not initialized")
)

// Encoding identifies a tiktoken vocabulary. Callers choose explicitly because
// token counts are vocabulary-specific; there is no model-independent default.
type Encoding string

// Supported encodings.
const (
	O200KBase  = Encoding(tiktokenlib.MODEL_O200K_BASE)
	CL100KBase = Encoding(tiktokenlib.MODEL_CL100K_BASE)
	P50KBase   = Encoding(tiktokenlib.MODEL_P50K_BASE)
	P50KEdit   = Encoding(tiktokenlib.MODEL_P50K_EDIT)
	R50KBase   = Encoding(tiktokenlib.MODEL_R50K_BASE)
)

// Validate reports whether the encoding names a supported vocabulary.
func (e Encoding) Validate() error {
	_, err := e.load()
	return err
}

func (e Encoding) load() (*tiktokenlib.Tiktoken, error) {
	name := string(e)
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("%w: name must not be blank", ErrInvalidEncoding)
	}
	encoding, err := tiktokenlib.GetEncoding(name)
	if err != nil {
		return nil, fmt.Errorf("%w: load %q: %w", ErrInvalidEncoding, name, err)
	}
	return encoding, nil
}

var (
	_ tokenizer.TextEstimator = Tokenizer{}
	_ tokenizer.Tokenizer     = Tokenizer{}
)

// Tokenizer encodes, decodes, and counts text with one tiktoken vocabulary.
// It is safe for concurrent use.
type Tokenizer struct {
	encoding *tiktokenlib.Tiktoken
}

// New loads encoding and returns an error when the vocabulary is unknown.
func New(encoding Encoding) (Tokenizer, error) {
	native, err := encoding.load()
	if err != nil {
		return Tokenizer{}, err
	}
	return Tokenizer{encoding: native}, nil
}

// EstimateText returns the exact token count for the configured vocabulary.
func (t Tokenizer) EstimateText(ctx context.Context, text string) (int, error) {
	tokens, err := t.Encode(ctx, text)
	if err != nil {
		return 0, err
	}
	return len(tokens), nil
}

// Encode converts text to token IDs.
func (t Tokenizer) Encode(ctx context.Context, text string) ([]int, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := t.validate(); err != nil {
		return nil, err
	}
	return t.encoding.Encode(text, nil, nil), nil
}

// Decode converts token IDs to text.
func (t Tokenizer) Decode(ctx context.Context, tokens []int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := t.validate(); err != nil {
		return "", err
	}
	return t.encoding.Decode(tokens), nil
}

func (t Tokenizer) validate() error {
	if t.encoding == nil {
		return ErrUninitialized
	}
	return nil
}
