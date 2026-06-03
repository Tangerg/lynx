package stability_test

import (
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/image"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/stability"
)

func TestImageModel_Call_Mock(t *testing.T) {
	imgB64 := base64.StdEncoding.EncodeToString([]byte("FAKE-PNG-BYTES"))
	body := `{"image":"` + imgB64 + `","finish_reason":"SUCCESS","seed":42}`
	srv := testutil.JSONServer(http.StatusOK, body)
	t.Cleanup(srv.Close)

	opts, err := image.NewOptions("stable-image-core")
	if err != nil {
		t.Fatal(err)
	}
	m, err := stability.NewImageModel(stability.ImageModelConfig{
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
	if m.Metadata().Provider != stability.Provider {
		t.Errorf("provider = %q", m.Metadata().Provider)
	}
}
