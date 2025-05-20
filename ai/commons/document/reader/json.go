package reader

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"unicode"

	"github.com/Tangerg/lynx/ai/commons/document"
	pkgio "github.com/Tangerg/lynx/pkg/io"
)

var _ document.Reader = (*JSONReader)(nil)

// JSONReader provides an implementation of document.Reader that reads JSON data
// from an io.Reader source and converts it into Document objects.
type JSONReader struct {
	reader         io.Reader // The source to read JSON data from
	readBufferSize int       // Buffer size used when reading from the source
}

// maybeArray checks if the provided data appears to be a JSON array.
// It determines this by checking if the trimmed data starts with '[' and ends with ']'.
//
// Parameters:
//
//	data - Byte slice of potential JSON data to check
//
// Returns:
//
//	true if the data appears to be a JSON array, false otherwise
func (j *JSONReader) maybeArray(data []byte) bool {
	trimmedData := bytes.TrimFunc(data, unicode.IsSpace)
	if len(trimmedData) < 2 {
		return false
	}
	return trimmedData[0] == '[' && trimmedData[len(trimmedData)-1] == ']'
}

// tryParseToArray attempts to parse the input bytes as a JSON array and
// convert each array element into a Document.
//
// Parameters:
//
//	v - Byte slice containing JSON array data
//
// Returns:
//
//	A slice of Documents, each containing a single array element as text
//	An error if JSON parsing fails or document creation fails
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
		doc, err1 := document.
			NewBuilder().
			WithText(string(marshal)).
			Build()
		if err1 != nil {
			return nil, err1
		}
		rv = append(rv, doc)
	}
	return rv, nil
}

// Read reads JSON content from the configured reader and creates Document objects.
// If the content is a JSON array, it creates a separate Document for each array element.
// If not an array (or array parsing fails), it creates a single Document with the entire JSON content.
//
// Parameters:
//
//	ctx - Context for request cancellation and timeout (not used in this implementation)
//
// Returns:
//
//	A slice of Document objects created from the JSON content
//	An error if reading or parsing fails
func (j *JSONReader) Read(_ context.Context) ([]*document.Document, error) {
	v, err := pkgio.ReadAll(j.reader, j.readBufferSize)
	if err != nil {
		return nil, err
	}

	if j.maybeArray(v) {
		docs, err1 := j.tryParseToArray(v)
		if err1 == nil {
			return docs, nil
		}
		// If array parsing fails, fall through to process as a single document
	}

	var val any
	err = json.Unmarshal(v, &val)
	if err != nil {
		return nil, err
	}

	doc, err := document.
		NewBuilder().
		WithText(string(v)).
		Build()
	if err != nil {
		return nil, err
	}

	return []*document.Document{doc}, nil
}

// NewJSONReader creates a new JSONReader with the specified reader and buffer size.
// If no buffer size is provided or if the provided size is <= 0, a default size of 8192 is used.
//
// Parameters:
//
//	reader - The source to read JSON data from
//	sizes  - Optional buffer size for reading (uses the first value > 0)
//
// Returns:
//
//	A new JSONReader instance configured with the provided parameters
func NewJSONReader(reader io.Reader, sizes ...int) *JSONReader {
	var size = 8192 // Default buffer size
	if len(sizes) > 0 && sizes[0] > 0 {
		size = sizes[0]
	}
	return &JSONReader{
		reader:         reader,
		readBufferSize: size,
	}
}
