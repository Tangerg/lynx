package prompt

import (
	"testing"
)

func TestNewTemplate(t *testing.T) {
	template := NewTemplate()
	err := template.Execute(`Hello, {{.Name}}! Welcome to {{.Place}}.`, map[string]any{
		"Name":  "Alice",
		"Place": "Wonderland",
	})
	if err != nil {
		t.Log(err)
	}
	render := template.Render()
	t.Log(render)
}
