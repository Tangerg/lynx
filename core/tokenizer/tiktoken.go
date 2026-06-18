package tokenizer

import (
	"context"
	"encoding/json"
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

// estimateOne tokenizes one [media.Media] payload — count the MIME
// type's tokens (so multimodal callers get a per-attachment overhead)
// then count the data's tokens. Non-string / non-bytes data is JSON-
// marshaled; unmarshalable values fall back to MIME-only counting
// rather than failing the whole estimate.
func (t *Tiktoken) estimateOne(_ context.Context, m *media.Media) (int, error) {
	if m == nil {
		return 0, nil
	}

	mimeTokens := len(t.encoding.Encode(m.MimeType.String(), nil, nil))

	text, ok := payloadAsText(m.Data)
	if !ok {
		return mimeTokens, nil
	}
	return mimeTokens + len(t.encoding.Encode(text, nil, nil)), nil
}

// payloadAsText converts a Data field into a string for token counting.
// Returns ("", false) when JSON marshaling would have failed; the
// caller treats that as "skip the data tokens" rather than erroring.
func payloadAsText(data any) (string, bool) {
	switch typed := data.(type) {
	case string:
		return typed, true
	case []byte:
		return string(typed), true
	default:
		bytes, err := json.Marshal(typed)
		if err != nil {
			return "", false
		}
		return string(bytes), true
	}
}

func (t *Tiktoken) Encode(_ context.Context, text string) ([]int, error) {
	return t.encoding.Encode(text, nil, nil), nil
}

func (t *Tiktoken) Decode(_ context.Context, tokens []int) (string, error) {
	return t.encoding.Decode(tokens), nil
}
