package mistral

import (
	"encoding/json"
	"testing"

	corechat "github.com/Tangerg/lynx/core/chat"
)

func TestMapMistralOutputFormat(t *testing.T) {
	format, err := corechat.NewJSONSchemaOutputFormat("answer", json.RawMessage(`{"type":"object"}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := mapMistralOutputFormat(&format)
	if err != nil {
		t.Fatal(err)
	}
	var mapped map[string]any
	if err := json.Unmarshal(raw, &mapped); err != nil {
		t.Fatal(err)
	}
	if mapped["type"] != "json_schema" {
		t.Fatalf("response format = %#v", mapped)
	}
	definition, ok := mapped["json_schema"].(map[string]any)
	if !ok || definition["name"] != "answer" || definition["strict"] != true {
		t.Fatalf("json_schema = %#v", mapped["json_schema"])
	}
}
