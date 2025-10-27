package rag

import (
	"context"

	"github.com/Tangerg/lynx/ai/media/document"
)

var _ QueryExpander = (*Nop)(nil)
var _ QueryTransformer = (*Nop)(nil)
var _ QueryAugmenter = (*Nop)(nil)
var _ DocumentRetriever = (*Nop)(nil)
var _ DocumentRefiner = (*Nop)(nil)

// Nop is a no-operation implementation of all RAG pipeline interfaces.
// It provides default pass-through behavior without performing any actual operations,
// useful for testing, optional pipeline stages, or as a placeholder implementation.
type Nop struct{}

// nop is a singleton instance of Nop to avoid unnecessary allocations.
var nop = &Nop{}

// NewNop returns a singleton instance of Nop.
// Since Nop is stateless, the same instance can be safely reused.
func NewNop() *Nop {
	return nop
}

// Expand returns the input query as a single-element slice without modification.
// It implements the QueryExpander interface with no-op behavior.
func (n *Nop) Expand(_ context.Context, query *Query) ([]*Query, error) {
	return []*Query{query}, nil
}

// Retrieve returns an empty document list without performing any retrieval.
// It implements the DocumentRetriever interface with no-op behavior.
func (n *Nop) Retrieve(_ context.Context, _ *Query) ([]*document.Document, error) {
	return nil, nil
}

// Transform returns the input query without modification.
// It implements the QueryTransformer interface with no-op behavior.
func (n *Nop) Transform(_ context.Context, query *Query) (*Query, error) {
	return query, nil
}

// Augment returns the input query without augmentation.
// It implements the QueryAugmenter interface with no-op behavior.
func (n *Nop) Augment(_ context.Context, query *Query, _ []*document.Document) (*Query, error) {
	return query, nil
}

// Refine returns the input documents without refinement.
// It implements the DocumentRefiner interface with no-op behavior.
func (n *Nop) Refine(_ context.Context, _ *Query, documents []*document.Document) ([]*document.Document, error) {
	return documents, nil
}
