package protocol

// PageQuery is the input shape for cursor-paginated list methods
// (API.md §4.11). Cursor is opaque to the client.
type PageQuery struct {
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

// Page is the wire response for cursor-paginated lists.
type Page[T any] struct {
	Data       []T    `json:"data"`
	NextCursor string `json:"nextCursor,omitempty"`
}
