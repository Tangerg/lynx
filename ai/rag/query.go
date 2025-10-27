package rag

import (
	"maps"
)

// Query represents a user query with its text content and additional metadata.
// It is used throughout the RAG pipeline for query transformation, expansion,
// retrieval, and augmentation operations.
type Query struct {
	// Text is the main content of the query.
	Text string

	// Extra holds additional metadata associated with the query,
	// such as filters, parameters, or any contextual information.
	Extra map[string]any
}

func (q *Query) ensureExtra() {
	if q.Extra == nil {
		q.Extra = make(map[string]any)
	}
}

func (q *Query) Get(key string) (any, bool) {
	q.ensureExtra()
	value, exists := q.Extra[key]
	return value, exists
}

func (q *Query) Set(key string, value any) {
	q.ensureExtra()
	q.Extra[key] = value
}

func (q *Query) Clone() *Query {
	return &Query{
		Text:  q.Text,
		Extra: maps.Clone(q.Extra),
	}
}
