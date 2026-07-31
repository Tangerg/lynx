package documentreaders

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/Tangerg/lynx/core/document"
)

// JSONReader parses a JSON payload — either a single object or a top-
// level array — into [*document.Document] entries. Top-level arrays produce one
// document per element; single objects produce one document whose Text
// is the raw JSON string.
//
// Use it to ingest API responses, dump files, or seed fixture data.
//
// Example:
//
//	r, err := documentreaders.NewJSONReader(strings.NewReader(`[{"id":1},{"id":2}]`))
//	docs, err := r.Read(ctx) // 2 documents
type JSONReader struct {
	reader io.Reader
}

func NewJSONReader(reader io.Reader) (*JSONReader, error) {
	if isNil(reader) {
		return nil, errors.New("document readers: JSON source must not be nil")
	}
	return &JSONReader{reader: reader}, nil
}

func (j *JSONReader) Read(ctx context.Context) ([]*document.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(j.reader)
	if err != nil {
		return nil, fmt.Errorf("document readers: read JSON source: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		return parseJSONArray(ctx, trimmed)
	}
	if err := json.Unmarshal(trimmed, new(any)); err != nil {
		return nil, fmt.Errorf("document readers: decode JSON source: %w", err)
	}

	doc, err := document.NewDocument(string(trimmed), nil)
	if err != nil {
		return nil, fmt.Errorf("document readers: build JSON document: %w", err)
	}
	return []*document.Document{doc}, nil
}

func parseJSONArray(ctx context.Context, data []byte) ([]*document.Document, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("document readers: decode JSON array: %w", err)
	}

	docs := make([]*document.Document, 0, len(items))
	for index, item := range items {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		doc, err := document.NewDocument(string(item), nil)
		if err != nil {
			return nil, fmt.Errorf("document readers: build JSON array document %d: %w", index, err)
		}
		docs = append(docs, doc)
	}
	return docs, nil
}
