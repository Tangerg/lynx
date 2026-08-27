// Package text reads plain text into documents.
package text

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/samber/lo"

	"github.com/Tangerg/lynx/core/document"
)

// Reader reads the entire contents of an [io.Reader] and packages
// it into one [*document.Document]. Use it for files, in-memory buffers, or
// network streams that fit comfortably in memory.
type Reader struct {
	source io.Reader
}

// New constructs a text Reader from source.
func New(source io.Reader) (*Reader, error) {
	if lo.IsNil(source) {
		return nil, errors.New("text reader: source must not be nil")
	}
	return &Reader{source: source}, nil
}

// Read consumes the source and returns one document containing its text. Blank
// input returns no documents.
func (r *Reader) Read(ctx context.Context) ([]*document.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(r.source)
	if err != nil {
		return nil, fmt.Errorf("text reader: read source: %w", err)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	text := string(data)
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}

	doc, err := document.NewDocument(text, nil)
	if err != nil {
		return nil, fmt.Errorf("text reader: build document: %w", err)
	}
	return []*document.Document{doc}, nil
}
