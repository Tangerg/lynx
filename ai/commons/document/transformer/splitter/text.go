package splitter

import (
	"context"
	"maps"
	"strings"

	"github.com/Tangerg/lynx/ai/commons/document"
)

var _ document.Transformer = (*TextSplitter)(nil)

type TextSplitter struct {
	textSplitFunc        func(string) []string
	copyContentFormatter bool
}

func NewTextSplitter(textSplitFunc func(string) []string) *TextSplitter {
	return &TextSplitter{textSplitFunc: textSplitFunc}
}

func (t *TextSplitter) SetCopyContentFormatter(copyContentFormatter bool) {
	t.copyContentFormatter = copyContentFormatter
}

func (t *TextSplitter) IsCopyContentFormatter() bool {
	return t.copyContentFormatter
}

func (t *TextSplitter) Transform(_ context.Context, documents []*document.Document) ([]*document.Document, error) {
	if t.textSplitFunc == nil {
		t.textSplitFunc = func(s string) []string {
			return strings.Split(s, "\n")
		}
	}
	return t.doSplitDocuments(documents), nil
}

func (t *TextSplitter) doSplitDocuments(docs []*document.Document) []*document.Document {
	var (
		texts      = make([]string, 0, len(docs))
		metadatas  = make([]map[string]any, 0, len(docs))
		formatters = make([]document.ContentFormatter, 0, len(docs))
	)

	for _, doc := range docs {
		texts = append(texts, doc.Text())
		metadatas = append(metadatas, doc.Metadata())
		formatters = append(formatters, doc.ContentFormatter())
	}
	return t.createDocuments(texts, metadatas, formatters)
}

func (t *TextSplitter) createDocuments(texts []string, metadatas []map[string]any, formatters []document.ContentFormatter) []*document.Document {
	docs := make([]*document.Document, 0, len(texts))
	for i := 0; i < len(texts); i++ {
		text := texts[i]
		metadata := metadatas[i]
		chunks := t.textSplitFunc(text)
		for _, chunk := range chunks {
			if chunk == "" {
				continue
			}
			metadataClone := maps.Clone(metadata)
			newDoc, _ := document.NewBuilder().
				WithMetadata(metadataClone).
				WithText(chunk).
				Build()
			if t.copyContentFormatter {
				newDoc.SetContentFormatter(formatters[i])
			}
			docs = append(docs, newDoc)
		}
	}
	return docs
}
