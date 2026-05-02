package chat

import (
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/pkg/text"
)

// PromptTemplate is a fluent builder for chat messages whose text comes
// from a Go-template-rendered string and whose attachments come from a
// media collection. Use it to keep prompt construction declarative and
// reusable.
//
// Example:
//
//	tmpl := chat.NewPromptTemplate().
//	    WithTemplate("Hello {{.name}}, please analyze this image.").
//	    WithVariable("name", "user").
//	    WithMedia(image)
//
//	user, err := tmpl.CreateUserMessage()
type PromptTemplate struct {
	renderer *text.Renderer
	media    []*media.Media
}

// NewPromptTemplate returns an empty [PromptTemplate] ready for fluent
// configuration.
func NewPromptTemplate() *PromptTemplate {
	return &PromptTemplate{
		renderer: text.NewRenderer(),
		media:    make([]*media.Media, 0),
	}
}

// WithTemplate sets the underlying template string. Use Go template
// syntax — variables look like `{{.name}}` by default.
func (p *PromptTemplate) WithTemplate(template string) *PromptTemplate {
	p.renderer.WithTemplate(template)
	return p
}

// WithVariable sets one variable.
func (p *PromptTemplate) WithVariable(name string, value any) *PromptTemplate {
	p.renderer.WithVariable(name, value)
	return p
}

// WithVariables sets variables in bulk; later keys overwrite earlier
// ones for duplicates.
func (p *PromptTemplate) WithVariables(variables map[string]any) *PromptTemplate {
	p.renderer.WithVariables(variables)
	return p
}

// WithMedia appends media attachments. Pass multiple values in one call
// to add several at once. Attachments only flow into [UserMessage]s —
// system messages are text-only.
func (p *PromptTemplate) WithMedia(items ...*media.Media) *PromptTemplate {
	if len(items) > 0 {
		p.media = append(p.media, items...)
	}
	return p
}

// RequireVariables checks that the named variables actually appear as
// placeholders in the template. The check is a literal string match
// against the current delimiters — it does not understand template
// expressions like `{{.User.Name}}`.
//
// Example:
//
//	tmpl.RequireVariables("user", "message") // checks for {{.user}} and {{.message}}
func (p *PromptTemplate) RequireVariables(names ...string) error {
	return p.renderer.RequireVariables(names...)
}

// Clone returns a deep copy. Renderer state and media list are
// independent, so post-clone mutations don't leak. nil receiver yields
// nil.
func (p *PromptTemplate) Clone() *PromptTemplate {
	if p == nil {
		return nil
	}
	return &PromptTemplate{
		renderer: p.renderer.Clone(),
		media:    slices.Clone(p.media),
	}
}

// Render evaluates the template with the configured variables and
// returns the resulting string. Errors are wrapped with the
// "chat.PromptTemplate" prefix.
func (p *PromptTemplate) Render() (string, error) {
	content, err := p.renderer.Render()
	if err != nil {
		return "", fmt.Errorf("chat.PromptTemplate.Render: %w", err)
	}
	return content, nil
}

// RenderWithVariables renders against a clone whose variables have been
// extended with the supplied map. The original template is not modified.
// Per-call variables override existing values for duplicate keys.
func (p *PromptTemplate) RenderWithVariables(variables map[string]any) (string, error) {
	rc := p.renderer.Clone()
	for key, val := range variables {
		rc.WithVariable(key, val)
	}

	content, err := rc.Render()
	if err != nil {
		return "", fmt.Errorf("chat.PromptTemplate.RenderWithVariables: %w", err)
	}
	return content, nil
}

// CreateSystemMessage renders the template and wraps the result in a
// [SystemMessage]. Media is not attached — system messages do not carry
// it.
func (p *PromptTemplate) CreateSystemMessage() (*SystemMessage, error) {
	content, err := p.renderer.Render()
	if err != nil {
		return nil, fmt.Errorf("chat.PromptTemplate.CreateSystemMessage: %w", err)
	}
	return NewSystemMessage(content), nil
}

// CreateUserMessage renders the template and wraps the result in a
// [UserMessage] together with all configured media.
func (p *PromptTemplate) CreateUserMessage() (*UserMessage, error) {
	content, err := p.renderer.Render()
	if err != nil {
		return nil, fmt.Errorf("chat.PromptTemplate.CreateUserMessage: %w", err)
	}
	return NewUserMessage(MessageParams{
		Text:  content,
		Media: p.media,
	}), nil
}

