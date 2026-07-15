package vectorstore

import (
	"context"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
)

// FilterDeleter removes documents selected by a metadata expression. It is a
// separate capability because some providers can search but cannot mutate
// their managed index.
type FilterDeleter interface {
	// DeleteWhere removes every document matching predicate. Implementations return
	// [ErrMissingFilter] for nil and reject invalid expressions.
	DeleteWhere(ctx context.Context, predicate filter.Predicate) error
}

// IDDeleter removes documents by identifier. It is independent from
// [FilterDeleter]: providers frequently expose only one of the two paths.
type IDDeleter interface {
	// DeleteIDs removes the documents with the given ids. Unknown ids
	// are ignored (idempotent); an empty slice is a no-op.
	DeleteIDs(ctx context.Context, ids []string) error
}
