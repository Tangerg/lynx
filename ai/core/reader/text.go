package reader

import (
	"context"
	"io"

	"github.com/Tangerg/lynx/ai/core/document"
	pkgio "github.com/Tangerg/lynx/pkg/io"
)

var _ document.Reader = (*TextReader)(nil)

type TextReader struct {
	reader         io.Reader
	readBufferSize int
}

func (t *TextReader) Read(_ context.Context) ([]*document.Document, error) {
	buffer, err := pkgio.ReadAll(t.reader, t.readBufferSize)
	if err != nil {
		return nil, err
	}
	return []*document.Document{
		document.NewBuilder().
			WithContent(string(buffer)).
			Build(),
	}, nil
}

func NewTextReader(reader io.Reader, sizes ...int) *TextReader {
	var size = 8192
	if len(sizes) > 0 && sizes[0] > 0 {
		size = sizes[0]
	}
	return &TextReader{
		reader:         reader,
		readBufferSize: size,
	}
}
