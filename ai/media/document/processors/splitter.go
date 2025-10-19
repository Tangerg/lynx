package processors

import (
	"context"
	"errors"
	"maps"

	"github.com/Tangerg/lynx/ai/media/document"
)

type SplitterConfig struct {
	CopyFormatter bool
	SplitFunc     func(context.Context, string) ([]string, error)
}

func (c *SplitterConfig) validate() error {
	if c == nil {
		return errors.New("config is required")
	}
	if c.SplitFunc == nil {
		return errors.New("config split func is required")
	}
	return nil
}

var _ document.Processor = (*Splitter)(nil)

type Splitter struct {
	config *SplitterConfig
}

func NewSplitter(config *SplitterConfig) (*Splitter, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	return &Splitter{
		config: config,
	}, nil
}

func (s *Splitter) splitSingleDocument(ctx context.Context, doc *document.Document) ([]*document.Document, error) {
	textChunks, err := s.config.SplitFunc(ctx, doc.Text)
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

		if s.config.CopyFormatter {
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
