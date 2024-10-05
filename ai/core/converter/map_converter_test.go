package converter

import (
	"testing"
)

func TestMapConverter_Convert(t *testing.T) {
	example := NewMapConverterWithExample(map[string]any{
		"name":  "the name of user",
		"email": "the email of user",
	})
	format := example.getFormat()
	t.Log(format)
}
