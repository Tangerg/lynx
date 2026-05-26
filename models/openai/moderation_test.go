package openai_test

import (
	"net/http"
	"testing"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/moderation"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/openai"
)

const modResponseJSON = `{
  "id": "mod-test",
  "model": "omni-moderation-latest",
  "results": [{
    "flagged": false,
    "categories": {"hate": false, "violence": false},
    "category_scores": {"hate": 0.01, "violence": 0.01}
  }]
}`

func TestModerationModel_Call_Mock(t *testing.T) {
	srv := testutil.JSONServer(http.StatusOK, modResponseJSON)
	t.Cleanup(srv.Close)

	opts, err := moderation.NewOptions("omni-moderation-latest")
	if err != nil {
		t.Fatal(err)
	}
	m, err := openai.NewModerationModel(&openai.ModerationModelConfig{
		APIKey:         model.NewAPIKey("test-key"),
		DefaultOptions: opts,
		RequestOptions: []option.RequestOption{option.WithBaseURL(srv.URL)},
	})
	if err != nil {
		t.Fatal(err)
	}

	req, err := moderation.NewRequest([]string{"hello world"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := m.Call(t.Context(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if len(out.Results) == 0 {
		t.Fatal("empty results")
	}
}

func TestModerationModel_Metadata(t *testing.T) {
	srv := testutil.JSONServer(http.StatusOK, "{}")
	t.Cleanup(srv.Close)
	opts, _ := moderation.NewOptions("omni-moderation-latest")
	m, _ := openai.NewModerationModel(&openai.ModerationModelConfig{
		APIKey:         model.NewAPIKey("test-key"),
		DefaultOptions: opts,
		RequestOptions: []option.RequestOption{option.WithBaseURL(srv.URL)},
	})
	if m.Metadata().Provider != openai.Provider {
		t.Errorf("provider = %q; want %q", m.Metadata().Provider, openai.Provider)
	}
}
