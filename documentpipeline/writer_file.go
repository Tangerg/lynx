package documentpipeline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/metadata"
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

type FileWriterConfig struct {
	Path                string
	WithDocumentMarkers bool
	AppendMode          bool
	Formatter           Formatter
	MetadataMode        MetadataMode
}

func (c FileWriterConfig) Validate() error {
	if c.Path == "" {
		return errors.New("documentpipeline.FileWriterConfig: Path is required")
	}
	return nil
}

var _ document.Writer = (*FileWriter)(nil)

// FileWriter persists documents as plain text. It honors AppendMode,
// optionally injects document-marker headers, and calls [*os.File].Sync
// before returning so callers can rely on durability when the call
// completes.
//
// Example:
//
//	w, err := documentpipeline.NewFileWriter(documentpipeline.FileWriterConfig{
//	    Path:                "out.txt",
//	    WithDocumentMarkers: true,
//	})
//	err = w.Write(ctx, docs)
type FileWriter struct {
	path                string
	withDocumentMarkers bool
	appendMode          bool
	formatter           Formatter
	metadataMode        MetadataMode
}

func NewFileWriter(config FileWriterConfig) (*FileWriter, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.Formatter == nil {
		config.Formatter = NewNop()
	}
	if config.MetadataMode == "" {
		config.MetadataMode = MetadataModeAll
	}
	return &FileWriter{
		path:                config.Path,
		withDocumentMarkers: config.WithDocumentMarkers,
		appendMode:          config.AppendMode,
		formatter:           config.Formatter,
		metadataMode:        config.MetadataMode,
	}, nil
}

// Write persists docs to the configured file. Close errors after a
// successful write are surfaced (joined with any earlier error) so
// callers can detect partial flushes that fail at close time.
func (f *FileWriter) Write(_ context.Context, docs []*document.Document) (err error) {
	file, err := os.OpenFile(f.path, f.openFlags(), 0o666)
	if err != nil {
		return fmt.Errorf("documentpipeline.FileWriter.Write: open %s: %w", f.path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("documentpipeline.FileWriter.Write: close: %w", closeErr))
		}
	}()

	if writeErr := f.writeBatched(docs, file); writeErr != nil {
		return fmt.Errorf("documentpipeline.FileWriter.Write: %w", writeErr)
	}
	return nil
}

func (f *FileWriter) openFlags() int {
	if f.appendMode {
		return os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	return os.O_CREATE | os.O_WRONLY | os.O_TRUNC
}

func (f *FileWriter) writeBatched(docs []*document.Document, file *os.File) error {
	var buf strings.Builder

	for i, doc := range docs {
		rendered, err := f.renderDocument(i, doc)
		if err != nil {
			return fmt.Errorf("render document %d: %w", i, err)
		}
		buf.WriteString(rendered)

		if (i+1)%fileWriterBatchSize == 0 {
			if _, err := file.WriteString(buf.String()); err != nil {
				return fmt.Errorf("documentpipeline.FileWriter.writeBatched: flush batch at index %d: %w", i, err)
			}
			buf.Reset()
		}
	}

	if buf.Len() > 0 {
		if _, err := file.WriteString(buf.String()); err != nil {
			return fmt.Errorf("documentpipeline.FileWriter.writeBatched: flush trailing batch: %w", err)
		}
	}
	return file.Sync()
}

func (f *FileWriter) renderDocument(index int, doc *document.Document) (string, error) {
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

	rendered, err := f.formatter.Format(doc, f.metadataMode)
	if err != nil {
		return "", err
	}
	buf.WriteString(rendered)
	buf.WriteString("\n\n")
	return buf.String(), nil
}

func (f *FileWriter) pageRange(doc *document.Document) (string, string, bool) {
	if doc.Metadata == nil {
		return "", "", false
	}
	startValue, _, _ := metadata.Decode[any](doc.Metadata, metadataKeyStartPageNumber)
	endValue, _, _ := metadata.Decode[any](doc.Metadata, metadataKeyEndPageNumber)
	start := cast.ToString(startValue)
	end := cast.ToString(endValue)
	if start == "" || end == "" {
		return "", "", false
	}
	return start, end, true
}
