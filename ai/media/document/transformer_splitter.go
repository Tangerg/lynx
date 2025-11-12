package document

import (
	"context"
	"errors"
	"maps"
)

// SplitterConfig holds the configuration for Splitter.
type SplitterConfig struct {
	// CopyFormatter determines whether to copy the formatter from the original document
	// to each split chunk.
	// Optional. Defaults to false if not provided.
	// Set to true if you want split chunks to inherit the parent document's
	// formatting behavior for consistent output across chunks.
	CopyFormatter bool

	// SplitFunc defines the function used to split document text into chunks.
	// Required. Must not be nil.
	// The function receives the document text and returns a slice of text chunks.
	// Implementations can use various strategies like fixed-size splitting,
	// sentence-based splitting, semantic chunking, etc.
	SplitFunc func(context.Context, string) ([]string, error)
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

var _ Transformer = (*Splitter)(nil)

// Splitter transforms documents by splitting their text content into smaller chunks.
//
// This transformer is useful for:
//   - Breaking large documents into manageable pieces for embedding generation
//   - Creating chunks that fit within token limits of language models
//   - Improving retrieval granularity by splitting at semantic boundaries
//   - Enabling parallel processing of document segments
//
// The splitter preserves metadata from the original document across all chunks,
// allowing each chunk to maintain context about its source. Empty chunks are
// automatically filtered out to avoid creating meaningless documents.
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

func (s *Splitter) splitSingleDocument(ctx context.Context, doc *Document) ([]*Document, error) {
	textChunks, err := s.config.SplitFunc(ctx, doc.Text)
	if err != nil {
		return nil, err
	}

	splitDocs := make([]*Document, 0, len(textChunks))

	for _, chunk := range textChunks {
		if chunk == "" {
			continue
		}
		chunkDoc, err := NewDocument(chunk, nil)
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

func (s *Splitter) Transform(ctx context.Context, docs []*Document) ([]*Document, error) {
	processedDocs := make([]*Document, 0, len(docs))

	for _, doc := range docs {
		docChunks, err := s.splitSingleDocument(ctx, doc)
		if err != nil {
			return nil, err
		}

		processedDocs = append(processedDocs, docChunks...)
	}

	return processedDocs, nil
}
