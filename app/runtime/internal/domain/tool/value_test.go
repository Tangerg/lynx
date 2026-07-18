package tool

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"
)

func TestArgumentsCanonicalizeAndOwnValues(t *testing.T) {
	nested := map[string]any{"enabled": true}
	source := map[string]any{"z": nested, "a": float64(1)}
	arguments, err := ArgumentsFromMap(source)
	if err != nil {
		t.Fatalf("ArgumentsFromMap: %v", err)
	}
	if got, want := arguments.Canonical(), `{"a":1,"z":{"enabled":true}}`; got != want {
		t.Fatalf("canonical arguments = %s, want %s", got, want)
	}

	nested["enabled"] = false
	source["new"] = "retained by caller"
	projection := arguments.Map()
	if !reflect.DeepEqual(projection, map[string]any{
		"a": json.Number("1"), "z": map[string]any{"enabled": true},
	}) {
		t.Fatalf("arguments changed through source ownership: %#v", projection)
	}
	projection["a"] = json.Number("2")
	projection["z"].(map[string]any)["enabled"] = false
	if got := arguments.Canonical(); got != `{"a":1,"z":{"enabled":true}}` {
		t.Fatalf("arguments changed through projection ownership: %s", got)
	}
}

func TestArgumentsAcceptEmptyAndRejectNonObjects(t *testing.T) {
	for _, raw := range []string{"", " \n\t", "{}"} {
		arguments, err := ParseArguments(raw)
		if err != nil || arguments.Canonical() != "{}" {
			t.Errorf("ParseArguments(%q) = (%s, %v), want empty object", raw, arguments.Canonical(), err)
		}
	}
	for _, raw := range []string{"null", "[]", `"text"`, "1", "true", "{", "{} {}"} {
		if _, err := ParseArguments(raw); !errors.Is(err, ErrInvalidArguments) {
			t.Errorf("ParseArguments(%q) error = %v, want ErrInvalidArguments", raw, err)
		}
	}
	if _, err := ArgumentsFromMap(nil); !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("ArgumentsFromMap(nil) error = %v, want ErrInvalidArguments", err)
	}
	if _, err := ArgumentsFromMap(map[string]any{"bad": math.Inf(1)}); !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("non-JSON argument error = %v, want ErrInvalidArguments", err)
	}
}

func TestArgumentsJSONRoundTrip(t *testing.T) {
	original, err := ParseArguments(`{"query":"lynx","limit":3}`)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Arguments
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Canonical() != original.Canonical() {
		t.Fatalf("round trip = %s, want %s", decoded.Canonical(), original.Canonical())
	}
}

func TestArgumentsStringField(t *testing.T) {
	arguments, err := ParseArguments(`{"command":"go test","timeout":30}`)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := arguments.StringField("command"); !ok || got != "go test" {
		t.Fatalf("command = (%q, %v), want go test/true", got, ok)
	}
	if _, ok := arguments.StringField("timeout"); ok {
		t.Fatal("numeric timeout reported as a string")
	}
	if _, ok := arguments.StringField("missing"); ok {
		t.Fatal("missing field reported as present")
	}
}

func TestArgumentsPreserveLargeNumbers(t *testing.T) {
	arguments, err := ParseArguments(`{"id":9007199254740993}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := arguments.Canonical(); got != `{"id":9007199254740993}` {
		t.Fatalf("large argument = %s, want exact integer", got)
	}
	if got := arguments.Map()["id"]; got != json.Number("9007199254740993") {
		t.Fatalf("large argument projection = %#v", got)
	}
}

func TestResultOwnsValuesAndDistinguishesText(t *testing.T) {
	nested := map[string]any{"items": []any{"first"}}
	result, err := NewResult(nested)
	if err != nil {
		t.Fatalf("NewResult: %v", err)
	}
	nested["items"].([]any)[0] = "mutated"
	if got := result.Any().(map[string]any)["items"].([]any)[0]; got != "first" {
		t.Fatalf("result changed through source ownership: %#v", got)
	}
	projection := result.Any().(map[string]any)
	projection["items"].([]any)[0] = "mutated again"
	if got := result.Any().(map[string]any)["items"].([]any)[0]; got != "first" {
		t.Fatalf("result changed through projection ownership: %#v", got)
	}
	if _, ok := result.String(); ok {
		t.Fatal("object result reported itself as text")
	}

	text := StringResult("done")
	if got, ok := text.String(); !ok || got != "done" {
		t.Fatalf("text result = (%q, %v), want done/true", got, ok)
	}
}

func TestResultZeroValueIsPresentNull(t *testing.T) {
	var result Result
	if got := result.Any(); got != nil {
		t.Fatalf("zero result projection = %#v, want nil", got)
	}
	encoded, err := json.Marshal(result)
	if err != nil || string(encoded) != "null" {
		t.Fatalf("zero result encoding = %s, %v; want null", encoded, err)
	}
	parsed, err := ParseResult([]byte("null"))
	if err != nil || parsed.Any() != nil {
		t.Fatalf("parsed null = (%#v, %v), want present null", parsed.Any(), err)
	}
}

func TestResultRejectsValuesOutsideProtocol(t *testing.T) {
	for _, raw := range []string{"", " ", "{"} {
		if _, err := ParseResult([]byte(raw)); !errors.Is(err, ErrInvalidResult) {
			t.Errorf("ParseResult(%q) error = %v, want ErrInvalidResult", raw, err)
		}
	}
	if _, err := NewResult(func() {}); !errors.Is(err, ErrInvalidResult) {
		t.Fatalf("function result error = %v, want ErrInvalidResult", err)
	}
}
