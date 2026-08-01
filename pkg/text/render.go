package text

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/Tangerg/lynx/pkg/assert"
)

const (
	defaultLeftDelimiter  = "{{"
	defaultRightDelimiter = "}}"
)

// Renderer is a reusable typed text/template renderer. It caches only the
// parsed template, never rendered output, so mutations visible through T cannot
// leave a stale result behind. A Renderer is not safe for concurrent mutation.
// For one-off rendering use [Render] or [MustRender].
type Renderer[T any] struct {
	templateSource string
	data           T
	leftDelimiter  string
	rightDelimiter string
	parsed         *template.Template
}

// NewRenderer returns a renderer with data and the standard template
// delimiters.
func NewRenderer[T any](data T) *Renderer[T] {
	return &Renderer[T]{
		data:           data,
		leftDelimiter:  defaultLeftDelimiter,
		rightDelimiter: defaultRightDelimiter,
	}
}

// SetTemplate replaces the template source and invalidates the parsed form.
func (r *Renderer[T]) SetTemplate(source string) *Renderer[T] {
	r.templateSource = source
	r.parsed = nil
	return r
}

// SetData replaces the typed execution data. The parsed template remains
// reusable because parsing is independent of data.
func (r *Renderer[T]) SetData(data T) *Renderer[T] {
	r.data = data
	return r
}

// SetDelimiters replaces the action delimiters and invalidates the parsed
// template. An empty side resets that side to its standard delimiter.
func (r *Renderer[T]) SetDelimiters(left, right string) *Renderer[T] {
	if left == "" {
		left = defaultLeftDelimiter
	}
	if right == "" {
		right = defaultRightDelimiter
	}
	r.leftDelimiter = left
	r.rightDelimiter = right
	r.parsed = nil
	return r
}

// Reset returns r to its initial configuration with replacement data.
func (r *Renderer[T]) Reset(data T) *Renderer[T] {
	r.templateSource = ""
	r.data = data
	r.leftDelimiter = defaultLeftDelimiter
	r.rightDelimiter = defaultRightDelimiter
	r.parsed = nil
	return r
}

// Clone returns an independent renderer with the same configuration and data.
// T follows ordinary Go assignment semantics; referenced values remain owned by
// the caller and must obey their own concurrency rules.
func (r *Renderer[T]) Clone() *Renderer[T] {
	return &Renderer[T]{
		templateSource: r.templateSource,
		data:           r.data,
		leftDelimiter:  r.leftDelimiter,
		rightDelimiter: r.rightDelimiter,
	}
}

// Render parses the template when necessary and executes it with the current
// data. An empty template renders as an empty string. Missing map keys are
// errors rather than implicit "<no value>" text.
func (r *Renderer[T]) Render() (string, error) {
	if r.templateSource == "" {
		return "", nil
	}
	if r.parsed == nil {
		parsed, err := template.New("renderer").
			Delims(r.leftDelimiter, r.rightDelimiter).
			Option("missingkey=error").
			Parse(r.templateSource)
		if err != nil {
			return "", fmt.Errorf("text: parse template: %w", err)
		}
		r.parsed = parsed
	}
	var output strings.Builder
	if err := r.parsed.Execute(&output, r.data); err != nil {
		return "", fmt.Errorf("text: execute template: %w", err)
	}
	return output.String(), nil
}

// MustRender is like [Renderer.Render] but panics on error.
func (r *Renderer[T]) MustRender() string {
	return assert.Must(r.Render())
}

// RequireVariables returns an error listing names whose direct textual
// placeholder is absent from the template. Matching is intentionally literal;
// nested expressions such as "{{.User.Name}}" are not direct placeholders.
func (r *Renderer[T]) RequireVariables(names ...string) error {
	missing := make([]string, 0, len(names))
	for _, name := range names {
		if name == "" || name != strings.TrimSpace(name) {
			return fmt.Errorf("text: invalid template variable name %q", name)
		}
		if !strings.Contains(r.templateSource, r.leftDelimiter+"."+name+r.rightDelimiter) {
			missing = append(missing, name)
		}
	}
	if len(missing) != 0 {
		return fmt.Errorf("text: template missing required variables: %s", strings.Join(missing, ", "))
	}
	return nil
}

// Render executes a one-shot typed template.
func Render[T any](source string, data T) (string, error) {
	return NewRenderer(data).SetTemplate(source).Render()
}

// MustRender is the panicking variant of [Render].
func MustRender[T any](source string, data T) string {
	return NewRenderer(data).SetTemplate(source).MustRender()
}
