package document

import (
	"context"
	"errors"
	"io"

	pkgio "github.com/Tangerg/lynx/pkg/io"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

var _ Reader = (*TextReader)(nil)

// TextReader reads text content from an io.Reader and converts it into a Document.
//
// This reader is useful for:
//   - Loading documents from files, network streams, or in-memory buffers
//   - Processing simple text documents without complex parsing requirements
//   - Building document ingestion pipelines with standard Go io interfaces
//   - Reading text data with configurable buffer sizes for memory optimization
//
// The reader loads the entire content into memory and creates a single Document
// with the text content. For large files or streaming scenarios, consider using
// chunking strategies or alternative readers.
type TextReader struct {
	reader     io.Reader
	bufferSize int
}

func NewTextReader(reader io.Reader, sizes ...int) (*TextReader, error) {
	if reader == nil {
		return nil, errors.New("reader is nil")
	}
	const defaultBufferSize = 8192

	bufferSize, _ := pkgSlices.First(sizes)
	if bufferSize <= 0 {
		bufferSize = defaultBufferSize
	}

	return &TextReader{
		reader:     reader,
		bufferSize: bufferSize,
	}, nil
}

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
