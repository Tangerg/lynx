package rag

import (
	"errors"
	"maps"
)

// Sentinel errors for the rag contracts. Callers can match these with
// [errors.Is] to distinguish caller-side input errors (nil query)
// from store/model failures returned by downstream retrievers,
// transformers, augmenters.
var (
	// ErrNilQuery is returned when a nil [*Query] reaches a RAG
	// operation.
	ErrNilQuery = errors.New("rag: query must not be nil")

	// ErrNilRetriever is returned when a nil [Retriever] is passed to
	// a helper that requires one.
	ErrNilRetriever = errors.New("rag: retriever must not be nil")
)

// ChatHistoryKey is the [Query.Extra] key used by [NewMiddleware] and
// [NewCompressionTransformer] to pass chat history through the RAG boundary.
const ChatHistoryKey = "lynx:ai:rag:chat_history"

// Query is the canonical user-input shape passed through RAG components.
// [Query.Text] is required; [Query.Extra] exposes per-call metadata that
// components may read or write (filters, user ids, language tags, ...).
type Query struct {
	// Text is the natural-language query body.
	Text string

	extra map[string]any
}

// NewQuery builds a [Query]. Returns an error when text is empty.
func NewQuery(text string) (*Query, error) {
	if text == "" {
		return nil, errors.New("rag.NewQuery: text must not be empty")
	}
	return &Query{Text: text}, nil
}

func (q *Query) ensureExtra() {
	if q.extra == nil {
		q.extra = make(map[string]any)
	}
}

// Get returns the Extra value for key plus an existence flag. Safe to
// call concurrently with other Get calls; concurrent with Set is not.
func (q *Query) Get(key string) (any, bool) {
	if q.extra == nil {
		return nil, false
	}
	value, exists := q.extra[key]
	return value, exists
}

// Set stores value under key, allocating the metadata map when needed.
func (q *Query) Set(key string, value any) {
	q.ensureExtra()
	q.extra[key] = value
}

// Extra returns a shallow copy of the query metadata.
func (q *Query) Extra() map[string]any {
	return maps.Clone(q.extra)
}

// Clone returns a copy with an independent metadata map.
func (q *Query) Clone() *Query {
	return &Query{
		Text:  q.Text,
		extra: maps.Clone(q.extra),
	}
}

func (q *Query) withText(text string) (*Query, error) {
	if text == "" {
		return nil, errors.New("rag.Query: text must not be empty")
	}
	clone := q.Clone()
	clone.Text = text
	return clone, nil
}

func (q *Query) withModelText(text string) *Query {
	if text == "" {
		return q.Clone()
	}
	clone := q.Clone()
	clone.Text = text
	return clone
}
