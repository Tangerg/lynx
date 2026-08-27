package jsonschema_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/jsonschema"
	"github.com/Tangerg/lynx/core/speech"
)

type wireFixture struct {
	Metadata  map[string]json.RawMessage `json:"metadata"`
	Signature []byte                     `json:"signature"`
}

func TestForMatchesEncodingJSONAndOwnsDocument(t *testing.T) {
	schema, err := jsonschema.For[wireFixture]()
	if err != nil {
		t.Fatal(err)
	}
	valid := []byte(`{"metadata":{"object":{"id":1},"boolean":true},"signature":"AAEC/w=="}`)
	if err := schema.Validate(valid); err != nil {
		t.Fatalf("Validate(valid) error = %v; schema = %s", err, schema.JSON())
	}
	if err := schema.Validate([]byte(`{"metadata":{},"signature":[1,2]}`)); err == nil {
		t.Fatal("Validate(array-encoded bytes) succeeded")
	}
	if err := schema.Validate([]byte(`{"metadata":{},"signature":null}`)); err != nil {
		t.Fatalf("Validate(nil byte slice) error = %v", err)
	}
	first := schema.JSON()
	first[0] = '['
	if second := schema.JSON(); second[0] != '{' {
		t.Fatal("JSON returned aliased bytes")
	}
	if !bytes.Contains(schema.JSON(), []byte(`"contentEncoding":"base64"`)) {
		t.Fatalf("byte slice schema = %s", schema.JSON())
	}
}

func TestForQualifiesSameNamedTypesFromDifferentPackages(t *testing.T) {
	type response struct {
		Chat   chat.Output   `json:"chat"`
		Speech speech.Output `json:"speech"`
	}
	schema, err := jsonschema.For[response]()
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Properties map[string]struct {
			Ref string `json:"$ref"`
		} `json:"properties"`
		Definitions map[string]json.RawMessage `json:"$defs"`
	}
	if err := json.Unmarshal(schema.JSON(), &document); err != nil {
		t.Fatal(err)
	}
	chatRef := document.Properties["chat"].Ref
	speechRef := document.Properties["speech"].Ref
	if chatRef == "" || speechRef == "" || chatRef == speechRef {
		t.Fatalf("same-named types have ambiguous refs: chat=%q speech=%q", chatRef, speechRef)
	}
	for _, ref := range []string{chatRef, speechRef} {
		name := strings.TrimPrefix(ref, "#/$defs/")
		if _, exists := document.Definitions[name]; !exists {
			t.Fatalf("ref %q has no matching definition", ref)
		}
	}
}

func TestParseRejectsMalformedAndUnresolvedSchemas(t *testing.T) {
	for _, raw := range [][]byte{
		nil,
		[]byte(`[]`),
		[]byte(`{"type":"not-a-type"}`),
		[]byte(`{"minLength":-1}`),
		[]byte(`{"$ref":"https://example.com/external-schema"}`),
	} {
		if _, err := jsonschema.Parse(raw); !errors.Is(err, jsonschema.ErrInvalid) {
			t.Fatalf("Parse(%q) error = %v, want ErrInvalid", raw, err)
		}
	}
}
