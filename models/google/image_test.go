package google_test

import (
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/image"
	"github.com/Tangerg/lynx/models/google"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

func TestImageModel_Call_Mock(t *testing.T) {
	// genai SDK maps wire-format `predictions` → typed
	// `generatedImages`. Each prediction entry exposes
	// `bytesBase64Encoded` + `mimeType` (the Mldev shape).
	imgB64 := base64.StdEncoding.EncodeToString([]byte("FAKE-PNG-BYTES"))
	body := `{"predictions":[{"bytesBase64Encoded":"` + imgB64 + `","mimeType":"image/png"}]}`
	srv := testutil.JSONServer(http.StatusOK, body)
	t.Cleanup(srv.Close)

	opts, err := image.NewOptions("imagen-3.0-generate-002")
	if err != nil {
		t.Fatal(err)
	}
	m, err := google.NewImageModel(&google.ImageModelConfig{
		ApiKey:         model.NewApiKey("test-key"),
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
	if out.Result == nil {
		t.Fatal("nil result")
	}
	if m.Metadata().Provider != google.Provider {
		t.Errorf("provider = %q", m.Metadata().Provider)
	}
}
