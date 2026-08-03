package documentpipeline

import (
	"context"
	"strings"

	"github.com/Tangerg/lynx/core/document"
)

// TextSplitterConfig configures fixed-separator chunking. The zero Separator
// uses a newline.
type TextSplitterConfig struct {
	Separator string

	// IDGenerator, when set, assigns an id to every emitted chunk.
	// nil leaves chunk IDs empty. See [SplitterConfig.IDGenerator].
	IDGenerator IDGenerator
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
//	s, _ := documentpipeline.NewTextSplitter(documentpipeline.TextSplitterConfig{Separator: "\n\n"})
//	chunks, _ := s.Transform(ctx, []*document.Document{doc})
type TextSplitter struct {
	splitter *Splitter
}

func NewTextSplitter(config TextSplitterConfig) (*TextSplitter, error) {
	separator := config.Separator
	if separator == "" {
		separator = "\n"
	}
	splitter, err := NewSplitter(SplitterConfig{
		IDGenerator: config.IDGenerator,
		SplitFunc: func(_ context.Context, text string) ([]string, error) {
			return strings.Split(text, separator), nil
		},
	})
	if err != nil {
		return nil, err
	}
	return &TextSplitter{splitter: splitter}, nil
}

func (t *TextSplitter) Transform(ctx context.Context, docs []*document.Document) ([]*document.Document, error) {
	return t.splitter.Transform(ctx, docs)
}
