package splitter

import (
	"context"
	"github.com/Tangerg/lynx/ai/core/document"
	"github.com/Tangerg/lynx/pkg/kv"
	"strings"
)

var _ document.Transformer = (*TextSplitter)(nil)

type TextSplitter struct {
	TextSplitFunc        func(string) []string
	copyContentFormatter bool
}

func NewTextSplitter(textSplitFunc func(string) []string) *TextSplitter {
	return &TextSplitter{TextSplitFunc: textSplitFunc}
}

func (t *TextSplitter) SetCopyContentFormatter(copyContentFormatter bool) {
	t.copyContentFormatter = copyContentFormatter
}

func (t *TextSplitter) IsCopyContentFormatter() bool {
	return t.copyContentFormatter
}

func (t *TextSplitter) Transform(_ context.Context, documents []*document.Document) ([]*document.Document, error) {
	if t.TextSplitFunc == nil {
		t.TextSplitFunc = func(s string) []string {
			return strings.Split(s, "\n")
		}
	}
	return t.doSplitDocuments(documents), nil
}

func (t *TextSplitter) doSplitDocuments(docs []*document.Document) []*document.Document {
	var (
		texts      = make([]string, 0, len(docs))
		metadatas  = make([]kv.KSVA, 0, len(docs))
		formatters = make([]document.ContentFormatter, 0, len(docs))
	)

	for _, doc := range docs {
		texts = append(texts, doc.Content())
		metadatas = append(metadatas, doc.Metadata())
		formatters = append(formatters, doc.ContentFormatter())
	}
	return t.createDocuments(texts, metadatas, formatters)
}

func (t *TextSplitter) createDocuments(texts []string, metadatas []kv.KSVA, formatters []document.ContentFormatter) []*document.Document {
	docs := make([]*document.Document, 0, len(texts))
	for i := 0; i < len(texts); i++ {
		text := texts[i]
		metadata := metadatas[i]
		chunks := t.TextSplitFunc(text)
		for _, chunk := range chunks {
			metadataClone := metadata.Clone()
			newDoc := document.NewBuilder().
				WithMetadata(metadataClone).
				WithContent(chunk).
				Build()
			if t.copyContentFormatter {
				newDoc.SetContentFormatter(formatters[i])
			}
			docs = append(docs, newDoc)
		}
	}
	return docs
}
