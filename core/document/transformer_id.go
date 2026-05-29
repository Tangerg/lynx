package document

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/core/document/id"
)

// IDAssignerConfig configures an [IDAssigner].
type IDAssignerConfig struct {
	// Generator produces each document's id. Required.
	Generator id.Generator

	// Overwrite re-generates ids for documents that already have one.
	// Defaults to false (only documents with an empty ID are assigned).
	Overwrite bool
}

// Validate returns an error when required fields are missing.
func (c IDAssignerConfig) Validate() error {
	if c.Generator == nil {
		return errors.New("document.IDAssignerConfig: Generator is required")
	}
	return nil
}

var _ Transformer = (*IDAssigner)(nil)

// IDAssigner is a [Transformer] that fills in document ids — the
// pipeline-stage form of [Document.EnsureID]. Drop it after a [Reader]
// or [Splitter] so every document carries an id before it reaches a
// vector store. Documents pass through in place (same slice, same
// pointers); only the ID field is touched.
//
// Pair an [id.Sha256Generator] for content-addressable, dedup-friendly
// ids, or an [id.UUIDGenerator] for unconditional uniqueness.
type IDAssigner struct {
	generator id.Generator
	overwrite bool
}

// NewIDAssigner builds an [IDAssigner]. Returns an error when the
// configuration is invalid.
func NewIDAssigner(config IDAssignerConfig) (*IDAssigner, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &IDAssigner{
		generator: config.Generator,
		overwrite: config.Overwrite,
	}, nil
}

// Transform assigns an id to each document and returns the same slice.
func (a *IDAssigner) Transform(ctx context.Context, docs []*Document) ([]*Document, error) {
	for _, doc := range docs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if doc.ID != "" && !a.overwrite {
			continue
		}
		if a.overwrite {
			doc.ID = ""
		}
		if err := doc.EnsureID(ctx, a.generator); err != nil {
			return nil, err
		}
	}
	return docs, nil
}
