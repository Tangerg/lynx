package text

import (
	"strings"
	"text/template"
)

type Renderer struct {
	template       *template.Template
	templateString string
	variables      map[string]any
	leftDelim      string
	rightDelim     string
	sb             *strings.Builder
}

func NewRenderer() *Renderer {
	return &Renderer{
		template:   template.New("renderer"),
		leftDelim:  "{{",
		rightDelim: "}}",
		sb:         new(strings.Builder),
	}
}

func (r *Renderer) Template(template string) *Renderer {
	if template != "" {
		r.templateString = template
		r.sb.Reset()
	}
	return r
}

func (r *Renderer) Variables(variables map[string]any) *Renderer {
	if len(variables) > 0 {
		r.variables = variables
		r.sb.Reset()
	}
	return r
}

func (r *Renderer) Delims(left, right string) *Renderer {
	if left != "" {
		r.leftDelim = left
	}
	if right != "" {
		r.rightDelim = right
	}
	return r
}

func (r *Renderer) render() (string, error) {
	tmpl := r.template.Delims(r.leftDelim, r.rightDelim)

	tmpl, err := tmpl.Parse(r.templateString)
	if err != nil {
		return "", err
	}

	err = tmpl.Execute(r.sb, r.variables)
	if err != nil {
		return "", err
	}

	return r.sb.String(), nil
}
func (r *Renderer) Render() (string, error) {
	if r.templateString == "" {
		return r.templateString, nil
	}
	if len(r.variables) == 0 {
		return r.templateString, nil
	}
	if r.sb.Len() > 0 {
		return r.sb.String(), nil
	}
	return r.render()
}

func (r *Renderer) MustRender() string {
	result, err := r.Render()
	if err != nil {
		panic(err)
	}
	return result
}

func Render(tmplStr string, data map[string]any) (string, error) {
	return NewRenderer().Template(tmplStr).Variables(data).Render()
}

func MustRender(tmplStr string, data map[string]any) string {
	return NewRenderer().Template(tmplStr).Variables(data).MustRender()
}
