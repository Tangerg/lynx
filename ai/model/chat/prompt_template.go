package chat

import (
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/ai/media"
	"github.com/Tangerg/lynx/pkg/text"
)

// PromptTemplate is a builder for constructing chat messages with template rendering
// and media attachment capabilities. It provides a fluent interface for configuring
// templates, variables, and media content before generating specific message types.
//
// Example usage:
//
//	template := NewPromptTemplate().
//	    WithTemplate("Hello {{.name}}, please analyze this image").
//	    WithVariable("name", "user").
//	    WithMedia(imageMedia)
//	userMsg, err := template.CreateUserMessage()
type PromptTemplate struct {
	renderer *text.Renderer
	media    []*media.Media
}

// NewPromptTemplate creates a new PromptTemplate instance with an initialized
// text renderer and an empty media collection.
func NewPromptTemplate() *PromptTemplate {
	return &PromptTemplate{
		renderer: text.NewRenderer(),
		media:    make([]*media.Media, 0),
	}
}

// WithTemplate sets the template string to be rendered. The template should follow
// Go template syntax and can contain variables in the form of {{.variableName}}.
//
// Returns the PromptTemplate instance for method chaining.
func (p *PromptTemplate) WithTemplate(template string) *PromptTemplate {
	p.renderer.WithTemplate(template)
	return p
}

// WithVariable adds a single template variable with its corresponding value.
// The variable can be referenced in the template using {{.name}} syntax.
//
// Parameters:
//   - name: the variable name (without dots or braces)
//   - value: the value to substitute, can be any type
//
// Returns the PromptTemplate instance for method chaining.
func (p *PromptTemplate) WithVariable(name string, value any) *PromptTemplate {
	p.renderer.WithVariable(name, value)
	return p
}

// WithVariables adds multiple template variables from a map in a single call.
// Each key-value pair in the map will be available as a template variable.
//
// Parameters:
//   - variables: a map where keys are variable names and values are their substitutions
//
// Returns the PromptTemplate instance for method chaining.
func (p *PromptTemplate) WithVariables(variables map[string]any) *PromptTemplate {
	p.renderer.WithVariables(variables)
	return p
}

// WithMedia replaces all existing media attachments with the provided ones.
// These media items will be included when creating user messages (system messages
// do not support media).
//
// Parameters:
//   - media: one or more media pointers to set as the new media collection
//
// Returns the PromptTemplate instance for method chaining.
func (p *PromptTemplate) WithMedia(media ...*media.Media) *PromptTemplate {
	if len(media) > 0 {
		p.media = append(p.media, media...)
	}
	return p
}

// RequireVariables verifies that all specified template variables exist in the template.
// Automatically constructs the placeholder format using current delimiters and dot notation.
// Returns an error if any of the variables are not found in the template.
//
// Note: This method performs literal string matching of the constructed placeholders.
// It does not account for:
//   - Template syntax validity beyond simple string matching
//   - Variables within comments or string literals
//   - Complex template expressions (e.g., {{.User.Name}})
//
// Example:
//
//	r.RequireVariables("user", "message")
//	// Will check for "{{.user}}" and "{{.message}}" with default delimiters
//
//	r.WithDelimiters("[[", "]]").RequireVariables("user")
//	// Will check for "[[.user]]"
func (p *PromptTemplate) RequireVariables(variableNames ...string) error {
	return p.renderer.RequireVariables(variableNames...)
}

// Clone creates a deep copy of the PromptTemplate, including its renderer state
// and media attachments. This is useful for template reuse scenarios or when you
// need to create variations of a base template without affecting the original.
//
// Returns nil if the receiver is nil.
func (p *PromptTemplate) Clone() *PromptTemplate {
	if p == nil {
		return nil
	}
	return &PromptTemplate{
		renderer: p.renderer.Clone(),
		media:    slices.Clone(p.media),
	}
}

// Render processes the template with the current variables and returns the
// resulting text content. This method does not create a message object.
//
// Returns:
//   - string: the rendered text content
//   - error: any error that occurred during template rendering
func (p *PromptTemplate) Render() (string, error) {
	content, err := p.renderer.Render()
	if err != nil {
		return "", fmt.Errorf("failed to render template: %w", err)
	}
	return content, nil
}

// RenderWithVariables renders the template with additional variables merged with
// existing ones. The original template's variables are not modified - a cloned
// renderer is used for the operation.
//
// Parameters:
//   - variables: additional variables to merge with existing template variables
//
// Returns:
//   - string: the rendered text content
//   - error: any error that occurred during template rendering
//
// Note: If a variable name conflicts with an existing one, the provided value
// will override the original.
func (p *PromptTemplate) RenderWithVariables(variables map[string]any) (string, error) {
	clonedRenderer := p.renderer.Clone()
	for key, val := range variables {
		clonedRenderer.WithVariable(key, val)
	}
	content, err := clonedRenderer.Render()
	if err != nil {
		return "", fmt.Errorf("failed to render template with additional variables: %w", err)
	}
	return content, nil
}

// CreateSystemMessage renders the template and creates a SystemMessage with the
// resulting content. System messages typically contain instructions or context
// for the AI model and should not include media attachments.
//
// Returns:
//   - *SystemMessage: the created system message
//   - error: any error that occurred during template rendering
func (p *PromptTemplate) CreateSystemMessage() (*SystemMessage, error) {
	content, err := p.renderer.Render()
	if err != nil {
		return nil, fmt.Errorf("failed to create system message: %w", err)
	}

	return NewSystemMessage(content), nil
}

// CreateUserMessage renders the template and creates a UserMessage containing
// both the rendered text content and any attached media. This is the primary
// method for generating user-facing prompts with multimedia content.
//
// Returns:
//   - *UserMessage: the created user message with text and media
//   - error: any error that occurred during template rendering
func (p *PromptTemplate) CreateUserMessage() (*UserMessage, error) {
	content, err := p.renderer.Render()
	if err != nil {
		return nil, fmt.Errorf("failed to create user message: %w", err)
	}

	return NewUserMessage(MessageParams{
		Text:  content,
		Media: p.media,
	}), nil
}
