package protocol

import (
	"encoding/json"
	"testing"

	"google.golang.org/genai"

	corechat "github.com/Tangerg/scope/core/chat"
)

func TestMapProtocolOutputFormat(t *testing.T) {
	format, err := corechat.NewJSONSchemaOutputFormat(corechat.JSONSchemaConfig{
		Name: "answer", Schema: json.RawMessage(`{"type":"object"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	config := &genai.GenerateContentConfig{}
	if mapProtocolOutputFormatErr := mapProtocolOutputFormat(&format, config); mapProtocolOutputFormatErr != nil {
		t.Fatal(mapProtocolOutputFormatErr)
	}
	if config.ResponseMIMEType != "application/json" || config.ResponseJsonSchema == nil {
		t.Fatalf("schema config = %#v", config)
	}
	text, err := corechat.NewOutputFormat(corechat.OutputFormatText)
	if err != nil {
		t.Fatal(err)
	}
	if err := mapProtocolOutputFormat(&text, config); err != nil {
		t.Fatal(err)
	}
	if config.ResponseMIMEType != "text/plain" || config.ResponseJsonSchema != nil {
		t.Fatalf("text config = %#v", config)
	}
}
