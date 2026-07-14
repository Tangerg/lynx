package documentpipeline

import (
	"context"

	"github.com/Tangerg/lynx/core/document"
)

var (
	_ Transformer = (*Nop)(nil)
	_ Formatter   = (*Nop)(nil)
	_ Batcher     = (*Nop)(nil)
)

// Nop is the identity formatter, transformer, and batcher.
type Nop struct{}

// nopSingleton is the shared zero-state instance — Nop has no fields,
// so allocating a fresh one per caller would just produce garbage.
var nopSingleton = &Nop{}

func NewNop() *Nop { return nopSingleton }

func (n *Nop) Format(doc *document.Document, _ MetadataMode) (string, error) {
	if doc == nil {
		return "", nil
	}
	return doc.Text, nil
}

func (n *Nop) Transform(_ context.Context, docs []*document.Document) ([]*document.Document, error) {
	return docs, nil
}

func (n *Nop) Batch(_ context.Context, docs []*document.Document) ([][]*document.Document, error) {
	return [][]*document.Document{docs}, nil
}
