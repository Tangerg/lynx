package document

import (
	"errors"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
)

// Document is the canonical content carrier. It holds identity, content, and
// metadata; query-specific relationships and runtime policies live outside
// this value.
type Document struct {
	ID string `json:"id,omitempty"`

	// Text is the textual content. May be empty if Media is set.
	Text string `json:"text,omitempty"`

	Media *media.Media `json:"media,omitempty"`

	Metadata metadata.Map `json:"metadata,omitzero"`
}

// NewDocument builds a [Document]. At least one of text or media is
// required.
//
// Example:
//
//	doc, err := document.NewDocument("hello", nil)
//	_ = doc.Metadata.Set("source", "manual")
func NewDocument(text string, media *media.Media) (*Document, error) {
	if text == "" && media == nil {
		return nil, errors.New("document.NewDocument: at least one of text or media is required")
	}

	doc := &Document{
		Text:     text,
		Media:    media,
		Metadata: metadata.New(),
	}
	return doc, doc.Validate()
}

// Validate reports whether the document contains usable content.
func (d *Document) Validate() error {
	if d == nil {
		return errors.New("document.Document: nil")
	}
	if d.Text == "" && d.Media == nil {
		return errors.New("document.Document: at least one of Text or Media is required")
	}
	if d.Media != nil {
		if err := d.Media.Validate(); err != nil {
			return errors.Join(errors.New("document.Document: invalid Media"), err)
		}
	}
	if err := d.Metadata.Validate(); err != nil {
		return errors.Join(errors.New("document.Document: invalid Metadata"), err)
	}
	return nil
}
