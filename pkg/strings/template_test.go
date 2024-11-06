package strings

import (
	"testing"
)

func TestNewTemplate(t *testing.T) {
	template := NewTextTemplate()
	err := template.ExecuteMap(`Hello, {{.Name}}! Welcome to {{.Place}}.`, map[string]any{
		"Name":  "Alice",
		"Place": "Wonderland",
	})
	if err != nil {
		t.Log(err)
	}
	render := template.Render()
	t.Log(render)
}
