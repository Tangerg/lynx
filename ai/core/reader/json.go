package reader

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/Tangerg/lynx/ai/core/document"
	"io"
	"unicode"
)

var _ document.Reader = (*JSONReader)(nil)

type JSONReader struct {
	reader io.Reader
}

func (j *JSONReader) maybeArray(data []byte) bool {
	trimmedData := bytes.TrimFunc(data, unicode.IsSpace)
	if len(trimmedData) < 2 {
		return false
	}
	return trimmedData[0] == '[' && trimmedData[len(trimmedData)-1] == ']'
}

func (j *JSONReader) tryParseToArray(v []byte) ([]*document.Document, error) {
	var array []any
	err := json.Unmarshal(v, &array)
	if err != nil {
		return nil, err
	}
	rv := make([]*document.Document, 0, len(array))
	for _, item := range array {
		marshal, err1 := json.Marshal(item)
		if err1 != nil {
			return nil, err1
		}
		rv = append(rv,
			document.
				NewBuilder().
				WithContent(string(marshal)).
				Build(),
		)
	}
	return rv, nil
}

func (j *JSONReader) Read(_ context.Context) ([]*document.Document, error) {
	v, err := io.ReadAll(j.reader)
	if err != nil {
		return nil, err
	}
	if j.maybeArray(v) {
		docs, err1 := j.tryParseToArray(v)
		if err1 == nil {
			return docs, nil
		}
	}
	return []*document.Document{
		document.
			NewBuilder().
			WithContent(string(v)).
			Build(),
	}, nil
}

func NewJSONReader(reader io.Reader) *JSONReader {
	return &JSONReader{reader: reader}
}
