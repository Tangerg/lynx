// Package tiktoken implements the core/tokenizer capabilities with OpenAI's
// tiktoken vocabularies, adapting github.com/pkoukk/tiktoken-go to the small
// interfaces Core defines.
//
// The vocabulary is chosen explicitly, because no single encoding is correct
// across models:
//
//	tokenizer, err := tiktoken.New(tiktoken.O200KBase)
//	if err != nil {
//	    return err
//	}
//
//	count, err := tokenizer.CountText(ctx, "hello")
//
// An unknown encoding returns ErrInvalidEncoding at construction rather than
// silently falling back to another vocabulary, because a wrong token count is
// not visible until a request is rejected for length.
//
// This module owns no provider request, no model-to-encoding routing, no text
// splitting, and no cache. Splitting belongs to etl; routing belongs to
// whatever knows the model.
//
// See README.md for usage and ARCHITECTURE.md for the boundaries this rests on.
package tiktoken
