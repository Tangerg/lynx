package document

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/ai/content"
)

const (
	MetadataDistance = "distance"
)

// Document represents a unit of information that may contain text, media,
// metadata, and a relevance score.
type Document struct {
	id        string
	text      string
	media     *content.Media
	metadata  map[string]any
	score     float32
	formatter Formatter
}

// NewDocument creates a new Document with the specified attributes.
// Returns an error if validation fails or required fields are missing.
// Automatically initializes empty metadata map and default formatter if needed.
func NewDocument(id string, text string, media *content.Media, metadata map[string]any, score float32) (*Document, error) {
	if id == "" {
		return nil, errors.New("failed to create document: ID cannot be empty")
	}

	if text == "" && media == nil {
		return nil, errors.New("failed to create document: at least one of text or media must be specified")
	}

	if metadata == nil {
		metadata = make(map[string]any)
	}

	return &Document{
		id:        id,
		text:      text,
		media:     media,
		metadata:  metadata,
		score:     score,
		formatter: NewDefaultFormatterBuilder().Build(),
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

// Formatter returns the document's Formatter
func (d *Document) Formatter() Formatter {
	return d.formatter
}

// Format returns the document content formatted with all metadata.
// This is a convenience method that calls FormatByMetadataMode with MetadataModeAll.
func (d *Document) Format() string {
	return d.FormatByMetadataMode(MetadataModeAll)
}

// FormatByMetadataMode returns the document formatted according to the
// specified metadata mode using the document's assigned formatter.
func (d *Document) FormatByMetadataMode(mode MetadataMode) string {
	return d.FormatByMetadataModeWithFormatter(mode, d.formatter)
}

// FormatByMetadataModeWithFormatter returns the document formatted
// using the specified formatter and metadata mode.
func (d *Document) FormatByMetadataModeWithFormatter(mode MetadataMode, formatter Formatter) string {
	return formatter.Format(d, mode)
}

// SetFormatter sets the Formatter for the document
func (d *Document) SetFormatter(formatter Formatter) {
	if formatter == nil {
		return
	}

	d.formatter = formatter
}

// Builder implements the builder pattern for creating Document objects
type Builder struct {
	id          string
	text        string
	media       *content.Media
	metadata    map[string]any
	score       float32
	idGenerator IDGenerator
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
func (b *Builder) WithIDGenerator(idGenerator IDGenerator) *Builder {
	b.idGenerator = idGenerator
	return b
}

// Build creates a new Document instance using the configured parameters.
// If no ID was set, it requires an ID generator to create one automatically.
func (b *Builder) Build() (*Document, error) {
	if b.id == "" {
		if b.idGenerator != nil {
			b.id = b.idGenerator.Generate(context.Background(), b.text, b.media, b.metadata)
		} else {
			b.id = uuid.New().String()
		}
	}

	return NewDocument(b.id, b.text, b.media, b.metadata, b.score)
}
