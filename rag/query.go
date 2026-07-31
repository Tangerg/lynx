package rag

import (
	"errors"
	"fmt"
	"maps"
	"strings"
)

var (
	// ErrNilQuery reports a missing query at a RAG stage boundary.
	ErrNilQuery = errors.New("rag: query must not be nil")
	// ErrInvalidQuery reports an invalid query text or extension key.
	ErrInvalidQuery = errors.New("rag: invalid query")
	// ErrNilRetriever reports a missing retrieval capability.
	ErrNilRetriever = errors.New("rag: retriever must not be nil")
)

// ChatHistoryKey identifies chat history in a Query's extension values.
const ChatHistoryKey = "lynx:ai:rag:chat_history"

// Query is an immutable retrieval query. Text is required; extension values
// carry per-call filters and ambient context without allowing parallel stages
// to mutate shared query state.
type Query struct {
	text   string
	values map[string]any
}

// NewQuery constructs a query and rejects blank text.
func NewQuery(text string) (*Query, error) {
	query := &Query{text: text}
	if err := query.Validate(); err != nil {
		return nil, err
	}
	return query, nil
}

// Validate reports whether q contains usable retrieval text.
func (q *Query) Validate() error {
	if q == nil {
		return ErrNilQuery
	}
	if strings.TrimSpace(q.text) == "" {
		return fmt.Errorf("%w: text must not be blank", ErrInvalidQuery)
	}
	return nil
}

// Text returns the query text. A nil query returns an empty string.
func (q *Query) Text() string {
	if q == nil {
		return ""
	}
	return q.text
}

// Value returns the extension value stored under key.
func (q *Query) Value(key string) (any, bool) {
	if q == nil {
		return nil, false
	}
	value, found := q.values[key]
	return value, found
}

// Values returns a shallow copy of all extension values.
func (q *Query) Values() map[string]any {
	if q == nil {
		return nil
	}
	return maps.Clone(q.values)
}

// WithValue returns an independent query containing key and value.
func (q *Query) WithValue(key string, value any) (*Query, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	return q.withValues(map[string]any{key: value})
}

// WithText returns an independent query with replacement text.
func (q *Query) WithText(text string) (*Query, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("%w: text must not be blank", ErrInvalidQuery)
	}
	return &Query{text: text, values: maps.Clone(q.values)}, nil
}

func (q *Query) withValues(values map[string]any) (*Query, error) {
	if q == nil {
		return nil, ErrNilQuery
	}
	for key := range values {
		if strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("%w: extension key must not be blank", ErrInvalidQuery)
		}
	}
	clone := &Query{text: q.text, values: maps.Clone(q.values)}
	if clone.values == nil {
		clone.values = make(map[string]any, len(values))
	}
	maps.Copy(clone.values, values)
	return clone, nil
}

func (q *Query) withModelText(text string) *Query {
	if strings.TrimSpace(text) == "" {
		return &Query{text: q.text, values: maps.Clone(q.values)}
	}
	return &Query{text: text, values: maps.Clone(q.values)}
}
