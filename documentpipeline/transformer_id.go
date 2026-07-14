package documentpipeline

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/documentpipeline/id"
)

type IDAssignerConfig struct {
	Generator id.Generator

	Overwrite bool
}

func (c IDAssignerConfig) Validate() error {
	if c.Generator == nil {
		return errors.New("documentpipeline.IDAssignerConfig: Generator is required")
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

func NewIDAssigner(config IDAssignerConfig) (*IDAssigner, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &IDAssigner{
		generator: config.Generator,
		overwrite: config.Overwrite,
	}, nil
}

func (a *IDAssigner) Transform(ctx context.Context, docs []*document.Document) ([]*document.Document, error) {
	for _, doc := range docs {
		if err := ctx.Err(); err != nil {
			return nil, err
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

func assignID(ctx context.Context, doc *document.Document, generator id.Generator) error {
	if doc.ID != "" || generator == nil {
		return nil
	}
	generated, err := generator.Generate(ctx, doc.Text, doc.Media, doc.Metadata)
	if err != nil {
		return err
	}
	doc.ID = generated
	return nil
}
