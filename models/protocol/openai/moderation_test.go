package openai_test

import (
	"net/http"
	"testing"

	"github.com/Tangerg/lynx/core/modeltest"
	"github.com/Tangerg/lynx/core/moderation"
	"github.com/Tangerg/lynx/models/protocol/openai"
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
	srv := modeltest.JSONServer(http.StatusOK, modResponseJSON)
	t.Cleanup(srv.Close)

	opts, err := moderation.NewOptions("omni-moderation-latest")
	if err != nil {
		t.Fatal(err)
	}
	m, err := openai.NewModerationModel(openai.ModerationModelConfig{
		Provider:       "openai",
		APIKey:         "test-key",
		DefaultOptions: opts,
		BaseURL:        srv.URL,
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
