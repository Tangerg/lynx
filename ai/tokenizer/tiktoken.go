package tokenizer

import (
	"context"
	"encoding/json"

	"github.com/pkoukk/tiktoken-go"

	"github.com/Tangerg/lynx/ai/media"
	"github.com/Tangerg/lynx/pkg/mime"
)

var _ Estimator = (*Tiktoken)(nil)
var _ Tokenizer = (*Tiktoken)(nil)

// Tiktoken is a token count estimator implementation using the tiktoken library.
// It provides token estimation for text and media content based on OpenAI's tokenization models.
type Tiktoken struct {
	encodingName string
	encoding     *tiktoken.Tiktoken
}

// NewTiktokenWithCL100KBase creates a new Tiktoken instance using the CL100K_BASE encoding model.
// This is a convenience function for the most commonly used encoding.
//
// Returns a new Tiktoken instance
func NewTiktokenWithCL100KBase() *Tiktoken {
	cli, err := NewTiktoken(tiktoken.MODEL_CL100K_BASE)
	if err != nil {
		panic(err)
	}
	return cli
}

// NewTiktoken creates a new Tiktoken instance with the specified encoding name.
//
// Parameters:
//   - encodingName: the name of the tiktoken encoding to use
//
// Returns a new Tiktoken instance or an error if the encoding cannot be loaded.
func NewTiktoken(encodingName string) (*Tiktoken, error) {
	encoding, err := tiktoken.GetEncoding(encodingName)
	if err != nil {
		return nil, err
	}
	return &Tiktoken{
		encodingName: encodingName,
		encoding:     encoding,
	}, nil
}

// EstimateText estimates the number of tokens in the given text.
// It converts the text to a Media object with text/plain MIME type and delegates to EstimateMedia.
//
// Parameters:
//   - ctx: context for request cancellation and timeout control
//   - text: the text to estimate the number of tokens for
//
// Returns the estimated number of tokens and any error that occurred during estimation.
func (t *Tiktoken) EstimateText(ctx context.Context, text string) (int, error) {
	mt, err := mime.New("text", "plain")
	if err != nil {
		return 0, err
	}
	return t.EstimateMedia(ctx, &media.Media{
		Data:     text,
		MimeType: mt,
	})
}

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
func (t *Tiktoken) EstimateMedia(ctx context.Context, media ...*media.Media) (int, error) {
	if len(media) == 0 {
		return 0, nil
	}
	var tokenCount int
	for _, m := range media {
		token, err := t.estimateMedia(ctx, m)
		if err != nil {
			return 0, err
		}
		tokenCount += token
	}
	return tokenCount, nil
}

// estimateMedia estimates the number of tokens for a single media object.
// It calculates tokens for both the MIME type and the media data content.
// The data is converted to text based on its type:
//   - string: used directly
//   - []byte: converted to string
//   - other types: JSON marshaled then converted to string
//
// Parameters:
//   - ctx: context (currently unused but kept for interface consistency)
//   - media: the media object to estimate tokens for
//
// Returns the estimated number of tokens for the media object and any error.
// If JSON marshaling fails for non-string/[]byte data, the error is ignored
// and only MIME type tokens are counted.
func (t *Tiktoken) estimateMedia(_ context.Context, media *media.Media) (int, error) {
	if media == nil {
		return 0, nil
	}
	// Count tokens for MIME type
	mt := media.MimeType.String()
	token := len(t.encoding.Encode(mt, nil, nil))

	// Convert data to text based on type
	data := media.Data
	var text string
	switch data.(type) {
	case string:
		text = data.(string)
	case []byte:
		text = string(data.([]byte))
	default:
		// Try to JSON marshal other types
		bytes, err := json.Marshal(data)
		if err != nil {
			return token, nil // ignore error, only count MIME type tokens
		}
		text = string(bytes)
	}

	// Count tokens for content and add to MIME type tokens
	token = token + len(t.encoding.Encode(text, nil, nil))
	return token, nil
}

func (t *Tiktoken) Encode(_ context.Context, text string) ([]int, error) {
	return t.encoding.Encode(text, nil, nil), nil
}

func (t *Tiktoken) Decode(_ context.Context, token []int) (string, error) {
	return t.encoding.Decode(token), nil
}
