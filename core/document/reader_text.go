package document

import (
	"context"
	"errors"
	"io"

	pkgio "github.com/Tangerg/lynx/pkg/io"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

const defaultTextReaderBufferSize = 8192

var _ Reader = (*TextReader)(nil)

// TextReader reads the entire contents of an [io.Reader] and packages
// it into one [*Document]. Use it for files, in-memory buffers, or
// network streams that fit comfortably in memory; for very large
// inputs run a splitter ([transformer_text_splitter.go],
// [transformer_token_splitter.go]) afterwards.
type TextReader struct {
	reader     io.Reader
	bufferSize int
}

// NewTextReader builds a [TextReader] over reader. Optional sizes[0]
// overrides the default 8 KiB read buffer; non-positive values fall
// back to the default.
func NewTextReader(reader io.Reader, sizes ...int) (*TextReader, error) {
	if reader == nil {
		return nil, errors.New("document.NewTextReader: reader must not be nil")
	}

	bufferSize, _ := pkgSlices.First(sizes)
	if bufferSize <= 0 {
		bufferSize = defaultTextReaderBufferSize
	}

	return &TextReader{reader: reader, bufferSize: bufferSize}, nil
}

// Read consumes the underlying reader fully and returns one [*Document]
// wrapping the textual content.
func (t *TextReader) Read(_ context.Context) ([]*Document, error) {
	data, err := pkgio.ReadAll(t.reader, t.bufferSize)
	if err != nil {
		return nil, err
	}

	doc, err := NewDocument(string(data), nil)
	if err != nil {
		return nil, err
	}
	return []*Document{doc}, nil
}
