package prodia_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/prodia"
)

func TestImageModel_Call_Mock(t *testing.T) {
	// Prodia /job returns the raw image bytes directly (sync endpoint).
	srv := testutil.BinaryServer(200, "image/jpeg", []byte("FAKE-JPEG-BYTES"))
	t.Cleanup(srv.Close)

	opts, err := image.NewOptions("inference.flux.schnell.txt2img.v1")
	if err != nil {
		t.Fatal(err)
	}
	m, err := prodia.NewImageModel(prodia.ImageModelConfig{
		APIKey:         model.NewAPIKey("test-key"),
		DefaultOptions: opts,
		BaseURL:        srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}

	req, _ := image.NewRequest("a small red square")
	out, err := m.Call(t.Context(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if out.Result == nil {
		t.Fatal("nil result")
	}
}
