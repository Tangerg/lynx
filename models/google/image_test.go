package google_test

import (
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/models/google"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

func TestImageModel_Call_Mock(t *testing.T) {
	// genai SDK maps wire-format `predictions` → typed
	// `generatedImages`. Each prediction entry exposes
	// `bytesBase64Encoded` + `mimeType` (the Mldev shape).
	first := base64.StdEncoding.EncodeToString([]byte("FIRST"))
	second := base64.StdEncoding.EncodeToString([]byte("SECOND"))
	body := `{"predictions":[{"bytesBase64Encoded":"` + first + `","mimeType":"image/png"},{"bytesBase64Encoded":"` + second + `","mimeType":"image/jpeg"}]}`
	srv := testutil.JSONServer(http.StatusOK, body)
	t.Cleanup(srv.Close)

	opts, err := image.NewOptions("imagen-3.0-generate-002")
	if err != nil {
		t.Fatal(err)
	}
	m, err := google.NewImageModel(google.ImageModelConfig{
		APIKey:         "test-key",
		DefaultOptions: opts,
		BaseURL:        srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}

	req, _ := image.NewRequest("a serene mountain lake")
	out, err := m.Call(t.Context(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if len(out.Results) != 2 || out.First() == nil {
		t.Fatalf("results = %#v", out.Results)
	}
	if out.Results[1].Media.MIME != "image/jpeg" {
		t.Fatalf("second MIME = %q", out.Results[1].Media.MIME)
	}
	hugeSeed := int64(1 << 31)
	req.Options = image.Options{Seed: &hugeSeed}
	if _, err := m.Call(t.Context(), req); err == nil {
		t.Fatal("Call accepted a seed that overflows the provider wire type")
	}
}
