package reader

import (
	"context"
	"io"

	"github.com/Tangerg/lynx/ai/commons/document"
	pkgio "github.com/Tangerg/lynx/pkg/io"
)

var _ document.Reader = (*TextReader)(nil)

// TextReader provides an implementation of document.Reader that reads text from
// an io.Reader source and converts it into a Document object.
type TextReader struct {
	reader         io.Reader // The source to read text from
	readBufferSize int       // Buffer size used when reading from the source
}

// Read reads all content from the configured reader and creates a Document with
// the text content. The context parameter is not used in this implementation
// but is required by the document.Reader interface.
//
// Parameters:
//
//	ctx - Context for request cancellation and timeout (not used in this implementation)
//
// Returns:
//
//	A slice containing a single Document with the text content
//	An error if reading fails or document creation fails
func (t *TextReader) Read(_ context.Context) ([]*document.Document, error) {
	buffer, err := pkgio.ReadAll(t.reader, t.readBufferSize)
	if err != nil {
		return nil, err
	}

	doc, err := document.NewBuilder().
		WithText(string(buffer)).
		Build()
	if err != nil {
		return nil, err
	}

	return []*document.Document{doc}, nil
}

// NewTextReader creates a new TextReader with the specified reader and buffer size.
// If no buffer size is provided or if the provided size is <= 0, a default size of 8192 is used.
//
// Parameters:
//
//	reader - The source to read text from
//	sizes  - Optional buffer size for reading (uses the first value > 0)
//
// Returns:
//
//	A new TextReader instance configured with the provided parameters
func NewTextReader(reader io.Reader, sizes ...int) *TextReader {
	var size = 8192 // Default buffer size
	for _, s := range sizes {
		if s > 0 {
			size = s
			break
		}
	}
	return &TextReader{
		reader:         reader,
		readBufferSize: size,
	}
}
