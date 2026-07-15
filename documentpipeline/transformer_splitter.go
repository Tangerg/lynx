package documentpipeline

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/documentpipeline/id"
)

// Chunk-lineage metadata keys. A [Splitter] stamps every emitted chunk
// with these so downstream stages (retrieval, reranking, citation) can
// trace a chunk back to the document it came from and reconstruct
// ordering. They mirror the de-facto field names used across the
// ecosystem.
const (
	// MetadataKeyParentID holds the source document's ID. Only set when
	// the source document carried a non-empty ID.
	MetadataKeyParentID = "parent_document_id"

	// MetadataKeyChunkIndex holds the 0-based position of this chunk
	// among its siblings (counts only emitted, non-empty chunks).
	MetadataKeyChunkIndex = "chunk_index"

	MetadataKeyChunkTotal = "chunk_total"
)

type SplitterConfig struct {
	SplitFunc func(ctx context.Context, text string) ([]string, error)

	IDGenerator id.Generator
}

func (c SplitterConfig) Validate() error {
	if c.SplitFunc == nil {
		return errors.New("documentpipeline.SplitterConfig: SplitFunc is required")
	}
	return nil
}

var _ Transformer = (*Splitter)(nil)

// Splitter is a [Transformer] that calls a configurable SplitFunc on
// each input document's text and emits one new document per non-empty
// chunk. Original metadata is cloned onto every chunk and stamped with
// chunk-lineage keys ([MetadataKeyParentID], [MetadataKeyChunkIndex],
// [MetadataKeyChunkTotal]) so callers can trace chunks back to their
// source.
type Splitter struct {
	splitFunc   func(ctx context.Context, text string) ([]string, error)
	idGenerator id.Generator
}

func NewSplitter(config SplitterConfig) (*Splitter, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Splitter{
		splitFunc:   config.SplitFunc,
		idGenerator: config.IDGenerator,
	}, nil
}

// Transform splits every input document and concatenates the resulting
// chunks. Order is preserved — chunks of doc[i] all appear before
// chunks of doc[i+1].
func (s *Splitter) Transform(ctx context.Context, docs []*document.Document) ([]*document.Document, error) {
	out := make([]*document.Document, 0, len(docs))
	for _, doc := range docs {
		chunks, err := s.splitOne(ctx, doc)
		if err != nil {
			return nil, err
		}
		out = append(out, chunks...)
	}
	return out, nil
}

func (s *Splitter) splitOne(ctx context.Context, doc *document.Document) ([]*document.Document, error) {
	chunks, err := s.splitFunc(ctx, doc.Text)
	if err != nil {
		return nil, err
	}

	nonEmpty := make([]string, 0, len(chunks))
	for _, text := range chunks {
		if text != "" {
			nonEmpty = append(nonEmpty, text)
		}
	}

	total := len(nonEmpty)
	out := make([]*document.Document, 0, total)
	for index, text := range nonEmpty {
		chunk, err := document.NewDocument(text, nil)
		if err != nil {
			return nil, err
		}

		chunk.Metadata = doc.Metadata.Clone()
		if chunk.Metadata == nil {
			chunk.Metadata = metadata.New()
		}
		if err := chunk.Metadata.Set(MetadataKeyChunkIndex, index); err != nil {
			return nil, err
		}
		if err := chunk.Metadata.Set(MetadataKeyChunkTotal, total); err != nil {
			return nil, err
		}
		if doc.ID != "" {
			if err := chunk.Metadata.Set(MetadataKeyParentID, doc.ID); err != nil {
				return nil, err
			}
		}
		if err := assignID(ctx, chunk, s.idGenerator); err != nil {
			return nil, err
		}
		out = append(out, chunk)
	}
	return out, nil
}
