package documentreaders

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/Tangerg/lynx/core/document"
)

// TextReader reads the entire contents of an [io.Reader] and packages
// it into one [*document.Document]. Use it for files, in-memory buffers, or
// network streams that fit comfortably in memory; for very large
// inputs run a splitter ([transformer_text_splitter.go],
// [transformer_token_splitter.go]) afterwards.
type TextReader struct {
	reader io.Reader
}

func NewTextReader(reader io.Reader) (*TextReader, error) {
	if isNil(reader) {
		return nil, errors.New("document readers: text source must not be nil")
	}
	return &TextReader{reader: reader}, nil
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

func (t *TextReader) Read(ctx context.Context) ([]*document.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(t.reader)
	if err != nil {
		return nil, fmt.Errorf("document readers: read text source: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	doc, err := document.NewDocument(string(data), nil)
	if err != nil {
		return nil, fmt.Errorf("document readers: build text document: %w", err)
	}
	return []*document.Document{doc}, nil
}
