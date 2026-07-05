package document

import (
	"context"
	"errors"
	"maps"

	"github.com/Tangerg/lynx/core/document/id"
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
	CopyFormatter bool

	SplitFunc func(ctx context.Context, text string) ([]string, error)

	IDGenerator id.Generator
}

func (c SplitterConfig) Validate() error {
	if c.SplitFunc == nil {
		return errors.New("document.SplitterConfig: SplitFunc is required")
	}
	return nil
}

var _ Transformer = (*Splitter)(nil)

// Splitter is a [Transformer] that calls a configurable SplitFunc on
// each input document's text and emits one new document per non-empty
// chunk. Original metadata is cloned onto every chunk and stamped with
// chunk-lineage keys ([MetadataKeyParentID], [MetadataKeyChunkIndex],
// [MetadataKeyChunkTotal]) so callers can trace chunks back to their
// source; the source document's retrieval score is carried through.
type Splitter struct {
	copyFormatter bool
	splitFunc     func(ctx context.Context, text string) ([]string, error)
	idGenerator   id.Generator
}

func NewSplitter(config SplitterConfig) (*Splitter, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Splitter{
		copyFormatter: config.CopyFormatter,
		splitFunc:     config.SplitFunc,
		idGenerator:   config.IDGenerator,
	}, nil
}

// Transform splits every input document and concatenates the resulting
// chunks. Order is preserved — chunks of doc[i] all appear before
// chunks of doc[i+1].
func (s *Splitter) Transform(ctx context.Context, docs []*Document) ([]*Document, error) {
	out := make([]*Document, 0, len(docs))
	for _, doc := range docs {
		chunks, err := s.splitOne(ctx, doc)
		if err != nil {
			return nil, err
		}
		out = append(out, chunks...)
	}
	return out, nil
}

func (s *Splitter) splitOne(ctx context.Context, doc *Document) ([]*Document, error) {
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
	out := make([]*Document, 0, total)
	for index, text := range nonEmpty {
		chunk, err := NewDocument(text, nil)
		if err != nil {
			return nil, err
		}

		chunk.Metadata = maps.Clone(doc.Metadata)
		if chunk.Metadata == nil {
			chunk.Metadata = make(map[string]any)
		}
		chunk.Metadata[MetadataKeyChunkIndex] = index
		chunk.Metadata[MetadataKeyChunkTotal] = total
		if doc.ID != "" {
			chunk.Metadata[MetadataKeyParentID] = doc.ID
		}
		chunk.Score = doc.Score

		if s.copyFormatter {
			chunk.Formatter = doc.Formatter
		}
		if err := chunk.EnsureID(ctx, s.idGenerator); err != nil {
			return nil, err
		}
		out = append(out, chunk)
	}
	return out, nil
}
