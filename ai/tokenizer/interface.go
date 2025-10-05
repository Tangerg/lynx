// Package tokenizer provides interfaces for text and media tokenization operations.
// This package defines the core abstractions for token estimation, encoding, and decoding
// operations used in AI and natural language processing applications.
package tokenizer

import (
	"context"

	"github.com/Tangerg/lynx/ai/media"
)

// TextEstimator estimates the number of tokens in text content.
// This interface is useful for calculating text token usage before making API calls
// to AI services that have token limits or charge based on token consumption.
type TextEstimator interface {
	// EstimateText estimates the number of tokens in the given text.
	//
	// This method provides a quick way to estimate token count without performing
	// the actual tokenization process, which can be more efficient for usage tracking
	// and cost estimation purposes.
	//
	// Parameters:
	//   - ctx: context for request cancellation and timeout control
	//   - text: the text to estimate the number of tokens for
	//
	// Returns the estimated number of tokens and any error that occurred during estimation.
	EstimateText(ctx context.Context, text string) (int, error)
}

// MediaEstimator estimates the number of tokens in media content.
// This interface is useful for calculating media token usage for multimodal AI services
// that process images, audio, video, and other non-text content types.
type MediaEstimator interface {
	// EstimateMedia estimates the number of tokens in the given media content.
	// This method accepts a variadic parameter, allowing estimation for single media,
	// multiple media objects, or an empty list.
	//
	// Different media types (images, audio, video) may have different token calculation
	// methods. The implementation should handle various media formats and return the
	// cumulative token count for all provided media objects.
	//
	// Parameters:
	//   - ctx: context for request cancellation and timeout control
	//   - media: the media content to estimate the number of tokens for
	//
	// Returns the total estimated number of tokens for all provided media and any error
	// that occurred during the estimation process.
	EstimateMedia(ctx context.Context, media ...*media.Media) (int, error)
}

// Estimator combines both text and media token estimation capabilities.
// This interface represents a complete estimation system that can handle
// both textual and multimedia content for comprehensive token usage calculation.
//
// Implementations of this interface should provide consistent estimation
// methods across different content types, allowing unified token management
// for complex multimodal applications.
type Estimator interface {
	TextEstimator
	MediaEstimator
}

// Encoder provides functionality to convert text into token sequences.
// This interface is typically used to prepare text input for AI models
// that operate on numerical token representations.
type Encoder interface {
	// Encode converts the given text into a sequence of token IDs.
	//
	// This method performs the actual tokenization process, breaking down
	// the input text into discrete tokens and mapping them to their
	// corresponding numerical identifiers in the tokenizer's vocabulary.
	//
	// Parameters:
	//   - ctx: context for request cancellation and timeout control
	//   - text: the input text to be tokenized
	//
	// Returns a slice of token IDs representing the input text and any error
	// that occurred during the encoding process.
	Encode(ctx context.Context, text string) ([]int, error)
}

// Decoder provides functionality to convert token sequences back into text.
// This interface is typically used to convert AI model outputs from
// numerical token representations back to human-readable text.
type Decoder interface {
	// Decode converts a sequence of token IDs back into text.
	//
	// This method performs the reverse operation of encoding, mapping
	// token IDs back to their corresponding text representations and
	// reconstructing the original or generated text.
	//
	// Parameters:
	//   - ctx: context for request cancellation and timeout control
	//   - tokens: slice of token IDs to be decoded
	//
	// Returns the reconstructed text and any error that occurred during
	// the decoding process.
	Decode(ctx context.Context, tokens []int) (string, error)
}

// Tokenizer combines both encoding and decoding capabilities.
// This interface represents a complete tokenization system that can
// convert between text and token representations in both directions.
//
// Implementations of this interface should ensure that the encoding
// and decoding operations are consistent with each other, meaning
// that decoding the result of encoding a text should yield the
// original text (or a semantically equivalent representation).
type Tokenizer interface {
	Encoder
	Decoder
}
