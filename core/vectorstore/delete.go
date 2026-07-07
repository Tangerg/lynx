package vectorstore

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
)

// DeleteRequest is the input to [Deleter.Delete]: a metadata filter
// expression selecting the documents to remove.
type DeleteRequest struct {
	Filter ast.Expr `json:"-"`
}

func NewDeleteRequest(filter ast.Expr) (*DeleteRequest, error) {
	req := &DeleteRequest{Filter: filter}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	return req, nil
}

func (r *DeleteRequest) Validate() error {
	if r == nil {
		return ErrNilRequest
	}
	if r.Filter == nil {
		return ErrMissingFilter
	}

	if err := filter.Analyze(r.Filter); err != nil {
		return fmt.Errorf("vectorstore.DeleteRequest: filter analysis: %w", err)
	}
	return nil
}

// Deleter removes documents matching a metadata filter from the vector
// store.
type Deleter interface {
	// Delete removes every document matching the request's filter.
	Delete(ctx context.Context, request *DeleteRequest) error
}

// IDDeleter removes documents by their ids — the direct counterpart to
// the metadata-filter [Deleter]. It is an optional capability kept OUT
// of [Store]: not every backend can address documents by id (some only
// expose filter deletes), and where it exists it is usually a distinct,
// cheaper API path than a filter scan. Consumers reach for it via a type
// assertion, so a backend that lacks it still satisfies [Store]:
//
//	if d, ok := store.(vectorstore.IDDeleter); ok {
//	    err := d.DeleteByIDs(ctx, []string{"a", "b"})
//	}
type IDDeleter interface {
	// DeleteByIDs removes the documents with the given ids. Unknown ids
	// are ignored (idempotent); an empty slice is a no-op.
	DeleteByIDs(ctx context.Context, ids []string) error
}
