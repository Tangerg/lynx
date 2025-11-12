package document

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cast"
)

// FileWriterConfig holds the configuration for FileWriter.
type FileWriterConfig struct {
	// Path specifies the file path where documents will be written.
	// Required. Must not be empty.
	// Can be an absolute or relative path. Parent directories will be created
	// if they don't exist (depending on file system permissions).
	Path string

	// WithDocumentMarkers determines whether to include document markers in the output.
	// Optional. Defaults to false.
	// When enabled, each document is prefixed with metadata including:
	//   - Document index number
	//   - Page range (if start_page_number and end_page_number metadata exist)
	// Format: "### Index: 0, Pages:[1,5]"
	WithDocumentMarkers bool

	// AppendMode determines whether to append to existing file or overwrite it.
	// Optional. Defaults to false (overwrite mode).
	// If true, documents are appended to the end of the file.
	// If false, the file is truncated before writing.
	AppendMode bool
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

var _ Writer = (*FileWriter)(nil)

// FileWriter writes documents to a file with optional formatting and markers.
//
// This writer is useful for:
//   - Exporting processed documents to disk for inspection or backup
//   - Creating human-readable document archives with optional metadata
//   - Building document export pipelines with append or overwrite modes
//   - Debugging document processing flows by examining intermediate outputs
//
// The writer uses batched writes (5 documents per batch) for improved I/O performance
// and calls file.Sync() to ensure data is persisted to disk. Documents are separated
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

func (f *FileWriter) Write(_ context.Context, documents []*Document) error {
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

func (f *FileWriter) writeDocumentBatch(documents []*Document, outputFile *os.File) error {
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

	return outputFile.Sync()
}

func (f *FileWriter) buildDocumentContent(docIndex int, doc *Document) string {
	const (
		startPageNumber = "start_page_number"
		endPageNumber   = "end_page_number"
	)

	var contentBuilder strings.Builder

	if f.withDocumentMarkers {
		contentBuilder.WriteString("### Index: ")
		contentBuilder.WriteString(strconv.Itoa(docIndex))

		docMetadata := doc.Metadata
		if docMetadata != nil {
			startPage := cast.ToString(docMetadata[startPageNumber])
			endPage := cast.ToString(docMetadata[endPageNumber])
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
