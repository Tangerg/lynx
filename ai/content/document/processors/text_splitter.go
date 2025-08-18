package processors

import (
	"context"
	"strings"
	"sync"

	"github.com/Tangerg/lynx/ai/content/document"
)

type TextSplitter struct {
	once          sync.Once
	splitter      *Splitter
	Separator     string
	CopyFormatter bool
}

func (t *TextSplitter) initializeSplitter() {
	t.once.Do(func() {
		t.splitter = &Splitter{
			CopyFormatter: t.CopyFormatter,
			SplitFunc: func(_ context.Context, text string) ([]string, error) {
				return strings.Split(text, t.Separator), nil
			},
		}
	})
}

func (t *TextSplitter) Process(ctx context.Context, docs []*document.Document) ([]*document.Document, error) {
	t.initializeSplitter()

	return t.splitter.Process(ctx, docs)
}
