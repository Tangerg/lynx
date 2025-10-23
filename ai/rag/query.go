package rag

import (
	"maps"
	"slices"

	"github.com/Tangerg/lynx/ai/model/chat"
)

type Query struct {
	Text    string
	History []chat.Message
	Extra   map[string]any
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
		Text:    q.Text,
		History: slices.Clone(q.History),
		Extra:   maps.Clone(q.Extra),
	}
}
