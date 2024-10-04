package prompt

import (
	"strings"
	"text/template"
)

type Template struct {
	tp *template.Template
	sb *strings.Builder
}

func NewTemplate() *Template {
	return &Template{
		tp: template.New("prompt"),
		sb: new(strings.Builder),
	}
}

func (t *Template) Render() string {
	return t.sb.String()
}

func (t *Template) Execute(content string, attr map[string]any) error {
	parse, err := t.tp.Parse(content)
	if err != nil {
		return err
	}
	t.tp = parse
	return t.tp.Execute(t.sb, attr)
}
