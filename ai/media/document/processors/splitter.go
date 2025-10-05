package processors

import (
	"context"
	"errors"
	"maps"

	"github.com/Tangerg/lynx/ai/media/document"
)

var _ document.Processor = (*Splitter)(nil)

type Splitter struct {
	CopyFormatter bool
	SplitFunc     func(context.Context, string) ([]string, error)
}

func (s *Splitter) splitTextContent(ctx context.Context, text string) ([]string, error) {
	if s.SplitFunc == nil {
		return nil, errors.New("split function is required")
	}

	return s.SplitFunc(ctx, text)
}

func (s *Splitter) splitSingleDocument(ctx context.Context, doc *document.Document) ([]*document.Document, error) {
	textChunks, err := s.splitTextContent(ctx, doc.Text)
	if err != nil {
		return nil, err
	}

	splitDocs := make([]*document.Document, 0, len(textChunks))

	for _, chunk := range textChunks {
		if chunk == "" {
			continue
		}
		chunkDoc, err := document.NewDocument(chunk, nil)
		if err != nil {
			return nil, err
		}
		chunkDoc.Metadata = maps.Clone(doc.Metadata)

		if s.CopyFormatter {
			chunkDoc.Formatter = doc.Formatter
		}

		splitDocs = append(splitDocs, chunkDoc)
	}

	return splitDocs, nil
}

func (s *Splitter) Process(ctx context.Context, docs []*document.Document) ([]*document.Document, error) {
	processedDocs := make([]*document.Document, 0, len(docs))

	for _, doc := range docs {
		docChunks, err := s.splitSingleDocument(ctx, doc)
		if err != nil {
			return nil, err
		}

		processedDocs = append(processedDocs, docChunks...)
	}

	return processedDocs, nil
}
