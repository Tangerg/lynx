package documentreaders

import (
	"context"
	"errors"
	"io"

	"github.com/Tangerg/lynx/core/document"
	pkgio "github.com/Tangerg/lynx/pkg/io"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

const defaultTextReaderBufferSize = 8192

// TextReader reads the entire contents of an [io.Reader] and packages
// it into one [*document.Document]. Use it for files, in-memory buffers, or
// network streams that fit comfortably in memory; for very large
// inputs run a splitter ([transformer_text_splitter.go],
// [transformer_token_splitter.go]) afterwards.
type TextReader struct {
	reader     io.Reader
	bufferSize int
}

func NewTextReader(reader io.Reader, sizes ...int) (*TextReader, error) {
	if reader == nil {
		return nil, errors.New("documentreaders.NewTextReader: reader must not be nil")
	}

	bufferSize, _ := pkgSlices.First(sizes)
	if bufferSize <= 0 {
		bufferSize = defaultTextReaderBufferSize
	}

	return &TextReader{reader: reader, bufferSize: bufferSize}, nil
}

func (t *TextReader) Read(_ context.Context) ([]*document.Document, error) {
	data, err := pkgio.ReadAll(t.reader, t.bufferSize)
	if err != nil {
		return nil, err
	}

	doc, err := document.NewDocument(string(data), nil)
	if err != nil {
		return nil, err
	}
	return []*document.Document{doc}, nil
}
