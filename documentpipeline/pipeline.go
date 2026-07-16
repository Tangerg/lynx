package documentpipeline

import (
	"context"

	"github.com/Tangerg/lynx/core/document"
)

// MetadataMode selects which metadata a Formatter includes.
type MetadataMode string

const (
	MetadataModeAll       MetadataMode = "all"
	MetadataModeEmbed     MetadataMode = "embed"
	MetadataModeInference MetadataMode = "inference"
	MetadataModeNone      MetadataMode = "none"
)

func validMetadataMode(mode MetadataMode) bool {
	switch mode {
	case MetadataModeAll, MetadataModeEmbed, MetadataModeInference, MetadataModeNone:
		return true
	default:
		return false
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
	if f.Formatter == nil {
		return formatText(doc, f.Mode)
	}
	return f.Formatter.Format(doc, f.Mode)
}

func formatText(doc *document.Document, _ MetadataMode) (string, error) {
	if doc == nil {
		return "", nil
	}
	return doc.Text, nil
}
