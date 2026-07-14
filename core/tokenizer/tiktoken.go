package tokenizer

import (
	"context"
	"fmt"

	"github.com/pkoukk/tiktoken-go"

	"github.com/Tangerg/lynx/core/media"
)

var (
	_ Estimator = (*Tiktoken)(nil)
	_ Tokenizer = (*Tiktoken)(nil)
)

// Tiktoken is the [Tokenizer] / [Estimator] implementation backed by
// OpenAI's tiktoken library. Most providers don't expose their actual
// tokenization, so tiktoken serves as a "close enough" general-purpose
// estimator across vendors.
//
// Example:
//
//	tk, _ := tokenizer.NewDefaultTiktoken()
//	n, _ := tk.EstimateText(ctx, "hello world") // ≈ 2 tokens
type Tiktoken struct {
	encoding *tiktoken.Tiktoken
}

func NewDefaultTiktoken() (*Tiktoken, error) {
	return NewTiktoken(tiktoken.MODEL_CL100K_BASE)
}

func NewTiktoken(encodingName string) (*Tiktoken, error) {
	encoding, err := tiktoken.GetEncoding(encodingName)
	if err != nil {
		return nil, fmt.Errorf("tokenizer.NewTiktoken: load %q: %w", encodingName, err)
	}
	return &Tiktoken{encoding: encoding}, nil
}

func (t *Tiktoken) EstimateText(_ context.Context, text string) (int, error) {
	return len(t.encoding.Encode(text, nil, nil)), nil
}

func (t *Tiktoken) EstimateMedia(ctx context.Context, items ...*media.Media) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}

	var total int
	for _, item := range items {
		count, err := t.estimateOne(ctx, item)
		if err != nil {
			return 0, err
		}
		total += count
	}
	return total, nil
}

// estimateOne tokenizes one [media.Media] payload: the MIME type contributes
// per-attachment overhead, followed by the active source value.
func (t *Tiktoken) estimateOne(_ context.Context, m *media.Media) (int, error) {
	if m == nil {
		return 0, nil
	}
	if err := m.Validate(); err != nil {
		return 0, err
	}

	mimeTokens := len(t.encoding.Encode(m.MIME, nil, nil))

	var text string
	switch m.Source.Kind {
	case media.SourceBytes:
		text = string(m.Source.Bytes)
	case media.SourceURI:
		text = m.Source.URI
	case media.SourceReference:
		text = m.Source.Ref
	}
	return mimeTokens + len(t.encoding.Encode(text, nil, nil)), nil
}

func (t *Tiktoken) Encode(_ context.Context, text string) ([]int, error) {
	return t.encoding.Encode(text, nil, nil), nil
}

func (t *Tiktoken) Decode(_ context.Context, tokens []int) (string, error) {
	return t.encoding.Decode(tokens), nil
}
