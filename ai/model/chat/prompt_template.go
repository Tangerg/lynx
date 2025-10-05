package chat

import (
	"slices"

	"github.com/Tangerg/lynx/ai/media"
	"github.com/Tangerg/lynx/pkg/text"
)

// PromptTemplate provides a builder for creating chat messages with
// template rendering and media attachment support.
type PromptTemplate struct {
	renderer *text.Renderer
	media    []*media.Media
}

// NewPromptTemplate creates a new PromptTemplate instance with
// an initialized renderer and empty media collection.
func NewPromptTemplate() *PromptTemplate {
	return &PromptTemplate{
		renderer: text.NewRenderer(),
		media:    make([]*media.Media, 0),
	}
}

// WithTemplate sets the template string to be rendered.
// Returns the template for method chaining.
func (p *PromptTemplate) WithTemplate(template string) *PromptTemplate {
	p.renderer.WithTemplate(template)
	return p
}

// WithVariable adds a single template variable with its value.
// Returns the template for method chaining.
func (p *PromptTemplate) WithVariable(name string, value any) *PromptTemplate {
	p.renderer.WithVariable(name, value)
	return p
}

// WithVariables adds multiple template variables from a map.
// Returns the template for method chaining.
func (p *PromptTemplate) WithVariables(variables map[string]any) *PromptTemplate {
	p.renderer.WithVariables(variables)
	return p
}

// WithMedia appends one or more media attachments to the template.
// Returns the template for method chaining.
func (p *PromptTemplate) WithMedia(media ...*media.Media) *PromptTemplate {
	p.media = append(p.media, media...)
	return p
}

// Clone creates a deep copy of the PromptTemplate with its
// renderer state and media attachments.
func (p *PromptTemplate) Clone() *PromptTemplate {
	return &PromptTemplate{
		renderer: p.renderer.Clone(),
		media:    slices.Clone(p.media),
	}
}

// RenderSystemMessage renders the template and creates a SystemMessage.
// Returns an error if template rendering fails.
func (p *PromptTemplate) RenderSystemMessage() (*SystemMessage, error) {
	contentText, err := p.renderer.Render()
	if err != nil {
		return nil, err
	}

	return NewSystemMessage(contentText), nil
}

// RenderUserMessage renders the template and creates a UserMessage with
// text content and any attached media. Returns an error if rendering fails.
func (p *PromptTemplate) RenderUserMessage() (*UserMessage, error) {
	contentText, err := p.renderer.Render()
	if err != nil {
		return nil, err
	}

	return NewUserMessage(MessageParams{
		Text:  contentText,
		Media: p.media,
	}), nil
}
