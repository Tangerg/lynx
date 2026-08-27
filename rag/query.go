package rag

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"strings"

	"github.com/samber/lo"
)

var (
	// ErrNilQuery reports a missing query at a RAG stage boundary.
	ErrNilQuery = errors.New("rag: query must not be nil")
	// ErrInvalidQuery reports an invalid query text or extension key.
	ErrInvalidQuery = errors.New("rag: invalid query")
	// ErrNilRetriever reports a missing retrieval capability.
	ErrNilRetriever = errors.New("rag: retriever must not be nil")
	// ErrInvalidQueryValueKey reports an empty, whitespace-padded, zero-value,
	// or untyped query value key.
	ErrInvalidQueryValueKey = errors.New("rag: invalid query value key")
	// ErrQueryValueTypeMismatch reports the same key name used with different
	// declared value types.
	ErrQueryValueTypeMismatch = errors.New("rag: query value type mismatch")
	// ErrNilQueryValue reports an ambiguous nil query value.
	ErrNilQueryValue = errors.New("rag: query value must not be nil")
)

var queryAnyType = reflect.TypeFor[any]()

// ValueKey is a typed, named slot in a [Query]. Define keys once and share the
// value with the code that writes and reads the slot. A key's name is its
// identity; reusing a name with another T is an explicit error.
//
// The zero value and ValueKey[any] are invalid. Use [NewValueKey] or
// [MustValueKey] with a concrete value type.
type ValueKey[T any] struct {
	name string
	typ  reflect.Type
}

// NewValueKey validates name and returns a typed query value key.
func NewValueKey[T any](name string) (ValueKey[T], error) {
	key := ValueKey[T]{name: name, typ: reflect.TypeFor[T]()}
	if err := key.validate(); err != nil {
		return ValueKey[T]{}, err
	}
	return key, nil
}

// MustValueKey is the declaration-time companion to [NewValueKey].
func MustValueKey[T any](name string) ValueKey[T] {
	key, err := NewValueKey[T](name)
	if err != nil {
		panic(err)
	}
	return key
}

// Name returns the stable diagnostic identity of the key.
func (v ValueKey[T]) Name() string { return v.name }

func (v ValueKey[T]) validate() error {
	switch {
	case v.name == "", v.name != strings.TrimSpace(v.name), v.typ == nil:
		return fmt.Errorf(
			"%w: name must be non-empty without surrounding whitespace",
			ErrInvalidQueryValueKey,
		)
	case v.typ == queryAnyType:
		return fmt.Errorf("%w: value type must not be any", ErrInvalidQueryValueKey)
	default:
		return nil
	}
}

type queryValue struct {
	typ   reflect.Type
	value any
}

// Query is a persistent retrieval query: Text is required, and WithText or
// WithValue returns a new envelope with an independent top-level value map.
// Referenced values remain caller-owned and must be treated as read-only when
// the same query is used by parallel retrieval stages.
type Query struct {
	text   string
	values map[string]queryValue
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

// Value returns the value stored under key. Missing values are distinct from
// invalid keys and same-name/different-type collisions.
func (q *Query) Value[T any](key ValueKey[T]) (T, bool, error) {
	var zero T
	if err := q.Validate(); err != nil {
		return zero, false, err
	}
	if err := key.validate(); err != nil {
		return zero, false, err
	}
	entry, found := q.values[key.name]
	if !found {
		return zero, false, nil
	}
	if entry.typ != key.typ {
		return zero, false, fmt.Errorf(
			"%w for %q: stored %s, requested %s",
			ErrQueryValueTypeMismatch,
			key.name,
			entry.typ,
			key.typ,
		)
	}
	value, ok := entry.value.(T)
	if !ok {
		return zero, false, fmt.Errorf(
			"%w for %q: stored %T, requested %s",
			ErrQueryValueTypeMismatch,
			key.name,
			entry.value,
			key.typ,
		)
	}
	return value, true, nil
}

// WithValue returns an independent query containing value under key.
func (q *Query) WithValue[T any](key ValueKey[T], value T) (*Query, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	if err := key.validate(); err != nil {
		return nil, err
	}
	if lo.IsNil(value) {
		return nil, fmt.Errorf("%w for %q", ErrNilQueryValue, key.name)
	}
	if current, found := q.values[key.name]; found && current.typ != key.typ {
		return nil, fmt.Errorf(
			"%w for %q: stored %s, new %s",
			ErrQueryValueTypeMismatch,
			key.name,
			current.typ,
			key.typ,
		)
	}
	clone := &Query{text: q.text, values: maps.Clone(q.values)}
	if clone.values == nil {
		clone.values = make(map[string]queryValue, 1)
	}
	clone.values[key.name] = queryValue{typ: key.typ, value: value}
	return clone, nil
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
