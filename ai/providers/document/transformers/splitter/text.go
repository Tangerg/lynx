package splitter

import (
	"context"
	"strings"

	"github.com/Tangerg/lynx/ai/content/document"
)

var _ document.Transformer = (*TextSplitter)(nil)

type TextSplitter struct {
	splitter *Splitter
}

func NewTextSplitter(separator string) *TextSplitter {
	splitFunc := func(text string) []string {
		return strings.Split(text, separator)
	}

	return &TextSplitter{
		splitter: NewSplitter(splitFunc),
	}
}

func (t *TextSplitter) SetCopyFormatter(copyFormatter bool) *TextSplitter {
	t.splitter.SetCopyFormatter(copyFormatter)
	return t
}

func (t *TextSplitter) Transform(ctx context.Context, docs []*document.Document) ([]*document.Document, error) {
	return t.splitter.Transform(ctx, docs)
}
