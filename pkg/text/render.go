package text

import (
	"fmt"
	"maps"
	"strings"
	"text/template"

	"github.com/Tangerg/lynx/pkg/assert"
)

// Renderer is a fluent interface for rendering text templates.
// It wraps Go's text/template package to provide a more convenient API.
// Uses caching to avoid re-rendering when configuration hasn't changed.
//
// NOTE: This type is NOT thread-safe. Do not use the same Renderer instance
// concurrently from multiple goroutines without proper synchronization.
// Create separate instances for each goroutine if concurrent usage is required.
type Renderer struct {
	templateString string         // Template string to be rendered
	variables      map[string]any // Variables to be injected into the template
	leftDelimiter  string         // Left delimiter for template variables
	rightDelimiter string         // Right delimiter for template variables
	changed        bool           // Flag to track if configuration has changed since last render
	renderedString string         // Cached rendered result
}

// NewRenderer creates a new Renderer instance with default settings.
// Default delimiters are "{{" and "}}".
func NewRenderer() *Renderer {
	renderer := &Renderer{}
	renderer.init()
	return renderer
}

// init initializes the renderer with default values.
func (r *Renderer) init() {
	r.templateString = ""
	r.variables = make(map[string]any)
	r.leftDelimiter = "{{"
	r.rightDelimiter = "}}"
	r.changed = false
	r.renderedString = ""
}

// markChanged marks the renderer as changed and clears the cached result.
// This method should be called whenever any configuration that affects
// the rendering output is modified.
func (r *Renderer) markChanged() {
	r.changed = true
	r.renderedString = ""
}

// WithTemplate sets the template string to be rendered.
// Returns the receiver for method chaining.
// An empty templateString is accepted and will be set as-is.
func (r *Renderer) WithTemplate(templateString string) *Renderer {
	r.templateString = templateString
	r.markChanged()
	return r
}

// WithVariable sets a single variable that can be used in the template.
// Returns the receiver for method chaining.
// Note: Empty variableName is accepted and will create a variable with empty string key.
// Existing variable with the same key will be overwritten.
func (r *Renderer) WithVariable(variableName string, variableValue any) *Renderer {
	r.variables[variableName] = variableValue
	r.markChanged()
	return r
}

// WithVariables replaces all existing variables with the provided map.
// This method clears any previously set variables before adding the new ones.
// Returns the receiver for method chaining.
// Nil or empty map will clear all variables.
func (r *Renderer) WithVariables(variableMap map[string]any) *Renderer {
	// Clear existing variables
	clear(r.variables)

	// Add new variables
	for variableName, variableValue := range variableMap {
		r.variables[variableName] = variableValue
	}

	r.markChanged()
	return r
}

// Reset clears all configuration and returns the renderer to its initial state.
// Returns the receiver for method chaining.
func (r *Renderer) Reset() *Renderer {
	r.init()
	return r
}

// Clone creates a deep copy of the renderer with all its current configuration and state.
// The cloned renderer is independent and modifications to it won't affect the original.
// The cached rendered result is also copied, preserving the cache state.
// Returns a new Renderer instance with the same settings as the original.
func (r *Renderer) Clone() *Renderer {
	clonedRenderer := NewRenderer()

	// Copy all configuration
	clonedRenderer.templateString = r.templateString
	clonedRenderer.variables = maps.Clone(r.variables)
	clonedRenderer.leftDelimiter = r.leftDelimiter
	clonedRenderer.rightDelimiter = r.rightDelimiter
	clonedRenderer.changed = r.changed
	clonedRenderer.renderedString = r.renderedString

	return clonedRenderer
}

// WithDelimiters sets the action delimiters to the specified strings.
// An empty delimiter stands for the corresponding default: {{ or }}.
// Returns the receiver for method chaining.
func (r *Renderer) WithDelimiters(leftDelimiter, rightDelimiter string) *Renderer {
	r.leftDelimiter = leftDelimiter
	r.rightDelimiter = rightDelimiter
	r.markChanged()
	return r
}

// render performs the actual template rendering.
// It creates a new template, parses the template string, and executes it with the variables.
// Returns the rendered string or an error if template parsing or execution fails.
// The error is returned as-is from the underlying text/template package without wrapping.
func (r *Renderer) render() (string, error) {
	// Create and parse template
	templateInstance, err := template.
		New("renderer").
		Delims(r.leftDelimiter, r.rightDelimiter).
		Parse(r.templateString)
	if err != nil {
		return "", err
	}

	// Execute template with variables
	var outputBuilder strings.Builder
	err = templateInstance.Execute(&outputBuilder, r.variables)
	if err != nil {
		return "", err
	}

	return outputBuilder.String(), nil
}

// Render renders the template with the configured variables and returns the result.
// Uses caching to avoid re-rendering when configuration hasn't changed since last render.
// Returns an empty string and nil error if no template is set.
// Returns an error if template parsing or execution fails.
// The cached result is updated only after successful rendering.
func (r *Renderer) Render() (string, error) {
	// Return empty string if no template is set
	if r.templateString == "" {
		return "", nil
	}

	// Re-render if configuration has changed
	if r.changed {
		renderedOutput, err := r.render()
		if err != nil {
			return "", err
		}

		r.renderedString = renderedOutput
		r.changed = false
	}

	return r.renderedString, nil
}

// MustRender renders the template and panics if an error occurs.
// This is a convenience method for cases where errors should not be handled gracefully.
// Uses the same caching mechanism as Render().
// Panics with the error returned from Render() if rendering fails.
func (r *Renderer) MustRender() string {
	return assert.Must(r.Render())
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
func (r *Renderer) RequireVariables(variableNames ...string) error {
	missingVariables := make([]string, 0, len(variableNames))

	for _, varName := range variableNames {
		// Construct the full placeholder with current delimiters
		placeholder := r.leftDelimiter + "." + varName + r.rightDelimiter

		if !strings.Contains(r.templateString, placeholder) {
			missingVariables = append(missingVariables, varName)
		}
	}

	if len(missingVariables) > 0 {
		return fmt.Errorf("the following variables must be present in the template: %s",
			strings.Join(missingVariables, ", "))
	}
	return nil
}

// Render is a convenience function that creates a new renderer,
// sets the template and variables, and renders the result in one call.
// Returns the rendered string and any error that occurred.
//
// This function is thread-safe as it creates a new Renderer instance for each call.
// Equivalent to: NewRenderer().WithTemplate(templateString).WithVariables(templateData).Render()
func Render(templateString string, templateData map[string]any) (string, error) {
	return NewRenderer().
		WithTemplate(templateString).
		WithVariables(templateData).
		Render()
}

// MustRender is a convenience function that creates a new renderer,
// sets the template and variables, and renders the result in one call.
// Panics if an error occurs during rendering.
//
// This function is thread-safe as it creates a new Renderer instance for each call.
// Equivalent to: NewRenderer().WithTemplate(templateString).WithVariables(templateData).MustRender()
func MustRender(templateString string, templateData map[string]any) string {
	return NewRenderer().
		WithTemplate(templateString).
		WithVariables(templateData).
		MustRender()
}
