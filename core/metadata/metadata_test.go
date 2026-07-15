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
	if err := m.Set("value", want); err != nil {
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
		{name: "empty key", m: metadata.New(), value: 1, want: metadata.ErrEmptyKey},
		{name: "unsupported value", m: metadata.New(), key: "key", value: func() {}, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.m.Set(tt.key, tt.value)
			if err == nil {
				t.Fatal("Set returned nil error")
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("Set error = %v, want errors.Is(%v)", err, tt.want)
			}
		})
	}

	t.Run("nil map initializes in place", func(t *testing.T) {
		var m metadata.Map
		if err := m.Set("key", 1); err != nil {
			t.Fatalf("Set on nil map: %v", err)
		}
		value, ok, err := metadata.Decode[int](m, "key")
		if err != nil || !ok || value != 1 {
			t.Fatalf("Decode after lazy init = (%v, %v, %v), want (1, true, nil)", value, ok, err)
		}
	})

	t.Run("nil pointer receiver", func(t *testing.T) {
		var m *metadata.Map
		if err := m.Set("key", 1); !errors.Is(err, metadata.ErrNilMap) {
			t.Fatalf("Set on nil *Map = %v, want ErrNilMap", err)
		}
	})
}

func TestMerge(t *testing.T) {
	source := metadata.Map{
		"shared": json.RawMessage(`{"from":"source"}`),
		"added":  json.RawMessage(`true`),
	}

	var zero metadata.Map
	if err := zero.Merge(source); err != nil {
		t.Fatalf("Merge into zero value: %v", err)
	}
	if len(zero) != len(source) {
		t.Fatalf("zero-value Merge length = %d, want %d", len(zero), len(source))
	}

	var empty metadata.Map
	if err := empty.Merge(nil); err != nil {
		t.Fatalf("Merge nil source: %v", err)
	}
	if empty != nil {
		t.Fatalf("Merge nil source initialized target: %#v", empty)
	}

	var target metadata.Map
	if err := target.Set("shared", map[string]string{"from": "target"}); err != nil {
		t.Fatal(err)
	}

	if err := target.Merge(source); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if got := string(target["shared"]); got != `{"from":"source"}` {
		t.Fatalf("shared = %s", got)
	}
	if got := string(target["added"]); got != `true` {
		t.Fatalf("added = %s", got)
	}

	source["shared"][0] = '['
	if got := string(target["shared"]); got != `{"from":"source"}` {
		t.Fatalf("Merge aliased source: %s", got)
	}
}

func TestMergeRejectsInvalidMapsAtomically(t *testing.T) {
	tests := []struct {
		name   string
		target metadata.Map
		source metadata.Map
	}{
		{
			name:   "invalid target",
			target: metadata.Map{"existing": json.RawMessage(`{`)},
			source: metadata.Map{"added": json.RawMessage(`true`)},
		},
		{
			name:   "invalid source",
			target: metadata.Map{"existing": json.RawMessage(`true`)},
			source: metadata.Map{"added": json.RawMessage(`{`)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := tt.target.Clone()
			if err := tt.target.Merge(tt.source); !errors.Is(err, metadata.ErrInvalidValue) {
				t.Fatalf("Merge error = %v, want ErrInvalidValue", err)
			}
			if !reflect.DeepEqual(tt.target, before) {
				t.Fatalf("failed Merge mutated target: got %#v, want %#v", tt.target, before)
			}
		})
	}

	var target *metadata.Map
	if err := target.Merge(nil); !errors.Is(err, metadata.ErrNilMap) {
		t.Fatalf("Merge on nil *Map = %v, want ErrNilMap", err)
	}
}

func TestDecodeMissingAndTypeMismatch(t *testing.T) {
	if got, ok, err := metadata.Decode[string](nil, "missing"); err != nil || ok || got != "" {
		t.Fatalf("missing Decode = (%q, %v, %v)", got, ok, err)
	}

	m := metadata.New()
	if err := m.Set("count", 3); err != nil {
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
	if err := src.Set("name", "lynx"); err != nil {
		t.Fatal(err)
	}
	if err := src.Set("nested", map[string]any{"enabled": true}); err != nil {
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

func TestValueMapBoundary(t *testing.T) {
	source := map[string]any{"name": "lynx", "count": 2.0, "nested": map[string]any{"ok": true}}
	encoded, err := metadata.FromValues(source)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := encoded.Values()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, source) {
		t.Fatalf("Values = %#v, want %#v", decoded, source)
	}
	if _, err := metadata.FromValues(map[string]any{"bad": func() {}}); err == nil {
		t.Fatal("FromValues accepted runtime behavior")
	}
}
