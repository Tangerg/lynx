package document

import (
	"context"
	"errors"
	"maps"
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
}

// validate returns a descriptive error when required fields are missing.
func (c *SplitterConfig) validate() error {
	if c.SplitFunc == nil {
		return errors.New("document.SplitterConfig: SplitFunc is required")
	}
	return nil
}

var _ Transformer = (*Splitter)(nil)

// Splitter is a [Transformer] that calls a configurable SplitFunc on
// each input document's text and emits one new document per non-empty
// chunk. Original metadata is cloned onto every chunk so callers can
// trace chunks back to their source.
type Splitter struct {
	copyFormatter bool
	splitFunc     func(ctx context.Context, text string) ([]string, error)
}

// NewSplitter builds a [Splitter] from config. Returns an error when
// the configuration is invalid.
func NewSplitter(config SplitterConfig) (*Splitter, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &Splitter{
		copyFormatter: config.CopyFormatter,
		splitFunc:     config.SplitFunc,
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

// splitOne splits one document and clones metadata onto each chunk.
// Empty chunks are dropped — they would just inflate the result without
// adding information.
func (s *Splitter) splitOne(ctx context.Context, doc *Document) ([]*Document, error) {
	chunks, err := s.splitFunc(ctx, doc.Text)
	if err != nil {
		return nil, err
	}

	out := make([]*Document, 0, len(chunks))
	for _, text := range chunks {
		if text == "" {
			continue
		}

		chunk, err := NewDocument(text, nil)
		if err != nil {
			return nil, err
		}
		chunk.Metadata = maps.Clone(doc.Metadata)
		if s.copyFormatter {
			chunk.Formatter = doc.Formatter
		}
		out = append(out, chunk)
	}
	return out, nil
}
