package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"

	corechat "github.com/Tangerg/lynx/core/chat"
)

func TestProtocolOutputFormatUsesNativeSchemaAndPromptFallback(t *testing.T) {
	schema, err := corechat.NewJSONSchemaOutputFormat("answer", json.RawMessage(`{"type":"object"}`))
	if err != nil {
		t.Fatal(err)
	}
	params := anthropicsdk.MessageNewParams{}
	if mapProtocolOutputFormatErr := mapProtocolOutputFormat(&schema, Dialect{NativeJSONSchema: true}, &params); mapProtocolOutputFormatErr != nil {
		t.Fatal(mapProtocolOutputFormatErr)
	}
	if len(params.OutputConfig.Format.Schema) == 0 || len(params.System) != 0 {
		t.Fatalf("native schema mapping = %#v", params)
	}

	jsonFormat, err := corechat.NewOutputFormat(corechat.OutputFormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	params = anthropicsdk.MessageNewParams{}
	if err := mapProtocolOutputFormat(&jsonFormat, Dialect{NativeJSONSchema: true}, &params); err != nil {
		t.Fatal(err)
	}
	if len(params.System) != 1 || !strings.Contains(params.System[0].Text, "valid JSON object") || len(params.OutputConfig.Format.Schema) != 0 {
		t.Fatalf("JSON fallback mapping = %#v", params)
	}

	params = anthropicsdk.MessageNewParams{}
	if err := mapProtocolOutputFormat(&schema, Dialect{}, &params); err != nil {
		t.Fatal(err)
	}
	if len(params.System) != 1 || !strings.Contains(params.System[0].Text, `{"type":"object"}`) || len(params.OutputConfig.Format.Schema) != 0 {
		t.Fatalf("schema fallback mapping = %#v", params)
	}
}

func TestOutputConfigExtensionKeepsNonFormatFields(t *testing.T) {
	fields := map[string]any{
		"output_config": map[string]any{
			"effort":       "high",
			"future_field": "preserved",
		},
	}
	config, err := decodeOutputConfig(fields, RequestExtensionKey)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got["effort"] != "high" || got["future_field"] != "preserved" {
		t.Fatalf("output_config = %s", encoded)
	}
	if _, exists := fields["output_config"]; exists {
		t.Fatal("decoded output_config remained in top-level extra fields")
	}

	for _, format := range []any{map[string]any{}, nil} {
		fields = map[string]any{"output_config": map[string]any{"format": format}}
		if _, err := decodeOutputConfig(fields, RequestExtensionKey); err == nil {
			t.Fatalf("format %#v unexpectedly accepted", format)
		}
	}
}
