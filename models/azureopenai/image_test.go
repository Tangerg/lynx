package azureopenai_test

import (
	"net/http"
	"testing"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/models/azureopenai"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

const azureImageJSON = `{"created":1700000000,"data":[{"url":"https://cdn.test/img.png"}]}`

func TestImageModel_Call_Mock(t *testing.T) {
	srv := testutil.JSONServer(http.StatusOK, azureImageJSON)
	t.Cleanup(srv.Close)

	opts, err := image.NewOptions("dall-e-3-deployment")
	if err != nil {
		t.Fatal(err)
	}
	m, err := azureopenai.NewImageModel(azureopenai.ImageModelConfig{
		APIKey:         "test-key",
		Endpoint:       srv.URL,
		DefaultOptions: opts,
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
