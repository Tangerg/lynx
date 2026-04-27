package text

import (
	"fmt"
	"maps"
	"strings"
	"text/template"

	"github.com/Tangerg/lynx/pkg/assert"
)

// Renderer is a fluent wrapper around text/template that caches the
// last rendered output.
//
// A Renderer is not safe for concurrent use. For one-off rendering use
// the package-level [Render] or [MustRender] instead.
type Renderer struct {
	tmpl     string
	vars     map[string]any
	lDelim   string
	rDelim   string
	dirty    bool
	rendered string
}

// NewRenderer returns a new Renderer with default delimiters "{{" and
// "}}".
func NewRenderer() *Renderer {
	r := &Renderer{}
	r.init()
	return r
}

// init resets internal state. Used by [NewRenderer] and [Renderer.Reset].
func (r *Renderer) init() {
	r.tmpl = ""
	r.vars = make(map[string]any)
	r.lDelim = "{{"
	r.rDelim = "}}"
	r.dirty = false
	r.rendered = ""
}

// markDirty invalidates the cached output.
func (r *Renderer) markDirty() {
	r.dirty = true
	r.rendered = ""
}

// WithTemplate sets the template source.
func (r *Renderer) WithTemplate(tmpl string) *Renderer {
	r.tmpl = tmpl
	r.markDirty()
	return r
}

// WithVariable sets a single template variable.
func (r *Renderer) WithVariable(name string, val any) *Renderer {
	r.vars[name] = val
	r.markDirty()
	return r
}

// WithVariables replaces all variables with vars.
func (r *Renderer) WithVariables(vars map[string]any) *Renderer {
	clear(r.vars)
	maps.Copy(r.vars, vars)
	r.markDirty()
	return r
}

// WithDelimiters sets the action delimiters. Empty values keep the
// defaults "{{" and "}}".
func (r *Renderer) WithDelimiters(left, right string) *Renderer {
	r.lDelim = left
	r.rDelim = right
	r.markDirty()
	return r
}

// Reset returns r to its initial state.
func (r *Renderer) Reset() *Renderer {
	r.init()
	return r
}

// Clone returns an independent copy of r, including its cached result.
func (r *Renderer) Clone() *Renderer {
	c := NewRenderer()
	c.tmpl = r.tmpl
	c.vars = maps.Clone(r.vars)
	c.lDelim = r.lDelim
	c.rDelim = r.rDelim
	c.dirty = r.dirty
	c.rendered = r.rendered
	return c
}

// Render parses and executes the template, returning the result. If
// no template is set, it returns "". Successive calls are served from
// the cache as long as no configuration has changed.
func (r *Renderer) Render() (string, error) {
	if r.tmpl == "" {
		return "", nil
	}
	if r.dirty {
		out, err := r.execute()
		if err != nil {
			return "", err
		}
		r.rendered = out
		r.dirty = false
	}
	return r.rendered, nil
}

// MustRender is like [Renderer.Render] but panics on error.
func (r *Renderer) MustRender() string {
	return assert.Must(r.Render())
}

// RequireVariables returns an error listing any names whose textual
// placeholder ("{{.name}}" with current delimiters) is not present in
// the template. Matching is literal — complex expressions like
// "{{.User.Name}}" are not detected.
func (r *Renderer) RequireVariables(names ...string) error {
	missing := make([]string, 0, len(names))
	for _, n := range names {
		if !strings.Contains(r.tmpl, r.lDelim+"."+n+r.rDelim) {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("template missing required variables: %s", strings.Join(missing, ", "))
	}
	return nil
}

// execute parses and runs the template, returning the rendered string.
func (r *Renderer) execute() (string, error) {
	t, err := template.New("renderer").Delims(r.lDelim, r.rDelim).Parse(r.tmpl)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	if err = t.Execute(&sb, r.vars); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// Render is a one-shot helper equivalent to
// NewRenderer().WithTemplate(tmpl).WithVariables(data).Render().
//
// Example:
//
//	out, err := text.Render("Hello {{.Name}}!", map[string]any{"Name": "world"})
func Render(tmpl string, data map[string]any) (string, error) {
	return NewRenderer().WithTemplate(tmpl).WithVariables(data).Render()
}

// MustRender is the panicking variant of [Render].
func MustRender(tmpl string, data map[string]any) string {
	return NewRenderer().WithTemplate(tmpl).WithVariables(data).MustRender()
}
