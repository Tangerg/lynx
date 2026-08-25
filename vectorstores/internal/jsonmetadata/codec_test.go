package jsonmetadata

import (
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/core/metadata"
)

func TestCodecUsesJSONObjectForNilMetadata(t *testing.T) {
	raw, err := (Codec{}).Marshal(nil)
	if err != nil {
		t.Fatalf("Marshal(nil) error = %v", err)
	}
	if string(raw) != "{}" {
		t.Fatalf("Marshal(nil) = %q, want {}", raw)
	}
}

func TestCodecRoundTrip(t *testing.T) {
	want, err := metadata.FromValues(map[string]any{"priority": int64(3)})
	if err != nil {
		t.Fatalf("FromValues() error = %v", err)
	}
	codec := Codec{}
	raw, err := codec.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	got, err := codec.Unmarshal(raw)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %#v, want %#v", got, want)
	}
}

func TestCodecTreatsAbsentValueAsNil(t *testing.T) {
	got, err := (Codec{}).Unmarshal(nil)
	if err != nil {
		t.Fatalf("Unmarshal(nil) error = %v", err)
	}
	if got != nil {
		t.Fatalf("Unmarshal(nil) = %#v, want nil", got)
	}
}
