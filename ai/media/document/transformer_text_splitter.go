package document

import (
	"context"
	"strings"
)

// TextSplitterConfig holds the configuration for TextSplitter.
type TextSplitterConfig struct {
	// Separator defines the string used to split document text into chunks.
	// Optional. Defaults to "\n" (newline) if not provided.
	// Common separators include:
	//   - "\n" for line-based splitting
	//   - "\n\n" for paragraph-based splitting
	//   - ". " for sentence-based splitting
	//   - Custom markers for structured documents
	Separator string

	// CopyFormatter determines whether to copy the formatter from the original document
	// to each split chunk.
	// Optional. Defaults to false if not provided.
	// Set to true if you want split chunks to inherit the parent document's
	// formatting behavior for consistent output across chunks.
	CopyFormatter bool
}

var _ Transformer = (*TextSplitter)(nil)

// TextSplitter is a simple text-based document splitter that divides documents
// using a string separator.
//
// This transformer is useful for:
//   - Quick and simple document chunking without complex logic
//   - Splitting documents by lines, paragraphs, or custom delimiters
//   - Building basic document processing pipelines
//   - Handling structured text with consistent separators
//
// TextSplitter is a convenience wrapper around Splitter that uses string.Split
// internally. For more advanced splitting strategies (semantic chunking, token-aware
// splitting, overlapping chunks), consider using Splitter directly with a custom
// SplitFunc.
type TextSplitter struct {
	config   *TextSplitterConfig
	splitter *Splitter
}

func NewTextSplitter(config *TextSplitterConfig) *TextSplitter {
	if config == nil {
		config = &TextSplitterConfig{
			Separator: "\n",
		}
	}
	splitter, _ := NewSplitter(&SplitterConfig{
		CopyFormatter: config.CopyFormatter,
		SplitFunc: func(ctx context.Context, s string) ([]string, error) {
			return strings.Split(s, config.Separator), nil
		},
	})

	return &TextSplitter{
		config:   config,
		splitter: splitter,
	}
}

func NewDefaultTextSplitter() *TextSplitter {
	return NewTextSplitter(nil)
}

func (t *TextSplitter) Transform(ctx context.Context, docs []*Document) ([]*Document, error) {
	return t.splitter.Transform(ctx, docs)
}
