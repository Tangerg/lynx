package transformers

import (
	"context"
	"strings"

	"github.com/Tangerg/lynx/ai/media/document"
)

type TextSplitterConfig struct {
	Separator     string
	CopyFormatter bool
}

var _ document.Transformer = (*TextSplitter)(nil)

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

func (t *TextSplitter) Transform(ctx context.Context, docs []*document.Document) ([]*document.Document, error) {
	return t.splitter.Transform(ctx, docs)
}
