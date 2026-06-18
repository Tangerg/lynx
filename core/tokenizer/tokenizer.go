package tokenizer

import (
	"context"

	"github.com/Tangerg/lynx/core/media"
)

// TextEstimator returns an approximate token count for a string. The
// estimate is allowed to skip full tokenization — implementations
// commonly trade accuracy for speed (e.g. char-count divided by 4).
type TextEstimator interface {
	EstimateText(ctx context.Context, text string) (int, error)
}

// MediaEstimator returns an approximate token count for one or more
// media payloads. Different modalities use different formulas (images
// by tile count, audio by seconds, ...) — the implementation knows
// which to apply.
type MediaEstimator interface {
	// EstimateMedia returns the cumulative token count for the items.
	// Empty input is allowed and returns 0.
	EstimateMedia(ctx context.Context, items ...*media.Media) (int, error)
}

// Estimator is the union of [TextEstimator] and [MediaEstimator] —
// "estimate anything that can be in a chat message".
type Estimator interface {
	TextEstimator
	MediaEstimator
}

// Encoder converts text into the model's numerical token IDs.
type Encoder interface {
	Encode(ctx context.Context, text string) ([]int, error)
}

type Decoder interface {
	// Decode reconstructs text from token IDs. The reconstruction is
	// faithful when the implementation is BPE-style — losing only
	// whitespace normalization that the model's vocab can't represent.
	Decode(ctx context.Context, tokens []int) (string, error)
}

// Tokenizer is the union of [Encoder] and [Decoder]. Implementations
// must satisfy round-trip soundness: Decode(Encode(x)) ≈ x.
type Tokenizer interface {
	Encoder
	Decoder
}
