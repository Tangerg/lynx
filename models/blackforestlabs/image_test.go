package blackforestlabs_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/image"
	"github.com/Tangerg/lynx/models/blackforestlabs"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

func TestImageModel_Call_Mock(t *testing.T) {
	var polls testutil.PollCounter

	srv := testutil.MuxServer(
		// POST /v1/<model> returns the async id
		testutil.Route{Method: "POST", Contains: "flux", Handle: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":"task-1","polling_url":"/v1/get_result?id=task-1"}`))
		}},
		// GET /v1/get_result?id=... polls until Ready
		testutil.Route{Method: "GET", Contains: "/get_result", Handle: func(w http.ResponseWriter, r *http.Request) {
			n := polls.Inc()
			status := "Pending"
			sample := ""
			if n >= 2 {
				status = "Ready"
				sample = "https://example.com/img.png"
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":"task-1","status":"` + status + `","result":{"sample":"` + sample + `","seed":42,"duration":100}}`))
		}},
	)
	t.Cleanup(srv.Close)

	opts, err := image.NewOptions("flux-pro-1.1")
	if err != nil {
		t.Fatal(err)
	}
	m, err := blackforestlabs.NewImageModel(&blackforestlabs.ImageModelConfig{
		APIKey:         model.NewAPIKey("test-key"),
		DefaultOptions: opts,
		BaseURL:        srv.URL,
		PollInterval:   10 * time.Millisecond,
		PollTimeout:    5 * time.Second,
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
