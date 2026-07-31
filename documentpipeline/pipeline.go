package documentpipeline

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/Tangerg/lynx/core/document"
)

var (
	// ErrNilDocument reports a nil document at a pipeline stage boundary.
	ErrNilDocument = errors.New("document pipeline: document must not be nil")
	// ErrInvalidMetadataMode reports an unknown formatter metadata policy.
	ErrInvalidMetadataMode = errors.New("document pipeline: invalid metadata mode")
)

// MetadataMode selects which metadata a Formatter includes.
type MetadataMode string

const (
	MetadataModeAll       MetadataMode = "all"
	MetadataModeEmbed     MetadataMode = "embed"
	MetadataModeInference MetadataMode = "inference"
	MetadataModeNone      MetadataMode = "none"
)

func normalizeMetadataMode(mode MetadataMode) (MetadataMode, error) {
	if mode == "" {
		return MetadataModeAll, nil
	}
	switch mode {
	case MetadataModeAll, MetadataModeEmbed, MetadataModeInference, MetadataModeNone:
		return mode, nil
	default:
		return "", fmt.Errorf("%w %q", ErrInvalidMetadataMode, mode)
	}
}

// Formatter renders a document without attaching behavior to the document.
type Formatter interface {
	Format(*document.Document, MetadataMode) (string, error)
}

// FormatterFunc adapts a function to Formatter.
type FormatterFunc func(*document.Document, MetadataMode) (string, error)

func (f FormatterFunc) Format(doc *document.Document, mode MetadataMode) (string, error) {
	return f(doc, mode)
}

// Transformer is one explicit document-processing stage.
type Transformer interface {
	Transform(context.Context, []*document.Document) ([]*document.Document, error)
}

// TransformerFunc adapts a function to Transformer.
type TransformerFunc func(context.Context, []*document.Document) ([]*document.Document, error)

func (f TransformerFunc) Transform(ctx context.Context, docs []*document.Document) ([]*document.Document, error) {
	return f(ctx, docs)
}

// Batcher preserves document order while partitioning a request.
type Batcher interface {
	Batch(context.Context, []*document.Document) ([][]*document.Document, error)
}

// BatcherFunc adapts a function to Batcher.
type BatcherFunc func(context.Context, []*document.Document) ([][]*document.Document, error)

func (f BatcherFunc) Batch(ctx context.Context, docs []*document.Document) ([][]*document.Document, error) {
	return f(ctx, docs)
}

// BoundFormatter fixes a metadata mode and exposes a consumer-friendly
// one-argument Format method.
type BoundFormatter struct {
	Formatter Formatter
	Mode      MetadataMode
}

func (f BoundFormatter) Format(doc *document.Document) (string, error) {
	mode, err := normalizeMetadataMode(f.Mode)
	if err != nil {
		return "", err
	}
	if f.Formatter == nil {
		return formatText(doc, mode)
	}
	return f.Formatter.Format(doc, mode)
}

func formatText(doc *document.Document, _ MetadataMode) (string, error) {
	if doc == nil {
		return "", ErrNilDocument
	}
	return doc.Text, nil
}

func isNil(value any) bool {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return true
	}
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
