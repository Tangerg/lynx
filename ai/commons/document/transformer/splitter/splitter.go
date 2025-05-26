package splitter

import (
	"context"
	"maps"

	"github.com/Tangerg/lynx/ai/commons/document"
)

var _ document.Transformer = (*Splitter)(nil)

// Splitter is a document transformer that splits documents into smaller chunks
// based on a provided split function. It preserves metadata and optionally
// copies content formatters from the original document to each chunk.
type Splitter struct {
	splitFunc            func(string) []string
	copyContentFormatter bool
}

// NewSplitter creates a new Splitter with the specified split function.
//
// Parameters:
//   - splitFunc: function that takes text and returns a slice of text chunks
//
// Returns a new Splitter instance.
func NewSplitter(splitFunc func(string) []string) *Splitter {
	return &Splitter{splitFunc: splitFunc}
}

// splitText splits the given text using the configured split function.
// If no split function is provided, returns the original text as a single chunk.
//
// Parameters:
//   - text: the text to split
//
// Returns a slice of text chunks.
func (s *Splitter) splitText(text string) []string {
	if s.splitFunc == nil {
		return []string{text}
	}
	return s.splitFunc(text)
}

// splitDocuments splits a single document into multiple documents based on text chunks.
// Each resulting document preserves the original metadata and optionally the content formatter.
// Empty chunks are filtered out.
//
// Parameters:
//   - doc: the document to split
//
// Returns a slice of split documents.
func (s *Splitter) splitDocuments(doc *document.Document) []*document.Document {
	chunks := s.splitText(doc.Text())
	docs := make([]*document.Document, 0, len(chunks))
	for _, chunk := range chunks {
		if chunk == "" {
			continue
		}
		newDoc, _ := document.NewBuilder().
			WithMetadata(maps.Clone(doc.Metadata())).
			WithText(chunk).
			Build()
		if s.copyContentFormatter {
			newDoc.SetContentFormatter(doc.ContentFormatter())
		}
		docs = append(docs, newDoc)
	}
	return docs
}

// SetCopyContentFormatter enables or disables copying content formatter from original
// documents to split chunks.
//
// Parameters:
//   - copyContentFormatter: true to enable copying content formatter, false to disable
//
// Returns the TextSplitter instance for method chaining.
func (s *Splitter) SetCopyContentFormatter(copyContentFormatter bool) *Splitter {
	s.copyContentFormatter = copyContentFormatter
	return s
}

// CopyContentFormatter returns whether the splitter copies content formatter
// from original documents to split chunks.
//
// Returns true if content formatter will be copied, false otherwise.
func (s *Splitter) CopyContentFormatter() bool {
	return s.copyContentFormatter
}

// Transform processes a batch of documents by splitting each document into smaller chunks.
// All resulting chunks are flattened into a single slice.
//
// Parameters:
//   - ctx: context for request cancellation and timeout control (currently unused)
//   - docs: input documents to be split
//
// Returns a slice of split documents and nil error. This implementation never returns an error.
func (s *Splitter) Transform(_ context.Context, docs []*document.Document) ([]*document.Document, error) {
	rv := make([]*document.Document, 0, len(docs))
	for _, doc := range docs {
		rv = append(rv, s.splitDocuments(doc)...)
	}
	return rv, nil
}
