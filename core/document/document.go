// Package document defines the [Document] container — the unit of
// content that flows through readers, transformers, embedders, and
// vector stores — together with the interfaces ([Reader], [Writer],
// [Transformer], [Batcher], [Formatter]) that operate on it.
package document

import (
	"errors"

	"github.com/Tangerg/lynx/core/media"
)

// Document is the canonical content carrier. A document holds a unique
// id, optional textual content, optional media, optional retrieval
// score, a metadata map, and the formatter used to render it for
// downstream consumers.
type Document struct {
	// ID identifies the document within its source. Often a hash of the
	// content; the [id] subpackage offers ready-made generators.
	ID string

	// Score is the retrieval relevance score. 0 when the document was
	// not produced by a search.
	Score float64

	// Text is the textual content. May be empty if Media is set.
	Text string

	// Media is the optional non-text payload (image, audio, ...).
	Media *media.Media

	// Formatter renders the document for downstream consumers (LLM
	// prompt, log line, ...). Defaults to a no-op that emits Text.
	Formatter Formatter

	// Metadata carries free-form annotations (source URL, page index,
	// section heading, ...).
	Metadata map[string]any
}

// NewDocument builds a [Document]. At least one of text or media is
// required.
//
// Example:
//
//	doc, err := document.NewDocument("hello", nil)
//	doc.Metadata["source"] = "manual"
func NewDocument(text string, media *media.Media) (*Document, error) {
	if text == "" && media == nil {
		return nil, errors.New("document.NewDocument: at least one of text or media is required")
	}

	return &Document{
		Text:      text,
		Media:     media,
		Metadata:  make(map[string]any),
		Formatter: NewNop(),
	}, nil
}

// Format renders the document with all metadata included.
func (d *Document) Format() string {
	return d.FormatByMetadataMode(MetadataModeAll)
}

// FormatByMetadataMode renders the document with the supplied metadata
// mode using the document's installed [Formatter].
func (d *Document) FormatByMetadataMode(mode MetadataMode) string {
	return d.FormatByMetadataModeWithFormatter(mode, d.Formatter)
}

// FormatByMetadataModeWithFormatter renders the document with the
// supplied formatter, falling back to a no-op when formatter is nil.
func (d *Document) FormatByMetadataModeWithFormatter(mode MetadataMode, formatter Formatter) string {
	if formatter == nil {
		formatter = NewNop()
	}
	return formatter.Format(d, mode)
}
