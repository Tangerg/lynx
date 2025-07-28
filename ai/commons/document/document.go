package document

import (
	"errors"

	"github.com/Tangerg/lynx/ai/commons/content"
	"github.com/Tangerg/lynx/ai/commons/document/id"
)

const (
	MetadataDistance = "distance"
)

// Document represents a unit of information that may contain text, media, metadata, and a relevance score.
type Document struct {
	id               string           // Unique identifier for the document
	text             string           // Text content of the document
	media            *content.Media   // Media content associated with the document (optional)
	metadata         map[string]any   // Additional metadata as key-value pairs
	score            float32          // Relevance or ranking score for the document
	contentFormatter ContentFormatter // Formatter used to generate string representations of the document
}

// NewDocument creates a new Document with the specified attributes.
// Returns an error if validation fails:
// - id must not be empty
// - at least one of text or media must be specified
//
// The function automatically initializes:
// - An empty metadata map if metadata is nil
// - A default content formatter using the DefaultContentFormatter
//
// Parameters:
//
//	id       - Unique identifier for the document
//	text     - Text content of the document
//	media    - Media content associated with the document (optional)
//	metadata - Additional metadata as key-value pairs
//	score    - Relevance or ranking score for the document
//
// Returns:
//
//	A new Document instance and nil error if successful
//	nil and an error if validation fails
func NewDocument(id string, text string, media *content.Media, metadata map[string]any, score float32) (*Document, error) {
	if id == "" {
		return nil, errors.New("id cannot empty")
	}
	if text == "" && media == nil {
		return nil, errors.New("exactly one of text or media must be specified")
	}
	if metadata == nil {
		metadata = make(map[string]any)
	}
	return &Document{
		id:               id,
		text:             text,
		media:            media,
		metadata:         metadata,
		score:            score,
		contentFormatter: NewDefaultContentFormatterBuilder().Build(),
	}, nil
}

// ID returns the document's unique identifier
func (d *Document) ID() string {
	return d.id
}

// Text returns the document's text content
func (d *Document) Text() string {
	return d.text
}

// Media returns the document's associated media content
func (d *Document) Media() *content.Media {
	return d.media
}

// Metadata returns the document's metadata as a key-value map
func (d *Document) Metadata() map[string]any {
	return d.metadata
}

// Score returns the document's relevance or ranking score
func (d *Document) Score() float32 {
	return d.score
}

// ContentFormatter returns the document's ContentFormatter
func (d *Document) ContentFormatter() ContentFormatter {
	return d.contentFormatter
}

// FormattedContent returns the document content formatted with all metadata.
// This is a convenience method that calls FormattedContentByMetadataMode with MetadataModeAll.
//
// Returns:
//
//	A formatted string representation of the document with all metadata
func (d *Document) FormattedContent() string {
	return d.FormattedContentByMetadataMode(MetadataModeAll)
}

// FormattedContentByMetadataMode returns the document formatted according to the specified metadata mode.
// Uses the document's assigned content formatter or a default formatter if none is assigned.
//
// Parameters:
//
//	mode - Specifies which metadata to include in the formatted output
//
// Returns:
//
//	A formatted string representation of the document
func (d *Document) FormattedContentByMetadataMode(mode MetadataMode) string {
	return d.FormattedContentByMetadataModeWithContentFormatter(mode, d.contentFormatter)
}

// FormattedContentByMetadataModeWithContentFormatter returns the document formatted
// using the specified formatter and metadata mode.
//
// Parameters:
//
//	mode      - Specifies which metadata to include in the formatted output
//	formatter - The formatter to use for document formatting
//
// Returns:
//
//	A formatted string representation of the document
func (d *Document) FormattedContentByMetadataModeWithContentFormatter(mode MetadataMode, formatter ContentFormatter) string {
	return formatter.Format(d, mode)
}

// SetContentFormatter sets the ContentFormatter for the document
func (d *Document) SetContentFormatter(formatter ContentFormatter) {
	if formatter == nil {
		return
	}
	d.contentFormatter = formatter
}

// Builder implements the builder pattern for creating Document objects
type Builder struct {
	id          string         // ID to assign to the document
	text        string         // Text content to assign to the document
	media       *content.Media // Media content to assign to the document
	metadata    map[string]any // Metadata to assign to the document
	score       float32        // Score to assign to the document
	idGenerator id.Generator   // Generator for creating IDs if not explicitly provided
}

// NewBuilder creates a new Document Builder instance
func NewBuilder() *Builder {
	return &Builder{}
}

// WithID sets the ID for the document being built
func (b *Builder) WithID(id string) *Builder {
	b.id = id
	return b
}

// WithText sets the text content for the document being built
func (b *Builder) WithText(text string) *Builder {
	b.text = text
	return b
}

// WithMedia sets the media content for the document being built
func (b *Builder) WithMedia(media *content.Media) *Builder {
	b.media = media
	return b
}

// WithMetadata sets the metadata for the document being built
func (b *Builder) WithMetadata(metadata map[string]any) *Builder {
	b.metadata = metadata
	return b
}

// WithScore sets the score for the document being built
func (b *Builder) WithScore(score float32) *Builder {
	b.score = score
	return b
}

// WithIDGenerator sets the ID generator for the document being built
func (b *Builder) WithIDGenerator(idGenerator id.Generator) *Builder {
	b.idGenerator = idGenerator
	return b
}

// Build creates a new Document instance using the configured parameters
// If no ID was set and no ID generator was provided, it uses a UUID generator by default
//
// Returns:
//
//	A new Document instance and nil error if successful
//	nil and an error if validation fails
func (b *Builder) Build() (*Document, error) {
	if b.id == "" {
		if b.idGenerator == nil {
			b.idGenerator = id.NewUUIDGenerator()
		}
		b.id = b.idGenerator.GenerateId(b.text, b.metadata)
	}
	return NewDocument(b.id, b.text, b.media, b.metadata, b.score)
}
