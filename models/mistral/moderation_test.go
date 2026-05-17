package mistral_test

import (
	"net/http"
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/moderation"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/mistral"
)

const mistralModerationJSON = `{
  "id": "mod-test",
  "model": "mistral-moderation-latest",
  "results": [{
    "categories": {"sexual": false, "violence_and_threats": false},
    "category_scores": {"sexual": 0.01, "violence_and_threats": 0.02}
  }]
}`

func TestModerationModel_Call_Mock(t *testing.T) {
	srv := testutil.JSONServer(http.StatusOK, mistralModerationJSON)
	t.Cleanup(srv.Close)

	opts, err := moderation.NewOptions("mistral-moderation-latest")
	if err != nil {
		t.Fatal(err)
	}
	m, err := mistral.NewModerationModel(&mistral.ModerationModelConfig{
		ApiKey:         model.NewApiKey("test-key"),
		DefaultOptions: opts,
		BaseURL:        srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}

	req, _ := moderation.NewRequest([]string{"hello world"})
	out, err := m.Call(t.Context(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if len(out.Results) == 0 {
		t.Fatal("empty results")
	}
	if m.Metadata().Provider != mistral.Provider {
		t.Errorf("provider = %q", m.Metadata().Provider)
	}
}
