package document

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"unicode"

	pkgio "github.com/Tangerg/lynx/pkg/io"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

var _ Reader = (*JSONReader)(nil)

// JSONReader reads and parses JSON data into Document objects.
//
// This reader is useful for:
//   - Loading documents from JSON files or API responses
//   - Processing both single JSON objects and JSON arrays
//   - Converting structured JSON data into document format
//   - Building document ingestion pipelines from JSON sources
//
// The reader automatically detects whether the input is a JSON array or
// a single JSON object. For arrays, each element becomes a separate document.
// For single objects, the entire JSON becomes one document's content.
//
// Example JSON array input:
//
//	[
//	  {"title": "Doc1", "content": "..."},
//	  {"title": "Doc2", "content": "..."}
//	]
//
// Example single JSON object input:
//
//	{"title": "Doc1", "content": "...", "metadata": {...}}
type JSONReader struct {
	reader     io.Reader
	bufferSize int
}

func NewJSONReader(reader io.Reader, sizes ...int) (*JSONReader, error) {
	if reader == nil {
		return nil, errors.New("reader is nil")
	}
	const defaultBufferSize = 8192

	bufferSize, _ := pkgSlices.First(sizes)
	if bufferSize <= 0 {
		bufferSize = defaultBufferSize
	}

	return &JSONReader{
		reader:     reader,
		bufferSize: bufferSize,
	}, nil
}

func (j *JSONReader) maybeJSONArray(data []byte) bool {
	trimmed := bytes.TrimFunc(data, unicode.IsSpace)
	if len(trimmed) < 2 {
		return false
	}
	return trimmed[0] == '[' && trimmed[len(trimmed)-1] == ']'
}

func (j *JSONReader) parseAsArray(data []byte) ([]*Document, error) {
	var items []any
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}

	documents := make([]*Document, 0, len(items))

	for _, item := range items {
		itemBytes, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}

		doc, err := NewDocument(string(itemBytes), nil)
		if err != nil {
			return nil, err
		}

		documents = append(documents, doc)
	}

	return documents, nil
}

func (j *JSONReader) Read(_ context.Context) ([]*Document, error) {
	data, err := pkgio.ReadAll(j.reader, j.bufferSize)
	if err != nil {
		return nil, err
	}

	if j.maybeJSONArray(data) {
		if docs, err1 := j.parseAsArray(data); err1 == nil {
			return docs, nil
		}
	}

	var value any
	if err = json.Unmarshal(data, &value); err != nil {
		return nil, err
	}

	doc, err := NewDocument(string(data), nil)
	if err != nil {
		return nil, err
	}

	return []*Document{doc}, nil
}
