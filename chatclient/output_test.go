package chatclient

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type recipe struct {
	Name  string   `json:"name"`
	Steps []string `json:"steps"`
}

func TestOutputValidate(t *testing.T) {
	tests := []struct {
		name   string
		output Output[string]
		valid  bool
	}{
		{name: "zero"},
		{name: "whitespace instructions", output: Output[string]{Instructions: " \n", Decode: func(value string) (string, error) { return value, nil }}},
		{name: "decode only", output: Output[string]{Decode: func(value string) (string, error) { return value, nil }}, valid: true},
		{name: "instructions and decode", output: Output[string]{Instructions: "plain text", Decode: func(value string) (string, error) { return value, nil }}, valid: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.output.Validate()
			if test.valid && err != nil {
				t.Fatal(err)
			}
			if !test.valid && !errors.Is(err, ErrInvalidOutput) {
				t.Fatalf("Validate error = %v, want ErrInvalidOutput", err)
			}
		})
	}
}

func TestJSONDecodesPlainAndMarkdownFencedValues(t *testing.T) {
	output := JSON[recipe]()
	inputs := []string{
		`{"name":"tea","steps":["steep"]}`,
		"```json\n{\"name\":\"tea\",\"steps\":[\"steep\"]}\n```",
		"```\n{\n\"name\": \"tea\",\n\"steps\": [\"steep\"]\n}\n```",
	}
	for _, input := range inputs {
		got, err := output.Decode(input)
		if err != nil {
			t.Fatalf("Decode(%q): %v", input, err)
		}
		want := recipe{Name: "tea", Steps: []string{"steep"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Decode(%q) = %#v, want %#v", input, got, want)
		}
	}
	if !strings.Contains(output.Instructions, "RFC 8259") {
		t.Fatalf("JSON instructions = %q", output.Instructions)
	}
}

func TestJSONPreservesEncodingJSONErrorIdentity(t *testing.T) {
	_, err := JSON[recipe]().Decode(`{"name":`)
	var syntaxError *json.SyntaxError
	if !errors.As(err, &syntaxError) {
		t.Fatalf("Decode error = %v, want *json.SyntaxError", err)
	}
	if _, err := JSON[recipe]().Decode("```yaml\nname: tea\n```"); err == nil {
		t.Fatal("wrong-language fence unexpectedly decoded")
	}
}

func TestJSONSchemaValidatesAndSnapshotsSchema(t *testing.T) {
	for _, schema := range []json.RawMessage{nil, []byte(`[]`), []byte(`null`), []byte(`{"type":`)} {
		output, err := JSONSchema[recipe](schema)
		if err == nil || output.Decode != nil || !errors.Is(err, ErrInvalidOutput) {
			t.Fatalf("JSONSchema(%s) = (%#v, %v), want invalid output", schema, output, err)
		}
	}

	schema := json.RawMessage(`{ "type": "object", "required": ["name"] }`)
	output, err := JSONSchema[recipe](schema)
	if err != nil {
		t.Fatal(err)
	}
	schema[2] = 'X'
	if !strings.Contains(output.Instructions, `{"type":"object","required":["name"]}`) {
		t.Fatalf("schema instructions = %q", output.Instructions)
	}
	got, err := output.Decode(`{"name":"tea","steps":[]}`)
	if err != nil || got.Name != "tea" {
		t.Fatalf("Decode = (%#v, %v)", got, err)
	}
}

func TestCommaSeparated(t *testing.T) {
	output := CommaSeparated()
	got, err := output.Decode(" apple, banana ,cherry ")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"apple", "banana", "cherry"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Decode = %v, want %v", got, want)
	}
	empty, err := output.Decode(" \n")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("blank Decode = (%#v, %v), want non-nil empty", empty, err)
	}
}
