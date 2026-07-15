package media_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/core/media"
)

func TestMediaGolden(t *testing.T) {
	inline, err := media.NewBytes("image/png", []byte{1, 2, 3, 255})
	if err != nil {
		t.Fatal(err)
	}
	inline.ID = "image-1"
	inline.Name = "pixel.png"
	if err := inline.Metadata.Set("width", 1); err != nil {
		t.Fatal(err)
	}
	uri, err := media.NewURI("application/pdf", "https://example.com/manual.pdf")
	if err != nil {
		t.Fatal(err)
	}
	reference, err := media.NewReference("audio/mpeg", "provider-file-1")
	if err != nil {
		t.Fatal(err)
	}

	fixture := struct {
		Bytes     *media.Media `json:"bytes"`
		URI       *media.Media `json:"uri"`
		Reference *media.Media `json:"reference"`
	}{Bytes: inline, URI: uri, Reference: reference}
	assertMediaGolden(t, "media.golden.json", fixture)
}

func assertMediaGolden(t *testing.T, name string, value any) {
	t.Helper()
	got, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	want, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden mismatch\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}
