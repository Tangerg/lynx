package rag

import (
	"errors"
	"maps"
)

// Query is the canonical user-input shape that flows through the RAG
// pipeline. [Query.Text] is required; [Query.Extra] holds per-call
// metadata that pipeline stages may read or write (filters, user ids,
// language tags, ...).
type Query struct {
	// Text is the natural-language query body.
	Text string

	// Extra carries free-form per-call metadata.
	Extra map[string]any
}

// NewQuery builds a [Query]. Returns an error when text is empty.
func NewQuery(text string) (*Query, error) {
	if text == "" {
		return nil, errors.New("rag.NewQuery: text must not be empty")
	}
	return &Query{Text: text}, nil
}

// ensureExtra lazily allocates Extra. Used by [Query.Set] only — Get
// must not mutate state since concurrent reads are valid.
func (q *Query) ensureExtra() {
	if q.Extra == nil {
		q.Extra = make(map[string]any)
	}
}

// Get returns the Extra value for key plus an existence flag. Safe to
// call concurrently with other Get calls; concurrent with Set is not.
func (q *Query) Get(key string) (any, bool) {
	if q.Extra == nil {
		return nil, false
	}
	value, exists := q.Extra[key]
	return value, exists
}

// Set stores value under key in Extra.
func (q *Query) Set(key string, value any) {
	q.ensureExtra()
	q.Extra[key] = value
}

// Clone returns a deep copy with an independent Extra map.
func (q *Query) Clone() *Query {
	return &Query{
		Text:  q.Text,
		Extra: maps.Clone(q.Extra),
	}
}
