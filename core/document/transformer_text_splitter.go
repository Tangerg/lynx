package document

import (
	"context"
	"strings"

	"github.com/Tangerg/lynx/core/document/id"
)

type TextSplitterConfig struct {
	Separator string

	// CopyFormatter copies the source document's [Formatter] to each
	// chunk. Defaults to false.
	CopyFormatter bool

	// IDGenerator, when set, assigns an id to every emitted chunk.
	// nil leaves chunk IDs empty. See [SplitterConfig.IDGenerator].
	IDGenerator id.Generator
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
//	s := document.NewTextSplitter(document.TextSplitterConfig{Separator: "\n\n"})
//	chunks, _ := s.Transform(ctx, []*document.Document{doc})
type TextSplitter struct {
	splitter *Splitter
}

func (c *TextSplitterConfig) ApplyDefaults() {
	if c.Separator == "" {
		c.Separator = "\n"
	}
}

func NewTextSplitter(config TextSplitterConfig) *TextSplitter {
	config.ApplyDefaults()
	splitter, _ := NewSplitter(SplitterConfig{
		CopyFormatter: config.CopyFormatter,
		IDGenerator:   config.IDGenerator,
		SplitFunc: func(_ context.Context, text string) ([]string, error) {
			return strings.Split(text, config.Separator), nil
		},
	})
	return &TextSplitter{splitter: splitter}
}

func (t *TextSplitter) Transform(ctx context.Context, docs []*Document) ([]*Document, error) {
	return t.splitter.Transform(ctx, docs)
}
