package splitter

import (
	"context"
	"strings"

	"github.com/Tangerg/lynx/ai/commons/document"
)

var _ document.Transformer = (*TextSplitter)(nil)

// TextSplitter is a document transformer that splits documents by a specified separator string.
// It provides a simple built-in implementation for common text splitting scenarios based on delimiters.
type TextSplitter struct {
	splitter *Splitter
}

// NewTextSplitter creates a new TextSplitter with the specified separator.
//
// Parameters:
//   - separator: the string used to split the document text
//
// Returns a new TextSplitter instance.
//
// Common usage examples:
//
//	NewTextSplitter("\n")      // Split by newlines
//	NewTextSplitter("\r\n")    // Split by Windows line endings
//	NewTextSplitter(",")       // Split by commas
//	NewTextSplitter(" ")       // Split by spaces
//	NewTextSplitter("\n\n")    // Split by double newlines (paragraphs)
func NewTextSplitter(separator string) *TextSplitter {
	return &TextSplitter{
		splitter: NewSplitter(func(s string) []string {
			return strings.Split(s, separator)
		}),
	}
}

// SetCopyContentFormatter enables or disables copying content formatter from original
// documents to split chunks.
//
// Parameters:
//   - copyContentFormatter: true to enable copying content formatter, false to disable
//
// Returns the TextSplitter instance for method chaining.
func (s *TextSplitter) SetCopyContentFormatter(copyContentFormatter bool) *TextSplitter {
	s.splitter.SetCopyContentFormatter(copyContentFormatter)
	return s
}

// Transform splits the provided documents using the configured separator.
// Each document's content is split by the separator string, creating new
// documents for each resulting chunk while preserving metadata.
//
// Parameters:
//   - ctx: the context for the transformation operation
//   - docs: slice of documents to be split
//
// Returns:
//   - []*document.Document: slice of split document chunks
//   - error: any error that occurred during transformation
//
// The transformation process:
//  1. Splits each document's content by the configured separator
//  2. Creates new documents for each non-empty chunk
//  3. Preserves original metadata in each chunk
//  4. Optionally copies content formatter if enabled
func (s *TextSplitter) Transform(ctx context.Context, docs []*document.Document) ([]*document.Document, error) {
	return s.splitter.Transform(ctx, docs)
}
