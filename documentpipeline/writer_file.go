package documentpipeline

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/core/document"
)

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
		return nil, errors.New("document pipeline: output path is required")
	}
	if config.Formatter == nil {
		config.Formatter = FormatterFunc(formatText)
	} else if isNil(config.Formatter) {
		return nil, errors.New("document pipeline: formatter must not be a typed nil")
	}
	mode, err := normalizeMetadataMode(config.Mode)
	if err != nil {
		return nil, err
	}
	return &FileWriter{
		path:            config.Path,
		documentMarkers: config.DocumentMarkers,
		append:          config.Append,
		formatter:       config.Formatter,
		mode:            mode,
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
		return fmt.Errorf("document pipeline: open output %q: %w", f.path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("document pipeline: close output %q: %w", f.path, closeErr))
		}
	}()

	if writeErr := f.write(ctx, docs, file); writeErr != nil {
		return fmt.Errorf("document pipeline: write output %q: %w", f.path, writeErr)
	}
	return nil
}

func (f *FileWriter) openFlags() int {
	if f.append {
		return os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	return os.O_CREATE | os.O_WRONLY | os.O_TRUNC
}

func (f *FileWriter) write(ctx context.Context, docs []*document.Document, file *os.File) error {
	buffered := bufio.NewWriter(file)
	for i, doc := range docs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if doc == nil {
			return fmt.Errorf("document %d: %w", i, ErrNilDocument)
		}
		rendered, err := f.renderDocument(i, doc)
		if err != nil {
			return fmt.Errorf("render document %d: %w", i, err)
		}
		if _, err := io.WriteString(buffered, rendered); err != nil {
			return fmt.Errorf("write document %d: %w", i, err)
		}
	}
	if err := buffered.Flush(); err != nil {
		return fmt.Errorf("flush buffered output: %w", err)
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

		start, end, hasRange, err := documentPageRange(doc)
		if err != nil {
			return "", err
		}
		if hasRange {
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

func documentPageRange(doc *document.Document) (string, string, bool, error) {
	if doc == nil || doc.Metadata == nil {
		return "", "", false, nil
	}
	startValue, startFound := doc.Metadata[metadataKeyStartPageNumber]
	endValue, endFound := doc.Metadata[metadataKeyEndPageNumber]
	if !startFound || !endFound {
		return "", "", false, nil
	}
	start, err := metadataValueText(startValue)
	if err != nil {
		return "", "", false, fmt.Errorf("format start page number: %w", err)
	}
	end, err := metadataValueText(endValue)
	if err != nil {
		return "", "", false, fmt.Errorf("format end page number: %w", err)
	}
	if start == "" || end == "" {
		return "", "", false, nil
	}
	return start, end, true, nil
}
