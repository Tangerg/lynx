package vectorstore

import (
	"errors"
	"github.com/Tangerg/lynx/ai/commons/content"
	"github.com/Tangerg/lynx/ai/commons/document"
	"maps"
)

var _ content.Content = (*Content)(nil)

type Content struct {
	id        string
	text      string
	metadata  map[string]any
	embedding []float64
}

func NewContent(id string, text string, metadata map[string]any, embedding []float64) (*Content, error) {
	if id == "" {
		return nil, errors.New("id must not be empty")
	}
	if text == "" {
		return nil, errors.New("text must not be empty")
	}

	return &Content{
		id:        id,
		text:      text,
		metadata:  metadata,
		embedding: embedding,
	}, nil
}

func (c *Content) ID() string {
	return c.id
}

func (c *Content) Text() string {
	return c.text
}

func (c *Content) Metadata() map[string]any {
	return c.metadata
}

func (c *Content) Embedding() []float64 {
	return c.embedding
}

func (c *Content) ToDocument() *document.Document {
	doc, _ := document.
		NewBuilder().
		WithID(c.id).
		WithText(c.text).
		WithMetadata(maps.Clone(c.metadata)).
		Build()
	return doc
}
