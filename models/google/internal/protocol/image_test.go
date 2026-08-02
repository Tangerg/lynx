package google_test

import (
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/models/google/internal/protocol"
	"github.com/Tangerg/lynx/models/google/internal/testutil"
)

func TestImageModelCallUsesInteractionsAPI(t *testing.T) {
	first := base64.StdEncoding.EncodeToString([]byte("FIRST"))
	second := base64.StdEncoding.EncodeToString([]byte("SECOND"))
	body := `{
		"id":"int_123",
		"model":"gemini-3.1-flash-image",
		"status":"completed",
		"object":"interaction",
		"created":"2026-07-31T10:20:30Z",
		"steps":[
			{"type":"thought","summary":[{"type":"image","data":"` + first + `","mime_type":"image/png"}]},
			{"type":"model_output","content":[
				{"type":"text","text":"done"},
				{"type":"image","data":"` + first + `","mime_type":"image/png"},
				{"type":"image","data":"` + second + `","mime_type":"image/jpeg"}
			]}
		]
	}`
	srv := testutil.JSONServer(http.StatusOK, body)
	t.Cleanup(srv.Close)

	opts, err := image.NewOptions(google.ModelGemini31FlashImage)
	if err != nil {
		t.Fatal(err)
	}
	opts.OutputFormat = "image/jpeg"
	if err := opts.SetExtension(google.ImageRequestExtensionKey, google.ImageGenerationOptions{
		AspectRatio: "16:9",
		ImageSize:   "2K",
	}); err != nil {
		t.Fatal(err)
	}
	model, err := google.NewImageModel(google.ImageModelConfig{
		APIKey:         "test-key",
		DefaultOptions: opts,
		BaseURL:        srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}

	req, _ := image.NewRequest("a serene mountain lake")
	out, err := model.Call(t.Context(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if len(out.Results) != 2 || out.First() == nil {
		t.Fatalf("results = %#v", out.Results)
	}
	if out.Results[1].Media.MIME != "image/jpeg" {
		t.Fatalf("second MIME = %q", out.Results[1].Media.MIME)
	}
	if got := out.Metadata.Created; got != time.Date(2026, 7, 31, 10, 20, 30, 0, time.UTC).Unix() {
		t.Fatalf("Created = %d", got)
	}
	if _, ok, err := metadata.Decode[map[string]any](out.Metadata.Extra, google.ImageResponseExtensionKey); err != nil || !ok {
		t.Fatalf("native response extension: exists=%t err=%v", ok, err)
	}

	hugeSeed := int64(1 << 31)
	req.Options = image.Options{Seed: &hugeSeed}
	if _, err := model.Call(t.Context(), req); err == nil {
		t.Fatal("Call accepted a seed that overflows the provider wire type")
	}
}

func TestImageModelRejectsUnsupportedImagenOnlyOptions(t *testing.T) {
	opts, _ := image.NewOptions(google.ModelGemini31FlashImage)
	model, err := google.NewImageModel(google.ImageModelConfig{
		APIKey:         "test-key",
		DefaultOptions: opts,
		BaseURL:        "https://example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := image.NewRequest("a lynx")
	req.Options.NegativePrompt = "blurry"
	if _, err := model.Call(t.Context(), req); err == nil {
		t.Fatal("Call accepted Imagen-only negative_prompt")
	}
}
