package extension

import (
	"testing"

	"github.com/Tangerg/lynx/core/metadata"
)

func TestSetAndValidate(t *testing.T) {
	var values metadata.Map
	if err := Set(&values, "provider/options", map[string]any{"enabled": true}); err != nil {
		t.Fatal(err)
	}
	if err := Validate(values); err != nil {
		t.Fatal(err)
	}

	before := values.Clone()
	if err := Set(&values, "invalid", true); err == nil {
		t.Fatal("Set accepted an unnamespaced key")
	}
	if len(values) != len(before) {
		t.Fatal("failed Set mutated the map")
	}
	if err := Validate(metadata.Map{"invalid": []byte("true")}); err == nil {
		t.Fatal("Validate accepted an unnamespaced key")
	}
}
