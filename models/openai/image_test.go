package openai_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/openai"
)

func newImageModel(t *testing.T, baseURL, modelID string) *openai.ImageModel {
	t.Helper()
	opts, err := image.NewOptions(modelID)
	if err != nil {
		t.Fatalf("NewOptions: %v", err)
	}
	m, err := openai.NewImageModel(openai.ImageModelConfig{
		APIKey:         model.NewAPIKey("test-key"),
		DefaultOptions: opts,
		RequestOptions: []option.RequestOption{option.WithBaseURL(baseURL)},
	})
	if err != nil {
		t.Fatalf("NewImageModel: %v", err)
	}
	return m
}

func TestImageModel_Call_Mock(t *testing.T) {
	resp := openaisdk.ImagesResponse{
		Created: 1700000000,
		Data: []openaisdk.Image{
			{URL: "https://example.com/img1.png", B64JSON: ""},
		},
	}
	body, _ := json.Marshal(resp)

	var seenURL string
	srv := testutil.JSONServer(http.StatusOK, string(body), func(r *http.Request) {
		seenURL = r.URL.Path
	})
	t.Cleanup(srv.Close)

	m := newImageModel(t, srv.URL, "dall-e-3")
	req, err := image.NewRequest("a serene mountain lake at sunset")
	if err != nil {
		t.Fatal(err)
	}

	out, err := m.Call(t.Context(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.HasSuffix(seenURL, "/images/generations") {
		t.Errorf("URL = %q; want /images/generations suffix", seenURL)
	}
	if out.Result == nil {
		t.Fatal("nil result")
	}
}
