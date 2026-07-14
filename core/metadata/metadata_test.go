package metadata_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/core/metadata"
)

func TestSetAndDecode(t *testing.T) {
	type value struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	m := metadata.New()
	want := value{Name: "lynx", Count: 2}
	if err := metadata.Set(m, "value", want); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, ok, err := metadata.Decode[value](m, "value")
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !ok {
		t.Fatal("Decode reported missing key")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Decode = %#v, want %#v", got, want)
	}
}

func TestSetRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name  string
		m     metadata.Map
		key   string
		value any
		want  error
	}{
		{name: "nil map", key: "key", value: 1, want: metadata.ErrNilMap},
		{name: "empty key", m: metadata.New(), value: 1, want: metadata.ErrEmptyKey},
		{name: "unsupported value", m: metadata.New(), key: "key", value: func() {}, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := metadata.Set(tt.m, tt.key, tt.value)
			if err == nil {
				t.Fatal("Set returned nil error")
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("Set error = %v, want errors.Is(%v)", err, tt.want)
			}
		})
	}
}

func TestDecodeMissingAndTypeMismatch(t *testing.T) {
	if got, ok, err := metadata.Decode[string](nil, "missing"); err != nil || ok || got != "" {
		t.Fatalf("missing Decode = (%q, %v, %v)", got, ok, err)
	}

	m := metadata.New()
	if err := metadata.Set(m, "count", 3); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := metadata.Decode[string](m, "count"); err == nil || !ok {
		t.Fatalf("type mismatch Decode = (present %v, error %v)", ok, err)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name string
		m    metadata.Map
		want error
	}{
		{name: "nil"},
		{name: "valid nested", m: metadata.Map{"nested": json.RawMessage(`{"items":[1,true,null]}`)}},
		{name: "empty key", m: metadata.Map{"": json.RawMessage(`1`)}, want: metadata.ErrEmptyKey},
		{name: "invalid value", m: metadata.Map{"bad": json.RawMessage(`{"open":`)}, want: metadata.ErrInvalidValue},
		{name: "empty value", m: metadata.Map{"bad": nil}, want: metadata.ErrInvalidValue},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.m.Validate()
			if !errors.Is(err, tt.want) {
				t.Fatalf("Validate error = %v, want errors.Is(%v)", err, tt.want)
			}
		})
	}
}

func TestJSONRoundTrip(t *testing.T) {
	src := metadata.New()
	if err := metadata.Set(src, "name", "lynx"); err != nil {
		t.Fatal(err)
	}
	if err := metadata.Set(src, "nested", map[string]any{"enabled": true}); err != nil {
		t.Fatal(err)
	}

	encoded, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got metadata.Map
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(got, src) {
		t.Fatalf("round trip = %#v, want %#v", got, src)
	}
}

func TestMarshalRejectsInvalidRawMessage(t *testing.T) {
	m := metadata.Map{"bad": json.RawMessage(`{`)}
	if _, err := json.Marshal(m); !errors.Is(err, metadata.ErrInvalidValue) {
		t.Fatalf("Marshal error = %v, want ErrInvalidValue", err)
	}
}

func TestUnmarshalRejectsNonObject(t *testing.T) {
	var m metadata.Map
	if err := json.Unmarshal([]byte(`[]`), &m); err == nil {
		t.Fatal("Unmarshal accepted an array")
	}
}

func TestCloneDoesNotAliasValues(t *testing.T) {
	src := metadata.Map{"value": json.RawMessage(`{"n":1}`)}
	clone := src.Clone()
	clone["value"][0] = '['
	if string(src["value"]) != `{"n":1}` {
		t.Fatalf("Clone aliased source: %s", src["value"])
	}
	if metadata.Map(nil).Clone() != nil {
		t.Fatal("nil Clone must remain nil")
	}
}
