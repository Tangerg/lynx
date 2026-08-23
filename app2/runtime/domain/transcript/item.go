// Package transcript owns durable user-visible facts independently of wire
// presentation. Rich item payloads remain opaque to the store.
package transcript

import "time"

type Record struct {
	ID, SessionID, RunID string
	Ordinal              int
	Body                 []byte
	SearchText           SearchableText
	CreatedAt            time.Time
}

type Order string

const (
	OrderAscending  Order = "asc"
	OrderDescending Order = "desc"
)

type Scope struct {
	SessionID         string
	RunID             string
	IncludeDescendants bool
}

type Cursor struct {
	CreatedAt time.Time
	RunID     string
	Ordinal   int
}

type Query struct {
	Scope  Scope
	Order  Order
	Limit  int
	Cursor *Cursor
}

type Page struct {
	Records []Record
	Next    *Cursor
}
