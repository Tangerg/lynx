package splitter

import (
	"context"
	"maps"

	"github.com/Tangerg/lynx/ai/content/document"
)

var _ document.Transformer = (*Splitter)(nil)

type Splitter struct {
	splitFunc     func(string) []string
	copyFormatter bool
}

func NewSplitter(splitFunc func(string) []string) *Splitter {
	return &Splitter{splitFunc: splitFunc}
}

func (s *Splitter) splitText(text string) []string {
	if s.splitFunc == nil {
		return []string{text}
	}

	return s.splitFunc(text)
}

func (s *Splitter) splitDocuments(doc *document.Document) []*document.Document {
	chunks := s.splitText(doc.Text())
	documents := make([]*document.Document, 0, len(chunks))

	for _, chunk := range chunks {
		if chunk == "" {
			continue
		}

		newDoc, _ := document.NewBuilder().
			WithMetadata(maps.Clone(doc.Metadata())).
			WithText(chunk).
			Build()

		if s.copyFormatter {
			newDoc.SetFormatter(doc.Formatter())
		}

		documents = append(documents, newDoc)
	}

	return documents
}

func (s *Splitter) SetCopyFormatter(copyFormatter bool) *Splitter {
	s.copyFormatter = copyFormatter
	return s
}

func (s *Splitter) CopyContentFormatter() bool {
	return s.copyFormatter
}

func (s *Splitter) Transform(_ context.Context, docs []*document.Document) ([]*document.Document, error) {
	results := make([]*document.Document, 0, len(docs))

	for _, doc := range docs {
		results = append(results, s.splitDocuments(doc)...)
	}

	return results, nil
}
