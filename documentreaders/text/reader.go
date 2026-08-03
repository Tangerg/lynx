// Package text reads plain text into documents.
package text

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/Tangerg/lynx/core/document"
)

// Reader reads the entire contents of an [io.Reader] and packages
// it into one [*document.Document]. Use it for files, in-memory buffers, or
// network streams that fit comfortably in memory.
type Reader struct {
	reader io.Reader
}

// New constructs a text Reader from source.
func New(reader io.Reader) (*Reader, error) {
	if isNil(reader) {
		return nil, errors.New("text reader: source must not be nil")
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

// Read consumes the source and returns one document containing its text.
func (t *Reader) Read(ctx context.Context) ([]*document.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(t.reader)
	if err != nil {
		return nil, fmt.Errorf("text reader: read source: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	doc, err := document.NewDocument(string(data), nil)
	if err != nil {
		return nil, fmt.Errorf("text reader: build document: %w", err)
	}
	return []*document.Document{doc}, nil
}
