package document

import (
	"errors"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/ai/content"
	"github.com/Tangerg/lynx/pkg/assert"
)

// Document represents a unit of information containing text, media,
// metadata, and a relevance score.
type Document struct {
	id        string
	text      string
	media     *content.Media
	metadata  map[string]any
	score     float64
	formatter Formatter
}

// NewDocument creates a new Document with the specified attributes.
// Returns an error if ID is empty or both text and media are nil.
// TODO use Document Config struct create
func NewDocument(id string, text string, media *content.Media, metadata map[string]any, score float64) (*Document, error) {
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
		formatter: NewNop(),
	}, nil
}

// ID returns the document's unique identifier.
func (d *Document) ID() string {
	return d.id
}

// Text returns the document's text content.
func (d *Document) Text() string {
	return d.text
}

// Media returns the document's associated media content.
func (d *Document) Media() *content.Media {
	return d.media
}

// Metadata returns the document's metadata as a key-value map.
func (d *Document) Metadata() map[string]any {
	return d.metadata
}

// Score returns the document's relevance or ranking score.
func (d *Document) Score() float64 {
	return d.score
}

// Formatter returns the document's current formatter.
func (d *Document) Formatter() Formatter {
	return d.formatter
}

// Format returns the document formatted with all metadata using
// the default formatter.
func (d *Document) Format() string {
	return d.FormatByMetadataMode(MetadataModeAll)
}

// FormatByMetadataMode formats the document according to the specified
// metadata mode using the document's assigned formatter.
func (d *Document) FormatByMetadataMode(mode MetadataMode) string {
	return d.FormatByMetadataModeWithFormatter(mode, d.formatter)
}

// FormatByMetadataModeWithFormatter formats the document using the
// specified formatter and metadata mode.
func (d *Document) FormatByMetadataModeWithFormatter(mode MetadataMode, formatter Formatter) string {
	return formatter.Format(d, mode)
}

// SetFormatter sets the document's formatter.
// Does nothing if the provided formatter is nil.
func (d *Document) SetFormatter(formatter Formatter) {
	if formatter == nil {
		return
	}

	d.formatter = formatter
}

// Builder implements the builder pattern for creating Document objects
// with optional fields and validation.
type Builder struct {
	id       string
	text     string
	media    *content.Media
	metadata map[string]any
	score    float64
}

// NewBuilder creates a new Document Builder instance.
func NewBuilder() *Builder {
	return &Builder{}
}

// WithID sets the ID for the document being built.
func (b *Builder) WithID(id string) *Builder {
	b.id = id
	return b
}

// WithText sets the text content for the document being built.
func (b *Builder) WithText(text string) *Builder {
	b.text = text
	return b
}

// WithMedia sets the media content for the document being built.
func (b *Builder) WithMedia(media *content.Media) *Builder {
	b.media = media
	return b
}

// WithMetadata sets the metadata for the document being built.
func (b *Builder) WithMetadata(metadata map[string]any) *Builder {
	b.metadata = metadata
	return b
}

// WithScore sets the relevance score for the document being built.
func (b *Builder) WithScore(score float64) *Builder {
	b.score = score
	return b
}

// Build creates a Document instance using the configured parameters.
// Generates a UUID if no ID was set explicitly.
func (b *Builder) Build() (*Document, error) {
	if b.id == "" {
		b.id = uuid.New().String()
	}

	return NewDocument(b.id, b.text, b.media, b.metadata, b.score)
}

// MustBuild creates a Document instance or panics if creation fails.
// Should only be used when you're certain the build will succeed.
func (b *Builder) MustBuild() *Document {
	return assert.ErrorIsNil(b.Build())
}
