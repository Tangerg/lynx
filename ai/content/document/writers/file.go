package writers

import (
	"context"
	"fmt"
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
	Path                string
	WithDocumentMarkers bool
	AppendMode          bool
}

func (fw *FileWriter) determineFileFlags() int {
	const (
		createWriteTrunc  = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		createWriteAppend = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	)

	if fw.AppendMode {
		return createWriteAppend
	}
	return createWriteTrunc
}

func (fw *FileWriter) buildDocumentContent(docIndex int, doc *document.Document) string {
	var contentBuilder strings.Builder

	if fw.WithDocumentMarkers {
		contentBuilder.WriteString("### Index: ")
		contentBuilder.WriteString(strconv.Itoa(docIndex))
		contentBuilder.WriteString(", ID: ")
		contentBuilder.WriteString(doc.ID())
		contentBuilder.WriteString(", Pages:[")

		docMetadata := doc.Metadata()
		startPage := cast.ToString(docMetadata[MetadataStartPageNumber])
		endPage := cast.ToString(docMetadata[MetadataEndPageNumber])

		contentBuilder.WriteString(startPage)
		contentBuilder.WriteString(",")
		contentBuilder.WriteString(endPage)
		contentBuilder.WriteString("]")
		contentBuilder.WriteString("\n")
	}

	if doc.Formatter() != nil {
		contentBuilder.WriteString(doc.Format())
	} else {
		contentBuilder.WriteString(doc.Text())
	}

	contentBuilder.WriteString("\n\n")
	return contentBuilder.String()
}

func (fw *FileWriter) writeDocumentBatch(documents []*document.Document, outputFile *os.File) error {
	const writeBatchSize = 5
	var batchBuffer strings.Builder

	for docIndex, currentDoc := range documents {
		formattedContent := fw.buildDocumentContent(docIndex, currentDoc)
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
		if _, writeErr := outputFile.WriteString(batchBuffer.String()); writeErr != nil {
			return fmt.Errorf("failed to write final document batch: %w", writeErr)
		}
	}

	return nil
}

func (fw *FileWriter) Write(_ context.Context, documents []*document.Document) error {
	fileFlags := fw.determineFileFlags()
	outputFile, openErr := os.OpenFile(fw.Path, fileFlags, 0666)
	if openErr != nil {
		return fmt.Errorf("failed to open file %s: %w", fw.Path, openErr)
	}
	defer outputFile.Close()

	err := fw.writeDocumentBatch(documents, outputFile)
	if err != nil {
		return fmt.Errorf("failed to write documents to file %s: %w", fw.Path, err)
	}

	return nil
}
