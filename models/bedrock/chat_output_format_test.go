package bedrock

import (
	"encoding/json"
	"testing"

	corechat "github.com/Tangerg/lynx/core/chat"
)

func TestMapOutputFormat(t *testing.T) {
	format, err := corechat.NewJSONSchemaOutputFormat("answer", json.RawMessage(`{"type":"object"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := mapOutputFormat(&format); got == nil || got.TextFormat == nil {
		t.Fatalf("mapOutputFormat(json_schema) = %#v", got)
	}
	jsonFormat, err := corechat.NewOutputFormat(corechat.OutputFormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	if got := mapOutputFormat(&jsonFormat); got != nil {
		t.Fatalf("mapOutputFormat(json) = %#v, want prompt fallback", got)
	}
}
