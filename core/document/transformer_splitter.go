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

	// MetadataKeyChunkTotal holds the total number of chunks produced
	// from the source document.
	MetadataKeyChunkTotal = "chunk_total"
)

// SplitterConfig configures a [Splitter]. SplitFunc is required;
// CopyFormatter copies the parent document's formatter to each chunk
// so downstream rendering stays consistent.
type SplitterConfig struct {
	// CopyFormatter copies the source document's [Formatter] to each
	// split chunk. Defaults to false (chunks get the no-op formatter).
	CopyFormatter bool

	// SplitFunc carves a document's text into chunks. Required.
	// Implementations can use any strategy (fixed-size, sentence,
	// token-aware) — see [transformer_text_splitter.go] and
	// [transformer_token_splitter.go] for ready-made strategies.
	SplitFunc func(ctx context.Context, text string) ([]string, error)

	// IDGenerator, when set, assigns an id to every chunk via
	// [Document.EnsureID] (after lineage metadata is stamped, so
	// content-addressable generators distinguish sibling chunks by
	// their differing chunk_index). nil leaves chunk IDs empty — assign
	// them later with an [IDAssigner] stage if needed.
	IDGenerator id.Generator
}

// validate returns a descriptive error when required fields are missing.
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

// NewSplitter builds a [Splitter] from config. Returns an error when
// the configuration is invalid.
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

// splitOne splits one document, clones metadata onto each chunk, and
// stamps chunk-lineage keys. Empty chunks are dropped first — they
// would just inflate the result without adding information, and would
// throw off chunk_index / chunk_total which count only emitted chunks.
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
