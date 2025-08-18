package readers

import (
	"context"
	"io"

	"github.com/Tangerg/lynx/ai/content/document"
	pkgio "github.com/Tangerg/lynx/pkg/io"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

var _ document.Reader = (*TextReader)(nil)

type TextReader struct {
	reader     io.Reader
	bufferSize int
}

func (t *TextReader) Read(_ context.Context) ([]*document.Document, error) {
	data, err := pkgio.ReadAll(t.reader, t.bufferSize)
	if err != nil {
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

func NewTextReader(reader io.Reader, sizes ...int) *TextReader {
	const defaultBufferSize = 8192

	bufferSize, _ := pkgSlices.First(sizes)
	if bufferSize <= 0 {
		bufferSize = defaultBufferSize
	}

	return &TextReader{
		reader:     reader,
		bufferSize: bufferSize,
	}
}
