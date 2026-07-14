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

func (m MetadataMode) String() string { return string(m) }

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

// Batcher preserves document order while partitioning a request.
type Batcher interface {
	Batch(context.Context, []*document.Document) ([][]*document.Document, error)
}

// BoundFormatter fixes a metadata mode and exposes a consumer-friendly
// one-argument Format method.
type BoundFormatter struct {
	Formatter Formatter
	Mode      MetadataMode
}

func (f BoundFormatter) Format(doc *document.Document) (string, error) {
	if f.Formatter == nil {
		return NewNop().Format(doc, f.Mode)
	}
	return f.Formatter.Format(doc, f.Mode)
}
