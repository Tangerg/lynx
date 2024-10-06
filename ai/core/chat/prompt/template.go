package prompt

import (
	"strings"
	"text/template"
)

// Template is a struct that encapsulates functionality for rendering text templates
// with dynamic content. It utilizes the Go `text/template` package to parse and execute
// templates, allowing for the substitution of placeholders with actual values.
//
// Fields:
//
// tp *template.Template
//   - A pointer to a `template.Template` instance, which represents the parsed template.
//   - This field is used to store and manage the template structure for rendering.
//
// sb *strings.Builder
//   - A pointer to a `strings.Builder` instance, which is used to efficiently build
//     and store the rendered template output as a string.
//
// Functions:
//
// NewTemplate() *Template
//   - Constructs and returns a new instance of the Template struct.
//   - Initializes the `tp` field with a new template named "prompt" and the `sb` field
//     with a new strings.Builder instance.
//
// Methods:
//
// Render() string
//   - Returns the current content of the strings.Builder as a string.
//   - This method provides access to the rendered output of the template after execution.
//
// Execute(content string, attr map[string]any) error
//   - Parses the provided template content and executes it with the given attributes.
//   - `content` is a string containing the template with placeholders to be replaced.
//   - `attr` is a map of key-value pairs used to substitute placeholders in the template.
//   - Returns an error if parsing or execution fails, otherwise updates the `tp` field
//     with the parsed template and appends the rendered output to the strings.Builder.
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
