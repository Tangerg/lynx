// Package json reads JSON values into documents.
package json

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/Tangerg/lynx/core/document"
)

// Reader parses a JSON payload — either a single object or a top-
// level array — into [*document.Document] entries. Top-level arrays produce one
// document per element; single objects produce one document whose Text
// is the raw JSON string.
//
// Use it to ingest API responses, dump files, or seed fixture data.
//
// Example:
//
//	r, err := json.New(strings.NewReader(`[{"id":1},{"id":2}]`))
//	docs, err := r.Read(ctx) // 2 documents
type Reader struct {
	reader io.Reader
}

// New constructs a JSON Reader from source.
func New(reader io.Reader) (*Reader, error) {
	if isNil(reader) {
		return nil, errors.New("JSON reader: source must not be nil")
	}
	return &Reader{reader: reader}, nil
}

func isNil(value any) bool {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return true
	}
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

// Read consumes the source and converts its top-level value to documents.
func (j *Reader) Read(ctx context.Context) ([]*document.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(j.reader)
	if err != nil {
		return nil, fmt.Errorf("JSON reader: read source: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		return parseJSONArray(ctx, trimmed)
	}
	if err := json.Unmarshal(trimmed, new(any)); err != nil {
		return nil, fmt.Errorf("JSON reader: decode source: %w", err)
	}

	doc, err := document.NewDocument(string(trimmed), nil)
	if err != nil {
		return nil, fmt.Errorf("JSON reader: build document: %w", err)
	}
	return []*document.Document{doc}, nil
}

func parseJSONArray(ctx context.Context, data []byte) ([]*document.Document, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("JSON reader: decode array: %w", err)
	}

	docs := make([]*document.Document, 0, len(items))
	for index, item := range items {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		doc, err := document.NewDocument(string(item), nil)
		if err != nil {
			return nil, fmt.Errorf("JSON reader: build array document %d: %w", index, err)
		}
		docs = append(docs, doc)
	}
	return docs, nil
}
