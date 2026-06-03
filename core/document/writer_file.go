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

// fileWriterBatchSize is the number of documents buffered between
// [os.File].WriteString flushes — small enough to bound peak memory.
const fileWriterBatchSize = 5

// Metadata keys recognized by [FileWriter] when writing document
// markers. These are conventions, not part of [Document]'s public
// fields.
const (
	metadataKeyStartPageNumber = "start_page_number"
	metadataKeyEndPageNumber   = "end_page_number"
)

// FileWriterConfig configures a [FileWriter].
type FileWriterConfig struct {
	// Path is the destination file. Required.
	Path string

	// WithDocumentMarkers prepends each document with a header line:
	//
	//	### Index: 0, Pages:[1,5]
	//
	// The Pages segment appears only when both start_page_number and
	// end_page_number live in the document's metadata.
	WithDocumentMarkers bool

	// AppendMode appends to an existing file instead of truncating it.
	AppendMode bool
}

func (c FileWriterConfig) Validate() error {
	if c.Path == "" {
		return errors.New("document.FileWriterConfig: Path is required")
	}
	return nil
}

var _ Writer = (*FileWriter)(nil)

// FileWriter persists documents as plain text. It honors AppendMode,
// optionally injects document-marker headers, and calls [*os.File].Sync
// before returning so callers can rely on durability when the call
// completes.
//
// Example:
//
//	w, err := document.NewFileWriter(document.FileWriterConfig{
//	    Path:                "out.txt",
//	    WithDocumentMarkers: true,
//	})
//	err = w.Write(ctx, docs)
type FileWriter struct {
	path                string
	withDocumentMarkers bool
	appendMode          bool
}

// NewFileWriter builds a [FileWriter]. Returns an error when config is
// nil or invalid.
func NewFileWriter(config FileWriterConfig) (*FileWriter, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &FileWriter{
		path:                config.Path,
		withDocumentMarkers: config.WithDocumentMarkers,
		appendMode:          config.AppendMode,
	}, nil
}

// Write persists docs to the configured file. Close errors after a
// successful write are surfaced (joined with any earlier error) so
// callers can detect partial flushes that fail at close time.
func (f *FileWriter) Write(_ context.Context, docs []*Document) (err error) {
	file, err := os.OpenFile(f.path, f.openFlags(), 0o666)
	if err != nil {
		return fmt.Errorf("document.FileWriter.Write: open %s: %w", f.path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("document.FileWriter.Write: close: %w", closeErr))
		}
	}()

	if writeErr := f.writeBatched(docs, file); writeErr != nil {
		return fmt.Errorf("document.FileWriter.Write: %w", writeErr)
	}
	return nil
}

// openFlags returns the appropriate os.OpenFile flags for the
// configured mode (truncate vs append).
func (f *FileWriter) openFlags() int {
	if f.appendMode {
		return os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	return os.O_CREATE | os.O_WRONLY | os.O_TRUNC
}

// writeBatched buffers fileWriterBatchSize documents at a time, then
// flushes to disk and Syncs at the end so the caller can rely on
// durability.
func (f *FileWriter) writeBatched(docs []*Document, file *os.File) error {
	var buf strings.Builder

	for i, doc := range docs {
		buf.WriteString(f.renderDocument(i, doc))

		if (i+1)%fileWriterBatchSize == 0 {
			if _, err := file.WriteString(buf.String()); err != nil {
				return fmt.Errorf("flush batch at index %d: %w", i, err)
			}
			buf.Reset()
		}
	}

	if buf.Len() > 0 {
		if _, err := file.WriteString(buf.String()); err != nil {
			return fmt.Errorf("flush trailing batch: %w", err)
		}
	}
	return file.Sync()
}

// renderDocument formats one document, optionally prefixing the marker
// header.
func (f *FileWriter) renderDocument(index int, doc *Document) string {
	var buf strings.Builder

	if f.withDocumentMarkers {
		buf.WriteString("### Index: ")
		buf.WriteString(strconv.Itoa(index))

		if start, end, ok := f.pageRange(doc); ok {
			buf.WriteString(", Pages:[")
			buf.WriteString(start)
			buf.WriteString(",")
			buf.WriteString(end)
			buf.WriteString("]")
		}
		buf.WriteString("\n")
	}

	buf.WriteString(doc.Format())
	buf.WriteString("\n\n")
	return buf.String()
}

// pageRange returns the start/end page from doc.Metadata when both
// fields are present and non-empty.
func (f *FileWriter) pageRange(doc *Document) (string, string, bool) {
	if doc.Metadata == nil {
		return "", "", false
	}
	start := cast.ToString(doc.Metadata[metadataKeyStartPageNumber])
	end := cast.ToString(doc.Metadata[metadataKeyEndPageNumber])
	if start == "" || end == "" {
		return "", "", false
	}
	return start, end, true
}
