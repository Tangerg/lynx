package document

import (
	"errors"
	"fmt"

	"github.com/Tangerg/scope/core/media"
	"github.com/Tangerg/scope/core/metadata"
)

// ErrInvalidDocument classifies malformed document values at construction,
// validation, and wire boundaries.
var ErrInvalidDocument = errors.New("document: invalid document")

// Document is the canonical content carrier. It holds identity, content, and
// metadata; query-specific relationships and runtime policies live outside
// this value. Clone recursively snapshots media and metadata so indexing and
// retrieval boundaries never retain caller-owned mutable buffers.
type Document struct {
	ID string `json:"id,omitempty"`

	// Text is the textual content. May be empty if Media is set.
	Text string `json:"text,omitempty"`

	Media *media.Media `json:"media,omitempty"`

	Metadata metadata.Map `json:"metadata,omitzero"`
}

func (d *Document) Clone() *Document {
	if d == nil {
		return nil
	}
	clone := *d
	clone.Media = d.Media.Clone()
	clone.Metadata = d.Metadata.Clone()
	return &clone
}

func NewDocument(text string, media *media.Media) (*Document, error) {
	if text == "" && media == nil {
		return nil, fmt.Errorf("document: create: %w: text or media is required", ErrInvalidDocument)
	}

	doc := &Document{
		Text:     text,
		Media:    media,
		Metadata: metadata.Map{},
	}
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	return doc, nil
}

func (d *Document) Validate() error {
	if d == nil {
		return fmt.Errorf("%w: receiver is nil", ErrInvalidDocument)
	}
	if d.Text == "" && d.Media == nil {
		return fmt.Errorf("%w: text or media is required", ErrInvalidDocument)
	}
	if d.Media != nil {
		if err := d.Media.Validate(); err != nil {
			return fmt.Errorf("%w: media: %w", ErrInvalidDocument, err)
		}
	}
	if err := d.Metadata.Validate(); err != nil {
		return fmt.Errorf("%w: metadata: %w", ErrInvalidDocument, err)
	}
	return nil
}
