package writer

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/ai/core/document"
	pkgSystem "github.com/Tangerg/lynx/pkg/system"
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
	appendMode          bool
}

func (f *FileWriter) getFileFlag() int {
	const (
		openAndTruncFlag  = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		openAndAppendFlag = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	)

	if f.appendMode {
		return openAndAppendFlag
	}
	return openAndTruncFlag
}

func (f *FileWriter) getDocContent(index int, doc *document.Document) string {
	var sb strings.Builder
	if f.withDocumentMarkers {
		sb.WriteString("### Index: ")
		sb.WriteString(strconv.Itoa(index))
		sb.WriteString(", Id: ")
		sb.WriteString(doc.Id())
		sb.WriteString(", Pages:[")
		sb.WriteString(cast.ToString(doc.Metadata()[MetadataStartPageNumber]))
		sb.WriteString(",")
		sb.WriteString(cast.ToString(doc.Metadata()[MetadataEndPageNumber]))
		sb.WriteString("]")
		sb.WriteString(pkgSystem.LineSeparator())
	}
	sb.WriteString(doc.FormattedContentByMetadataMode(f.metadataMode))
	sb.WriteString(pkgSystem.LineSeparator())
	sb.WriteString(pkgSystem.LineSeparator())
	return sb.String()
}

func (f *FileWriter) writeDocs(docs []*document.Document, file *os.File) error {
	var sb strings.Builder
	for i, doc := range docs {
		sb.WriteString(f.getDocContent(i, doc))
		if i%5 != 4 {
			continue
		}
		_, err := file.WriteString(sb.String())
		if err != nil {
			return err
		}
		sb.Reset()
	}

	if sb.Len() == 0 {
		return nil
	}

	_, err := file.WriteString(sb.String())
	return err
}

func (f *FileWriter) Write(_ context.Context, docs []*document.Document) error {
	file, err := os.OpenFile(f.path, f.getFileFlag(), 0666)
	if err != nil {
		return err
	}
	defer file.Close()
	return f.writeDocs(docs, file)
}

func NewFileWriterBuilder() *FileWriterBuilder {
	return &FileWriterBuilder{
		fileWriter: &FileWriter{
			metadataMode: document.None,
		},
	}
}

type FileWriterBuilder struct {
	fileWriter *FileWriter
}

func (f *FileWriterBuilder) WithPath(path string) *FileWriterBuilder {
	f.fileWriter.path = path
	return f
}
func (f *FileWriterBuilder) WithDocumentMarkers() *FileWriterBuilder {
	f.fileWriter.withDocumentMarkers = true
	return f
}
func (f *FileWriterBuilder) WithAppendMode() *FileWriterBuilder {
	f.fileWriter.appendMode = true
	return f
}
func (f *FileWriterBuilder) WithMetadataMode(mode document.MetadataMode) *FileWriterBuilder {
	f.fileWriter.metadataMode = mode
	return f
}

func (f *FileWriterBuilder) Build() (*FileWriter, error) {
	if f.fileWriter.path == "" {
		return nil, errors.New("file path is empty")
	}
	return f.fileWriter, nil
}
