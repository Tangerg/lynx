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

const defaultJSONReaderBufferSize = 8192

var _ Reader = (*JSONReader)(nil)

// JSONReader parses a JSON payload — either a single object or a top-
// level array — into [*Document] entries. Top-level arrays produce one
// document per element; single objects produce one document whose Text
// is the raw JSON string.
//
// Use it to ingest API responses, dump files, or seed fixture data.
//
// Example:
//
//	r, err := document.NewJSONReader(strings.NewReader(`[{"id":1},{"id":2}]`))
//	docs, err := r.Read(ctx) // 2 documents
type JSONReader struct {
	reader     io.Reader
	bufferSize int
}

func NewJSONReader(reader io.Reader, sizes ...int) (*JSONReader, error) {
	if reader == nil {
		return nil, errors.New("document.NewJSONReader: reader must not be nil")
	}

	bufferSize, _ := pkgSlices.First(sizes)
	if bufferSize <= 0 {
		bufferSize = defaultJSONReaderBufferSize
	}

	return &JSONReader{reader: reader, bufferSize: bufferSize}, nil
}

func (j *JSONReader) Read(_ context.Context) ([]*Document, error) {
	data, err := pkgio.ReadAll(j.reader, j.bufferSize)
	if err != nil {
		return nil, err
	}

	if j.looksLikeArray(data) {
		if docs, parseErr := j.parseAsArray(data); parseErr == nil {
			return docs, nil
		}
		// fall through to single-document path on array decode failure;
		// the caller's payload may be an array of unsupported items, in
		// which case wrapping the raw bytes is still useful.
	}

	if err = json.Unmarshal(data, new(any)); err != nil {
		return nil, err
	}

	doc, err := NewDocument(string(data), nil)
	if err != nil {
		return nil, err
	}
	return []*Document{doc}, nil
}

func (j *JSONReader) looksLikeArray(data []byte) bool {
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

	docs := make([]*Document, 0, len(items))
	for _, item := range items {
		bytes, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}

		doc, err := NewDocument(string(bytes), nil)
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, nil
}
