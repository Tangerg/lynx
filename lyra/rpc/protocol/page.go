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

// NewPage wraps a fully-materialized (bounded) slice as a single page with
// no continuation cursor — the common case for local, non-paginated lists.
// data is normalized to a non-nil empty slice so the wire carries `[]`, not
// `null`. Methods that genuinely paginate set NextCursor themselves.
func NewPage[T any](data []T) *Page[T] {
	if data == nil {
		data = []T{}
	}
	return &Page[T]{Data: data}
}

// WorkspaceListQuery is WorkspaceQuery + pagination — the input for the
// paginated workspace list reads (API.md §7.5: `WorkspaceQuery & { cursor?,
// limit? }`). The non-list reads (getDiff / getFileHead / grep) keep their
// own request structs.
type WorkspaceListQuery struct {
	WorkspaceQuery
	PageQuery
}
