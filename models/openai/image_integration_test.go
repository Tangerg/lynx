//go:build integration

package openai_test

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/image"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/openai"
)

func TestImageModel_Call_Integration(t *testing.T) {
	key := testutil.RequireKey(t, "openai")
	modelID, _ := testutil.LookupEnv("LYNX_TEST_OPENAI_IMAGE_MODEL")
	if modelID == "" {
		modelID = "dall-e-2" // cheaper than dall-e-3
	}
	opts, err := image.NewOptions(modelID)
	if err != nil {
		t.Fatal(err)
	}
	m, err := openai.NewImageModel(&openai.ImageModelConfig{
		APIKey:         model.NewAPIKey(key),
		DefaultOptions: opts,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := testutil.WithTimeout(t, 60*time.Second)
	defer cancel()
	req, _ := image.NewRequest("a small red square on white background")
	out, err := m.Call(ctx, req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if out.Result == nil {
		t.Fatal("nil result")
	}
}
