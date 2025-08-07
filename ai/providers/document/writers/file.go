package writers

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/ai/content/document"
)

const (
	MetadataStartPageNumber = "start_page_number"
	MetadataEndPageNumber   = "end_page_number"
)

var _ document.Writer = (*FileWriter)(nil)

type FileWriter struct {
	path                string
	withDocumentMarkers bool
	metadataMode        document.MetadataMode
	formatter           document.Formatter
	appendMode          bool
}

func (f *FileWriter) getOpenFileFlags() int {
	const (
		createWriteTrunc  = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		createWriteAppend = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	)

	if f.appendMode {
		return createWriteAppend
	}
	return createWriteTrunc
}

func (f *FileWriter) formatDocumentContent(index int, doc *document.Document) string {
	var builder strings.Builder

	if f.withDocumentMarkers {
		builder.WriteString("### Index: ")
		builder.WriteString(strconv.Itoa(index))
		builder.WriteString(", ID: ")
		builder.WriteString(doc.ID())
		builder.WriteString(", Pages:[")

		metadata := doc.Metadata()
		builder.WriteString(cast.ToString(metadata[MetadataStartPageNumber]))
		builder.WriteString(",")
		builder.WriteString(cast.ToString(metadata[MetadataEndPageNumber]))
		builder.WriteString("]")
		builder.WriteString("\n")
	}

	builder.WriteString(doc.FormatByMetadataModeWithFormatter(f.metadataMode, f.formatter))
	builder.WriteString("\n\n")

	return builder.String()
}

func (f *FileWriter) writeDocumentsToFile(docs []*document.Document, file *os.File) error {
	const batchSize = 5
	var buffer strings.Builder

	for i, doc := range docs {
		buffer.WriteString(f.formatDocumentContent(i, doc))

		if (i+1)%batchSize == 0 {
			if _, err := file.WriteString(buffer.String()); err != nil {
				return err
			}
			buffer.Reset()
		}
	}

	if buffer.Len() > 0 {
		_, err := file.WriteString(buffer.String())
		return err
	}

	return nil
}

func (f *FileWriter) Write(_ context.Context, docs []*document.Document) error {
	file, err := os.OpenFile(f.path, f.getOpenFileFlags(), 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	return f.writeDocumentsToFile(docs, file)
}

func NewFileWriterBuilder() *FileWriterBuilder {
	return &FileWriterBuilder{
		metadataMode: document.MetadataModeNone,
		formatter:    document.NewDefaultFormatterBuilder().Build(),
	}
}

type FileWriterBuilder struct {
	path                string
	withDocumentMarkers bool
	metadataMode        document.MetadataMode
	formatter           document.Formatter
	appendMode          bool
}

func (b *FileWriterBuilder) WithPath(path string) *FileWriterBuilder {
	b.path = path
	return b
}

func (b *FileWriterBuilder) WithDocumentMarkers() *FileWriterBuilder {
	b.withDocumentMarkers = true
	return b
}

func (b *FileWriterBuilder) WithAppendMode() *FileWriterBuilder {
	b.appendMode = true
	return b
}

func (b *FileWriterBuilder) WithMetadataMode(mode document.MetadataMode) *FileWriterBuilder {
	b.metadataMode = mode
	return b
}

func (b *FileWriterBuilder) WithFormatter(formatter document.Formatter) *FileWriterBuilder {
	if formatter != nil {
		b.formatter = formatter
	}
	return b
}

func (b *FileWriterBuilder) Build() (*FileWriter, error) {
	if b.path == "" {
		return nil, errors.New("file path must not be empty")
	}

	return &FileWriter{
		path:                b.path,
		withDocumentMarkers: b.withDocumentMarkers,
		metadataMode:        b.metadataMode,
		formatter:           b.formatter,
		appendMode:          b.appendMode,
	}, nil
}
