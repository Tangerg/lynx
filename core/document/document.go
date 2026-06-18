package document

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/core/document/id"
	"github.com/Tangerg/lynx/core/media"
)

// Document is the canonical content carrier. A document holds a unique
// id, optional textual content, optional media, optional retrieval
// score, a metadata map, and the formatter used to render it for
// downstream consumers.
type Document struct {
	ID string `json:"id,omitempty"`

	// Score is the retrieval relevance score. 0 when the document was
	// not produced by a search.
	Score float64 `json:"score,omitempty"`

	// Text is the textual content. May be empty if Media is set.
	Text string `json:"text,omitempty"`

	Media *media.Media `json:"media,omitempty"`

	// Formatter renders the document for downstream consumers (LLM
	// prompt, log line, ...). Defaults to a no-op that emits Text.
	// Excluded from JSON: the value holds runtime behavior that cannot
	// meaningfully round-trip — consumers rehydrate Documents via
	// [NewDocument] or set Formatter explicitly after Unmarshal.
	Formatter Formatter `json:"-"`

	Metadata map[string]any `json:"metadata,omitzero"`
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

// EnsureID assigns an id when the document has none, deriving it from
// the document's content (text, media, metadata) via the supplied
// [id.Generator]. A document that already carries an id is left
// untouched, so EnsureID is safe to call repeatedly across a pipeline.
//
// Pass an [id.Sha256Generator] for content-addressable, dedup-friendly
// ids, or an [id.UUIDGenerator] for unconditional uniqueness. Returns
// an error only when the generator does (e.g. an input that fails to
// JSON-marshal).
func (d *Document) EnsureID(ctx context.Context, generator id.Generator) error {
	if d.ID != "" || generator == nil {
		return nil
	}
	generated, err := generator.Generate(ctx, d.Text, d.Media, d.Metadata)
	if err != nil {
		return err
	}
	d.ID = generated
	return nil
}

func (d *Document) Format() string {
	if d == nil {
		return ""
	}
	return d.FormatByMetadataMode(MetadataModeAll)
}

func (d *Document) FormatByMetadataMode(mode MetadataMode) string {
	if d == nil {
		return ""
	}
	return d.FormatWith(mode, d.Formatter)
}

// FormatWith renders the document with the supplied metadata mode and
// formatter, falling back to a no-op when formatter is nil.
func (d *Document) FormatWith(mode MetadataMode, formatter Formatter) string {
	if d == nil {
		return ""
	}
	if formatter == nil {
		formatter = NewNop()
	}
	return formatter.Format(d, mode)
}
