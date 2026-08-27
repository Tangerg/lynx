package etl

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/samber/lo"

	"github.com/Tangerg/scope/core/document"
)

// TextFileWriterConfig configures plain-text output for [TextFileWriter].
type TextFileWriterConfig struct {
	// Path is required. Existing files are replaced unless Append is true.
	Path string
	// DocumentMarkers adds an index header before each document.
	DocumentMarkers bool
	// Append preserves existing file contents.
	Append bool
	// Formatter renders each document. Nil writes document text only.
	Formatter Formatter
}

// TextFileWriter persists documents as plain text. It honors Append,
// optionally injects document-marker headers, and calls [*os.File].Sync
// before returning so callers can rely on durability when the call
// completes.
//
// Example:
//
//	w, err := etl.NewTextFileWriter(etl.TextFileWriterConfig{
//	    Path:            "out.txt",
//	    DocumentMarkers: true,
//	})
//	err = w.Write(ctx, docs)
type TextFileWriter struct {
	path            string
	documentMarkers bool
	append          bool
	formatter       Formatter
}

// NewTextFileWriter constructs a plain-text file load target with a fixed
// formatting and append policy.
func NewTextFileWriter(config TextFileWriterConfig) (*TextFileWriter, error) {
	if config.Path == "" {
		return nil, errors.New("etl: output path is required")
	}
	if config.Formatter == nil {
		config.Formatter = TextFormatter{}
	} else if lo.IsNil(config.Formatter) {
		return nil, errors.New("etl: formatter must not be a typed nil")
	}
	return &TextFileWriter{
		path:            config.Path,
		documentMarkers: config.DocumentMarkers,
		append:          config.Append,
		formatter:       config.Formatter,
	}, nil
}

// Write persists docs to the configured file. Close errors after a
// successful write are surfaced (joined with any earlier error) so
// callers can detect partial flushes that fail at close time.
func (t *TextFileWriter) Write(ctx context.Context, docs []*document.Document) (err error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	file, err := os.OpenFile(t.path, t.openFlags(), 0o666)
	if err != nil {
		return fmt.Errorf("etl: open output %q: %w", t.path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("etl: close output %q: %w", t.path, closeErr))
		}
	}()

	if writeErr := t.write(ctx, docs, file); writeErr != nil {
		return fmt.Errorf("etl: write output %q: %w", t.path, writeErr)
	}
	return nil
}

func (t *TextFileWriter) openFlags() int {
	if t.append {
		return os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	return os.O_CREATE | os.O_WRONLY | os.O_TRUNC
}

func (t *TextFileWriter) write(ctx context.Context, docs []*document.Document, file *os.File) error {
	buffered := bufio.NewWriter(file)
	for i, doc := range docs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if doc == nil {
			return fmt.Errorf("document %d: %w", i, ErrNilDocument)
		}
		if err := doc.Validate(); err != nil {
			return fmt.Errorf("validate document %d: %w", i, err)
		}
		rendered, err := t.renderDocument(i, doc)
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

func (t *TextFileWriter) renderDocument(index int, doc *document.Document) (string, error) {
	var buf strings.Builder

	if t.documentMarkers {
		buf.WriteString("### Index: ")
		buf.WriteString(strconv.Itoa(index))
		buf.WriteString("\n")
	}

	rendered, err := t.formatter.Format(doc)
	if err != nil {
		return "", err
	}
	buf.WriteString(rendered)
	buf.WriteString("\n\n")
	return buf.String(), nil
}
