package document

import (
	"github.com/Tangerg/lynx/ai/core/document/id"
	"github.com/Tangerg/lynx/ai/core/model/media"
)

var _ media.Content = (*Document)(nil)

type Document struct {
	id               string
	metadata         map[string]any
	content          string
	media            []*media.Media
	embedding        []float64
	contentFormatter ContentFormatter
}

func (d *Document) Id() string {
	return d.id
}

func (d *Document) Content() string {
	return d.content
}

func (d *Document) Metadata() map[string]any {
	return d.metadata
}

func (d *Document) Media() []*media.Media {
	return d.media
}

func (d *Document) SetEmbedding(embedding []float64) *Document {
	d.embedding = embedding
	return d
}

func (d *Document) Embedding() []float64 {
	return d.embedding
}
func (d *Document) SetContentFormatter(formatter ContentFormatter) *Document {
	d.contentFormatter = formatter
	return d
}
func (d *Document) ContentFormatter() ContentFormatter {
	return d.contentFormatter
}
func (d *Document) FormattedContent() string {
	return d.FormattedContentByMetadataMode(All)
}
func (d *Document) FormattedContentByMetadataMode(mode MetadataMode) string {
	return d.contentFormatter.Format(d, mode)
}

func (d *Document) FormattedContentByFormatterAndMetadataMode(formatter ContentFormatter, mode MetadataMode) string {
	return formatter.Format(d, mode)
}

func NewBuilder() *Builder {
	return &Builder{
		document: &Document{
			metadata: make(map[string]any),
		},
		idGenerator: new(id.UUIDGenerator),
	}
}

type Builder struct {
	document    *Document
	idGenerator id.Generator
}

func (b *Builder) WithId(id string) *Builder {
	b.document.id = id
	return b
}
func (b *Builder) WithContent(content string) *Builder {
	b.document.content = content
	return b
}
func (b *Builder) WithMedia(media ...*media.Media) *Builder {
	b.document.media = append(b.document.media, media...)
	return b
}
func (b *Builder) WithMetadata(metadata map[string]any) *Builder {
	for k, v := range metadata {
		b.document.metadata[k] = v
	}
	return b
}
func (b *Builder) WithIdGenerator(idGenerator id.Generator) *Builder {
	b.idGenerator = idGenerator
	return b
}
func (b *Builder) Build() *Document {
	if b.document.id == "" {
		b.document.id = b.idGenerator.GenerateId(b.document.content, b.document.metadata)
	}
	return b.document
}
