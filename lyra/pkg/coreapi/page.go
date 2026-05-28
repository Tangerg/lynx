package coreapi

// PageQuery is the input shape for every list method (API.md §6.4).
// limit caps the page size (server enforces a max of 100, default 20);
// cursor is opaque to the client — passed back from a previous
// Page.NextCursor.
type PageQuery struct {
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

// Page is the wire response for every list method.
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
	HasMore    bool   `json:"hasMore"`
}
