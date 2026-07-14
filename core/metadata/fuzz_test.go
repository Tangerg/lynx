package metadata_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Tangerg/lynx/core/metadata"
)

func FuzzMapJSON(f *testing.F) {
	for _, seed := range []string{
		`null`,
		`{}`,
		`{"text":"hello","count":2,"nested":{"ok":true}}`,
		`{"bad":`,
		`[]`,
	} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var first metadata.Map
		if err := json.Unmarshal(data, &first); err != nil {
			return
		}
		if err := first.Validate(); err != nil {
			t.Fatalf("successful Unmarshal produced invalid Map: %v", err)
		}
		firstWire, err := json.Marshal(first)
		if err != nil {
			t.Fatalf("Marshal after successful Unmarshal: %v", err)
		}

		var second metadata.Map
		if err := json.Unmarshal(firstWire, &second); err != nil {
			t.Fatalf("Unmarshal canonical wire: %v", err)
		}
		secondWire, err := json.Marshal(second)
		if err != nil {
			t.Fatalf("Marshal second value: %v", err)
		}
		if !bytes.Equal(firstWire, secondWire) {
			t.Fatalf("wire did not reach fixed point: first=%s second=%s", firstWire, secondWire)
		}
	})
}
