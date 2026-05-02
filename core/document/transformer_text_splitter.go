package document

import (
	"context"
	"strings"
)

// TextSplitterConfig configures a [TextSplitter] — the simple
// string-separator splitter.
type TextSplitterConfig struct {
	// Separator splits the text. Common values: "\n" (lines),
	// "\n\n" (paragraphs), ". " (sentences). Defaults to "\n".
	Separator string

	// CopyFormatter copies the source document's [Formatter] to each
	// chunk. Defaults to false.
	CopyFormatter bool
}

var _ Transformer = (*TextSplitter)(nil)

// TextSplitter is the convenience wrapper around [Splitter] that
// splits on a fixed string separator. Reach for it when you want
// quick line / paragraph chunking; for semantic or token-aware
// chunking use [Splitter] with a custom SplitFunc, or
// [TokenSplitter].
//
// Example:
//
//	s := document.NewTextSplitter(&document.TextSplitterConfig{Separator: "\n\n"})
//	chunks, _ := s.Transform(ctx, []*document.Document{doc})
type TextSplitter struct {
	config   *TextSplitterConfig
	splitter *Splitter
}

// NewTextSplitter builds a [TextSplitter]. nil config falls back to
// line-by-line splitting.
func NewTextSplitter(config *TextSplitterConfig) *TextSplitter {
	if config == nil {
		config = &TextSplitterConfig{Separator: "\n"}
	}
	splitter, _ := NewSplitter(&SplitterConfig{
		CopyFormatter: config.CopyFormatter,
		SplitFunc: func(_ context.Context, text string) ([]string, error) {
			return strings.Split(text, config.Separator), nil
		},
	})
	return &TextSplitter{config: config, splitter: splitter}
}

// NewDefaultTextSplitter is a shorthand for NewTextSplitter(nil).
func NewDefaultTextSplitter() *TextSplitter { return NewTextSplitter(nil) }

// Transform delegates to the wrapped [Splitter].
func (t *TextSplitter) Transform(ctx context.Context, docs []*Document) ([]*Document, error) {
	return t.splitter.Transform(ctx, docs)
}
