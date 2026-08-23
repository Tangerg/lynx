package protocol

// PageQuery is the input shape for cursor-paginated list methods
// (API.md §4.11). Cursor is opaque to the client.
type PageQuery struct {
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

// Page is the wire response for ALL list methods (API.md §4.11): the
// client reads `resp.data` everywhere, and the presence of `nextCursor`
// is the "has more" signal. A bounded local list leaves NextCursor empty
// but keeps the shape — one read path, no breaking growth to pagination.
type Page[T any] struct {
	Data       []T    `json:"data"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// NewPage wraps a fully-materialized (bounded) slice as a single page with no
// continuation cursor — the common case for local, non-paginated lists.
func NewPage[T any](data []T) *Page[T] { return NewPageWithCursor(data, "") }

// NewPageWithCursor wraps one page of a keyset read: its rows plus the cursor that
// continues it.
//
// Both constructors normalize nil to an empty slice, and building a Page any other
// way is a bug: a nil slice marshals to `null`, so a caller that assembled its rows
// with `var out []T` and matched nothing would put `"data": null` on the wire while
// every other list method sent `[]`. A client would then have to handle two shapes
// for "no rows" depending on which method it called.
func NewPageWithCursor[T any](data []T, nextCursor string) *Page[T] {
	if data == nil {
		data = []T{}
	}
	return &Page[T]{Data: data, NextCursor: nextCursor}
}
