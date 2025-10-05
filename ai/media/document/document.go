package document

import (
	"errors"

	"github.com/Tangerg/lynx/ai/media"
)

// Document represents a structured information unit containing textual content,
// media attachments, metadata, and an optional relevance score.
type Document struct {
	ID        string
	Score     float64
	Text      string
	Media     *media.Media
	Formatter Formatter
	Metadata  map[string]any
}

// NewDocument creates a new Document with the provided text and/or media.
// Returns an error if both text and media are empty/nil.
func NewDocument(text string, media *media.Media) (*Document, error) {
	if text == "" && media == nil {
		return nil, errors.New("document requires either text content or media attachment")
	}

	return &Document{
		Text:      text,
		Media:     media,
		Metadata:  make(map[string]any),
		Formatter: NewNop(),
	}, nil
}

// Format returns the formatted document string including all metadata
// using the document's default formatter.
func (d *Document) Format() string {
	return d.FormatByMetadataMode(MetadataModeAll)
}

// FormatByMetadataMode formats the document with the specified metadata mode
// using the document's assigned formatter.
func (d *Document) FormatByMetadataMode(mode MetadataMode) string {
	return d.FormatByMetadataModeWithFormatter(mode, d.Formatter)
}

// FormatByMetadataModeWithFormatter formats the document using a custom formatter
// and metadata mode. Falls back to no-op formatter if nil is provided.
func (d *Document) FormatByMetadataModeWithFormatter(mode MetadataMode, formatter Formatter) string {
	if formatter == nil {
		formatter = NewNop()
	}
	return formatter.Format(d, mode)
}
