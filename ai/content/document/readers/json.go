package readers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"unicode"

	"github.com/Tangerg/lynx/ai/content/document"
	pkgio "github.com/Tangerg/lynx/pkg/io"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

var _ document.Reader = (*JSONReader)(nil)

type JSONReader struct {
	reader     io.Reader
	bufferSize int
}

func (j *JSONReader) maybeJSONArray(data []byte) bool {
	trimmed := bytes.TrimFunc(data, unicode.IsSpace)
	if len(trimmed) < 2 {
		return false
	}
	return trimmed[0] == '[' && trimmed[len(trimmed)-1] == ']'
}

func (j *JSONReader) parseAsArray(data []byte) ([]*document.Document, error) {
	var items []any
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}

	documents := make([]*document.Document, 0, len(items))

	for _, item := range items {
		itemBytes, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}

		doc, err := document.NewBuilder().
			WithText(string(itemBytes)).
			Build()
		if err != nil {
			return nil, err
		}

		documents = append(documents, doc)
	}

	return documents, nil
}

func (j *JSONReader) Read(_ context.Context) ([]*document.Document, error) {
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

	doc, err := document.NewBuilder().
		WithText(string(data)).
		Build()
	if err != nil {
		return nil, err
	}

	return []*document.Document{doc}, nil
}

func NewJSONReader(reader io.Reader, sizes ...int) *JSONReader {
	const defaultBufferSize = 8192

	bufferSize, _ := pkgSlices.First(sizes)
	if bufferSize <= 0 {
		bufferSize = defaultBufferSize
	}

	return &JSONReader{
		reader:     reader,
		bufferSize: bufferSize,
	}
}
