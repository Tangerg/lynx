package reader

import (
	"context"
	"errors"
	"github.com/Tangerg/lynx/ai/core/document"
	"io"
	"strings"
)

var _ document.Reader = (*TextReader)(nil)

type TextReader struct {
	reader         io.Reader
	readBufferSize int
}

func (t *TextReader) Read(_ context.Context) ([]*document.Document, error) {
	buffer := make([]byte, 0, t.readBufferSize)
	sb := strings.Builder{}
	for {
		_, err := t.reader.Read(buffer)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		sb.Write(buffer)
	}
	return []*document.Document{
		document.NewBuilder().
			WithContent(sb.String()).
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
