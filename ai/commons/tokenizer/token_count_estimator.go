package tokenizer

import (
	"context"

	"github.com/Tangerg/lynx/ai/commons/content"
)

// TokenCountEstimator estimates the number of tokens in a given text or media content.
type TokenCountEstimator interface {
	// EstimateText estimates the number of tokens in the given text.
	//
	// Parameters:
	//   - ctx: context for request cancellation and timeout control
	//   - text: the text to estimate the number of tokens for
	//
	// Returns the estimated number of tokens and any error that occurred during estimation.
	EstimateText(ctx context.Context, text string) (int, error)

	// EstimateMedia estimates the number of tokens in the given media content.
	// This method accepts a variadic parameter, allowing estimation for single media,
	// multiple media objects, or an empty list.
	//
	// Parameters:
	//   - ctx: context for request cancellation and timeout control
	//   - media: the media content to estimate the number of tokens for
	//
	// Returns the total estimated number of tokens for all provided media and any error
	// that occurred during the estimation process.
	//
	// Usage examples:
	//   EstimateMedia(ctx)                    // Empty list, returns 0
	//   EstimateMedia(ctx, media1)            // Single media object
	//   EstimateMedia(ctx, media1, media2)    // Multiple media objects
	//   EstimateMedia(ctx, mediaList...)      // Spread slice of media objects
	EstimateMedia(ctx context.Context, media ...*content.Media) (int, error)
}
