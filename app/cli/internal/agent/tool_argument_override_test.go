package agent

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestToolArgumentOverrideOwnsANormalizedJSONObject(t *testing.T) {
	t.Parallel()
	override, err := ParseToolArgumentOverride([]byte(`{
  "count": 9007199254740993,
  "nested": {"enabled": true}
}`))
	if err != nil {
		t.Fatal(err)
	}
	want := []byte(`{"count":9007199254740993,"nested":{"enabled":true}}`)
	if got := override.JSON(); !bytes.Equal(got, want) {
		t.Fatalf("normalized override = %s, want %s", got, want)
	}
	detached := override.JSON()
	detached[0] = '['
	if validateErr := override.Validate(); validateErr != nil {
		t.Fatalf("caller mutated the override through JSON(): %v", validateErr)
	}
	object, err := override.Object()
	if err != nil {
		t.Fatal(err)
	}
	if number, ok := object["count"].(json.Number); !ok || number.String() != "9007199254740993" {
		t.Fatalf("large JSON number = %#v", object["count"])
	}
	object["count"] = "mutated"
	if !override.Equal(override.Clone()) {
		t.Fatal("Object returned shared mutable state")
	}

	wire, err := json.Marshal(override)
	if err != nil {
		t.Fatal(err)
	}
	var restored ToolArgumentOverride
	if err := json.Unmarshal(wire, &restored); err != nil {
		t.Fatal(err)
	}
	if !override.Equal(&restored) {
		t.Fatalf("restored override = %s, want %s", restored.JSON(), override.JSON())
	}
}

func TestToolArgumentOverrideRejectsAmbiguousOrEmptyValues(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		value string
	}{
		{name: "empty object", value: `{}`},
		{name: "array", value: `[]`},
		{name: "null", value: `null`},
		{name: "trailing value", value: `{"one":1} {"two":2}`},
		{name: "duplicate root key", value: `{"path":"one","path":"two"}`},
		{name: "duplicate nested key", value: `{"options":{"mode":"one","mode":"two"}}`},
		{name: "malformed", value: `{"path":`},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseToolArgumentOverride([]byte(test.value)); err == nil {
				t.Fatalf("ParseToolArgumentOverride(%s) succeeded", test.value)
			}
		})
	}
}
