package documentpipeline

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/document"
)

// IDAssignerConfig configures document ID generation.
type IDAssignerConfig struct {
	// Generator is required.
	Generator IDGenerator

	// Overwrite replaces existing IDs instead of preserving them.
	Overwrite bool
}

var _ Transformer = (*IDAssigner)(nil)

// IDAssigner is a [Transformer] that fills in document ids — the
// pipeline-stage form of [Document.EnsureID]. Drop it after a [Reader]
// or [Splitter] so every document carries an id before it reaches a
// vector store. Documents pass through in place (same slice, same
// pointers); only the ID field is touched.
//
// Pair an [SHA256IDGenerator] for content-addressable, dedup-friendly
// ids, or an [UUIDGenerator] for unconditional uniqueness.
type IDAssigner struct {
	generator IDGenerator
	overwrite bool
}

func NewIDAssigner(config IDAssignerConfig) (*IDAssigner, error) {
	if isNil(config.Generator) {
		return nil, errors.New("document pipeline: ID generator is required")
	}
	return &IDAssigner{
		generator: config.Generator,
		overwrite: config.Overwrite,
	}, nil
}

func (a *IDAssigner) Transform(ctx context.Context, docs []*document.Document) ([]*document.Document, error) {
	for index, doc := range docs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if doc == nil {
			return nil, fmt.Errorf("document pipeline: assign ID to document %d: %w", index, ErrNilDocument)
		}
		if a.overwrite {
			doc.ID = ""
		} else if doc.ID != "" {
			continue
		}
		if err := assignID(ctx, doc, a.generator); err != nil {
			return nil, err
		}
	}
	return docs, nil
}

func assignID(ctx context.Context, doc *document.Document, generator IDGenerator) error {
	if doc == nil {
		return ErrNilDocument
	}
	if doc.ID != "" || generator == nil {
		return nil
	}
	generated, err := generator.Generate(ctx, doc)
	if err != nil {
		return err
	}
	if strings.TrimSpace(generated) == "" {
		return errors.New("document pipeline: ID generator returned an empty ID")
	}
	doc.ID = generated
	return nil
}
