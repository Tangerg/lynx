package tokenizer

import "context"

// TextEstimator reports the token count a model would assign to text.
// Implementations may use a local vocabulary or a provider API.
type TextEstimator interface {
	EstimateText(context.Context, string) (int, error)
}

// Encoder converts text into vocabulary token IDs.
type Encoder interface {
	Encode(context.Context, string) ([]int, error)
}

// Decoder converts vocabulary token IDs back into text.
type Decoder interface {
	Decode(context.Context, []int) (string, error)
}

// Tokenizer combines the encoding capabilities required by token-aware text
// splitters. Providers that only count tokens should implement TextEstimator
// instead.
type Tokenizer interface {
	Encoder
	Decoder
}
