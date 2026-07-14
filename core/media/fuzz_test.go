package media_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Tangerg/lynx/core/media"
)

func FuzzMediaJSON(f *testing.F) {
	for _, seed := range []string{
		`{"mime":"image/png","source":{"kind":"bytes","bytes":"AQID"}}`,
		`{"mime":"image/png","source":{"kind":"uri","uri":"https://example.com/image.png"}}`,
		`{"mime":"image/png","source":{"kind":"reference","ref":"file-1"}}`,
		`{"mime":"image/png","source":{"kind":"future","ref":"file-1"}}`,
		`{}`,
	} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var first media.Media
		if err := json.Unmarshal(data, &first); err != nil {
			return
		}
		if err := first.Validate(); err != nil {
			t.Fatalf("successful Unmarshal produced invalid Media: %v", err)
		}
		firstWire, err := json.Marshal(first)
		if err != nil {
			t.Fatalf("Marshal after successful Unmarshal: %v", err)
		}

		var second media.Media
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
