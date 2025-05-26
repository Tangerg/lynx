package splitter

import (
	"context"
	"errors"
	"strings"

	"github.com/pkoukk/tiktoken-go"

	"github.com/Tangerg/lynx/ai/commons/document"
)

// tokenSplitter handles the core token-based text splitting logic.
// It uses tiktoken encoding to split text into semantically meaningful chunks
// while respecting token limits and preserving sentence boundaries.
type tokenSplitter struct {
	encoding              *tiktoken.Tiktoken // Token encoder/decoder
	chunkSize             int                // Maximum tokens per chunk
	minChunkSizeChars     int                // Minimum characters before breaking at punctuation
	minChunkLengthToEmbed int                // Minimum chunk length to include in results
	maxNumChunks          int                // Maximum number of chunks to generate
	keepSeparator         bool               // Whether to preserve line separators
}

// encodeTokens converts text to token IDs using the configured encoding.
func (t *tokenSplitter) encodeTokens(s string) []int {
	return t.encoding.Encode(s, nil, nil)
}

// decodeTokens converts token IDs back to text using the configured encoding.
func (t *tokenSplitter) decodeTokens(tokens []int) string {
	return t.encoding.Decode(tokens)
}

// split divides the input text into chunks based on token count and semantic boundaries.
// It attempts to break at sentence boundaries when possible to maintain readability.
func (t *tokenSplitter) split(text string) []string {
	if strings.TrimSpace(text) == "" {
		return []string{}
	}

	tokens := t.encodeTokens(text)
	chunks := make([]string, 0, t.chunkSize)
	numChunks := 0

	for len(tokens) > 0 && numChunks < t.maxNumChunks {
		chunkEnd := min(t.chunkSize, len(tokens))
		chunk := tokens[:chunkEnd]
		chunkText := t.decodeTokens(chunk)

		// Skip empty chunks
		if strings.TrimSpace(chunkText) == "" {
			tokens = tokens[len(chunk):]
			continue
		}

		// Find the last punctuation mark to break at sentence boundaries
		lastPunctuation := max(
			strings.LastIndex(chunkText, "."),
			max(strings.LastIndex(chunkText, "?"),
				max(strings.LastIndex(chunkText, "!"),
					strings.LastIndex(chunkText, "\n"))),
		)

		// Break at punctuation if it's far enough from the start
		if lastPunctuation != -1 && lastPunctuation > t.minChunkSizeChars {
			chunkText = chunkText[:lastPunctuation+1]
		}

		// Process the chunk text based on separator preferences
		var chunkTextToAppend string
		if t.keepSeparator {
			chunkTextToAppend = strings.TrimSpace(chunkText)
		} else {
			chunkTextToAppend = strings.TrimSpace(
				strings.ReplaceAll(chunkText, "\n", " "),
			)
		}

		// Only include chunks that meet minimum length requirements
		if len(chunkTextToAppend) > t.minChunkLengthToEmbed {
			chunks = append(chunks, chunkTextToAppend)
		}

		// Advance token position based on actual processed text
		processedTokens := t.encodeTokens(chunkText)
		tokens = tokens[len(processedTokens):]

		numChunks++
	}

	// Handle any remaining tokens
	if len(tokens) > 0 {
		remainingText := strings.TrimSpace(
			strings.ReplaceAll(
				t.decodeTokens(tokens),
				"\n",
				" ",
			),
		)
		if len(remainingText) > t.minChunkLengthToEmbed {
			chunks = append(chunks, remainingText)
		}
	}

	return chunks
}

var _ document.Transformer = (*TokenSplitter)(nil)

// TokenSplitter is a document transformer that splits documents based on token count
// using tiktoken encoding. It intelligently breaks text at sentence boundaries
// while respecting token limits to create semantically coherent chunks.
type TokenSplitter struct {
	splitter *Splitter
}

// SetCopyContentFormatter enables or disables copying content formatter from original
// documents to split chunks.
//
// Parameters:
//   - copyContentFormatter: true to enable copying content formatter, false to disable
//
// Returns the TokenSplitter instance for method chaining.
func (s *TokenSplitter) SetCopyContentFormatter(copyContentFormatter bool) *TokenSplitter {
	s.splitter.SetCopyContentFormatter(copyContentFormatter)
	return s
}

// Transform splits the provided documents using token-based chunking.
// Each document's content is split into chunks based on token count,
// attempting to preserve sentence boundaries for better readability.
//
// Parameters:
//   - ctx: the context for the transformation operation
//   - docs: slice of documents to be split
//
// Returns:
//   - []*document.Document: slice of split document chunks
//   - error: any error that occurred during transformation
func (s *TokenSplitter) Transform(ctx context.Context, docs []*document.Document) ([]*document.Document, error) {
	return s.splitter.Transform(ctx, docs)
}

// TokenSplitterBuilder provides a fluent interface for constructing TokenSplitter instances
// with customizable parameters for token-based text splitting.
type TokenSplitterBuilder struct {
	encoding              *tiktoken.Tiktoken // Token encoder/decoder (required)
	chunkSize             int                // Maximum tokens per chunk
	minChunkSizeChars     int                // Minimum characters before breaking at punctuation
	minChunkLengthToEmbed int                // Minimum chunk length to include in results
	maxNumChunks          int                // Maximum number of chunks to generate
	keepSeparator         bool               // Whether to preserve line separators
}

// NewTokenSplitterBuilder creates a new TokenSplitterBuilder with sensible defaults.
//
// Default values:
//   - chunkSize: 800 tokens
//   - minChunkSizeChars: 350 characters
//   - minChunkLengthToEmbed: 5 characters
//   - maxNumChunks: 10000 chunks
//   - keepSeparator: true
//
// Note: encoding must be set before calling Build().
func NewTokenSplitterBuilder() *TokenSplitterBuilder {
	return &TokenSplitterBuilder{
		chunkSize:             800,
		minChunkSizeChars:     350,
		minChunkLengthToEmbed: 5,
		maxNumChunks:          10000,
		keepSeparator:         true,
	}
}

// WithEncoding sets the tiktoken encoding to use for tokenization.
//
// Parameters:
//   - encoding: the tiktoken encoding instance (must not be nil)
//
// Returns the builder for method chaining.
func (t *TokenSplitterBuilder) WithEncoding(encoding *tiktoken.Tiktoken) *TokenSplitterBuilder {
	if encoding != nil {
		t.encoding = encoding
	}
	return t
}

// WithEncodingByEncodingName sets the tiktoken encoding by name.
//
// Parameters:
//   - encodingName: name of the encoding (e.g., "cl100k_base", "p50k_base")
//
// Returns the builder for method chaining.
// If the encoding name is invalid, this method has no effect.
func (t *TokenSplitterBuilder) WithEncodingByEncodingName(encodingName string) *TokenSplitterBuilder {
	encoding, err := tiktoken.GetEncoding(encodingName)
	if err == nil {
		t.encoding = encoding
	}
	return t
}

// WithChunkSize sets the maximum number of tokens per chunk.
//
// Parameters:
//   - chunkSize: maximum tokens per chunk (must be > 0)
//
// Returns the builder for method chaining.
func (t *TokenSplitterBuilder) WithChunkSize(chunkSize int) *TokenSplitterBuilder {
	if chunkSize > 0 {
		t.chunkSize = chunkSize
	}
	return t
}

// WithMinChunkSizeChars sets the minimum number of characters required
// before the splitter will break at a punctuation mark.
//
// Parameters:
//   - minChunkSizeChars: minimum characters before punctuation break (must be > 0)
//
// Returns the builder for method chaining.
func (t *TokenSplitterBuilder) WithMinChunkSizeChars(minChunkSizeChars int) *TokenSplitterBuilder {
	if minChunkSizeChars > 0 {
		t.minChunkSizeChars = minChunkSizeChars
	}
	return t
}

// WithMinChunkLengthToEmbed sets the minimum chunk length to include in results.
// Chunks shorter than this will be filtered out.
//
// Parameters:
//   - minChunkLengthToEmbed: minimum chunk length in characters (must be > 0)
//
// Returns the builder for method chaining.
func (t *TokenSplitterBuilder) WithMinChunkLengthToEmbed(minChunkLengthToEmbed int) *TokenSplitterBuilder {
	if minChunkLengthToEmbed > 0 {
		t.minChunkLengthToEmbed = minChunkLengthToEmbed
	}
	return t
}

// WithMaxNumChunks sets the maximum number of chunks to generate.
//
// Parameters:
//   - maxNumChunks: maximum number of chunks (must be > 0)
//
// Returns the builder for method chaining.
func (t *TokenSplitterBuilder) WithMaxNumChunks(maxNumChunks int) *TokenSplitterBuilder {
	if maxNumChunks > 0 {
		t.maxNumChunks = maxNumChunks
	}
	return t
}

// WithKeepSeparator controls whether line separators are preserved in chunks.
//
// Parameters:
//   - keepSeparator: true to preserve line breaks, false to replace with spaces
//
// Returns the builder for method chaining.
func (t *TokenSplitterBuilder) WithKeepSeparator(keepSeparator bool) *TokenSplitterBuilder {
	t.keepSeparator = keepSeparator
	return t
}

// Build creates a new TokenSplitter instance with the configured parameters.
//
// Returns:
//   - *TokenSplitter: the configured token splitter
//   - error: error if encoding is not set or other validation fails
//
// Example usage:
//
//	splitter, err := NewTokenSplitterBuilder().
//	    WithEncodingByEncodingName("cl100k_base").
//	    WithChunkSize(1000).
//	    WithMinChunkSizeChars(400).
//	    Build()
func (t *TokenSplitterBuilder) Build() (*TokenSplitter, error) {
	if t.encoding == nil {
		return nil, errors.New("encoding is required")
	}
	ts := &tokenSplitter{
		encoding:              t.encoding,
		chunkSize:             t.chunkSize,
		minChunkSizeChars:     t.minChunkSizeChars,
		minChunkLengthToEmbed: t.minChunkLengthToEmbed,
		maxNumChunks:          t.maxNumChunks,
		keepSeparator:         t.keepSeparator,
	}
	return &TokenSplitter{
		splitter: NewSplitter(ts.split),
	}, nil
}
