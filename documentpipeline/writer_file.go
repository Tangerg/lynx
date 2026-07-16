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

// FileWriterConfig configures plain-text output for [FileWriter].
type FileWriterConfig struct {
	// Path is required. Existing files are replaced unless Append is true.
	Path string
	// DocumentMarkers adds an index header before each document.
	DocumentMarkers bool
	// Append preserves existing file contents.
	Append bool
	// Formatter renders each document. Nil writes document text only.
	Formatter Formatter
	// Mode is passed to Formatter. The zero value is MetadataModeAll.
	Mode MetadataMode
}

// FileWriter persists documents as plain text. It honors Append,
// optionally injects document-marker headers, and calls [*os.File].Sync
// before returning so callers can rely on durability when the call
// completes.
//
// Example:
//
//	w, err := documentpipeline.NewFileWriter(documentpipeline.FileWriterConfig{
//	    Path:            "out.txt",
//	    DocumentMarkers: true,
//	})
//	err = w.Write(ctx, docs)
type FileWriter struct {
	path            string
	documentMarkers bool
	append          bool
	formatter       Formatter
	mode            MetadataMode
}

func NewFileWriter(config FileWriterConfig) (*FileWriter, error) {
	if config.Path == "" {
		return nil, errors.New("documentpipeline.FileWriterConfig: Path is required")
	}
	if config.Formatter == nil {
		config.Formatter = FormatterFunc(formatText)
	}
	if config.Mode == "" {
		config.Mode = MetadataModeAll
	}
	if !validMetadataMode(config.Mode) {
		return nil, fmt.Errorf("documentpipeline.FileWriterConfig: invalid Mode %q", config.Mode)
	}
	return &FileWriter{
		path:            config.Path,
		documentMarkers: config.DocumentMarkers,
		append:          config.Append,
		formatter:       config.Formatter,
		mode:            config.Mode,
	}, nil
}

// Write persists docs to the configured file. Close errors after a
// successful write are surfaced (joined with any earlier error) so
// callers can detect partial flushes that fail at close time.
func (f *FileWriter) Write(ctx context.Context, docs []*document.Document) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := os.OpenFile(f.path, f.openFlags(), 0o666)
	if err != nil {
		return fmt.Errorf("documentpipeline.FileWriter.Write: open %s: %w", f.path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("documentpipeline.FileWriter.Write: close: %w", closeErr))
		}
	}()

	if writeErr := f.writeBatched(ctx, docs, file); writeErr != nil {
		return fmt.Errorf("documentpipeline.FileWriter.Write: %w", writeErr)
	}
	return nil
}

func (f *FileWriter) openFlags() int {
	if f.append {
		return os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	return os.O_CREATE | os.O_WRONLY | os.O_TRUNC
}

func (f *FileWriter) writeBatched(ctx context.Context, docs []*document.Document, file *os.File) error {
	var buf strings.Builder

	for i, doc := range docs {
		if err := ctx.Err(); err != nil {
			return err
		}
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
	if err := ctx.Err(); err != nil {
		return err
	}
	return file.Sync()
}

func (f *FileWriter) renderDocument(index int, doc *document.Document) (string, error) {
	var buf strings.Builder

	if f.documentMarkers {
		buf.WriteString("### Index: ")
		buf.WriteString(strconv.Itoa(index))

		if start, end, ok := documentPageRange(doc); ok {
			buf.WriteString(", Pages:[")
			buf.WriteString(start)
			buf.WriteString(",")
			buf.WriteString(end)
			buf.WriteString("]")
		}
		buf.WriteString("\n")
	}

	rendered, err := f.formatter.Format(doc, f.mode)
	if err != nil {
		return "", err
	}
	buf.WriteString(rendered)
	buf.WriteString("\n\n")
	return buf.String(), nil
}

func documentPageRange(doc *document.Document) (string, string, bool) {
	if doc == nil || doc.Metadata == nil {
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
