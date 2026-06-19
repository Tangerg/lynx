package rag

import (
	"context"

	"github.com/Tangerg/lynx/core/document"
)

var (
	_ QueryExpander     = (*Nop)(nil)
	_ QueryTransformer  = (*Nop)(nil)
	_ QueryAugmenter    = (*Nop)(nil)
	_ DocumentRetriever = (*Nop)(nil)
	_ DocumentRefiner   = (*Nop)(nil)
)

// Nop is the do-nothing implementation that satisfies every RAG
// interface — pass-through expand/transform/augment/refine and an
// empty-list retriever. Useful as a default when a pipeline stage is
// optional, or as a test double.
type Nop struct{}

// nopSingleton is the shared zero-state instance — Nop has no fields,
// so allocating per caller would just produce garbage.
var nopSingleton = &Nop{}

func NewNop() *Nop { return nopSingleton }

func (n *Nop) Expand(_ context.Context, query *Query) ([]*Query, error) {
	return []*Query{query}, nil
}

func (n *Nop) Retrieve(_ context.Context, _ *Query) ([]*document.Document, error) {
	return nil, nil
}

func (n *Nop) Transform(_ context.Context, query *Query) (*Query, error) {
	return query, nil
}

func (n *Nop) Augment(_ context.Context, query *Query, _ []*document.Document) (*Query, error) {
	return query, nil
}

func (n *Nop) Refine(_ context.Context, _ *Query, documents []*document.Document) ([]*document.Document, error) {
	return documents, nil
}
