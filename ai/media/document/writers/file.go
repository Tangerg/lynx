package writers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/ai/media/document"
)

const (
	MetadataStartPageNumber = "start_page_number"
	MetadataEndPageNumber   = "end_page_number"
)

type FileWriterConfig struct {
	Path                string
	WithDocumentMarkers bool
	AppendMode          bool
}

func (c *FileWriterConfig) validate() error {
	if c == nil {
		return errors.New("config is required")
	}
	if c.Path == "" {
		return errors.New("file path is required")
	}
	return nil
}

var _ document.Writer = (*FileWriter)(nil)

type FileWriter struct {
	path                string
	withDocumentMarkers bool
	appendMode          bool
}

func NewFileWriter(config *FileWriterConfig) (*FileWriter, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	return &FileWriter{
		path:                config.Path,
		withDocumentMarkers: config.WithDocumentMarkers,
		appendMode:          config.AppendMode,
	}, nil
}

func (f *FileWriter) Write(ctx context.Context, documents []*document.Document) error {
	fileFlags := f.determineFileFlags()
	outputFile, err := os.OpenFile(f.path, fileFlags, 0666)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", f.path, err)
	}
	defer outputFile.Close()

	if err = f.writeDocumentBatch(documents, outputFile); err != nil {
		return fmt.Errorf("failed to write documents to file %s: %w", f.path, err)
	}

	return nil
}

func (f *FileWriter) determineFileFlags() int {
	const (
		createWriteTrunc  = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		createWriteAppend = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	)

	if f.appendMode {
		return createWriteAppend
	}
	return createWriteTrunc
}

func (f *FileWriter) writeDocumentBatch(documents []*document.Document, outputFile *os.File) error {
	const writeBatchSize = 5
	var batchBuffer strings.Builder

	for docIndex, currentDoc := range documents {
		formattedContent := f.buildDocumentContent(docIndex, currentDoc)
		batchBuffer.WriteString(formattedContent)

		shouldFlushBatch := (docIndex+1)%writeBatchSize == 0
		if shouldFlushBatch {
			if _, err := outputFile.WriteString(batchBuffer.String()); err != nil {
				return fmt.Errorf("failed to write document batch at index %d: %w", docIndex, err)
			}
			batchBuffer.Reset()
		}
	}

	if batchBuffer.Len() > 0 {
		if _, err := outputFile.WriteString(batchBuffer.String()); err != nil {
			return fmt.Errorf("failed to write final document batch: %w", err)
		}
	}

	return nil
}

func (f *FileWriter) buildDocumentContent(docIndex int, doc *document.Document) string {
	var contentBuilder strings.Builder

	if f.withDocumentMarkers {
		contentBuilder.WriteString("### Index: ")
		contentBuilder.WriteString(strconv.Itoa(docIndex))

		docMetadata := doc.Metadata
		if docMetadata != nil {
			startPage := cast.ToString(docMetadata[MetadataStartPageNumber])
			endPage := cast.ToString(docMetadata[MetadataEndPageNumber])
			if startPage != "" && endPage != "" {
				contentBuilder.WriteString(", Pages:[")
				contentBuilder.WriteString(startPage)
				contentBuilder.WriteString(",")
				contentBuilder.WriteString(endPage)
				contentBuilder.WriteString("]")
			}
		}

		contentBuilder.WriteString("\n")
	}

	contentBuilder.WriteString(doc.Format())

	contentBuilder.WriteString("\n\n")

	return contentBuilder.String()
}
