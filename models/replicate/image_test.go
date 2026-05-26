package replicate_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/image"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/replicate"
)

func TestImageModel_Call_Mock(t *testing.T) {
	var polls testutil.PollCounter

	srv := testutil.MuxServer(
		// POST /v1/models/<owner>/<name>/predictions OR /v1/predictions
		testutil.Route{Method: "POST", Contains: "/predictions", Handle: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":"pred-1","status":"starting","urls":{"get":"/v1/predictions/pred-1"}}`))
		}},
		// GET /v1/predictions/<id>
		testutil.Route{Method: "GET", Contains: "/predictions/", Handle: func(w http.ResponseWriter, r *http.Request) {
			n := polls.Inc()
			status := "processing"
			output := "null"
			if n >= 2 {
				status = "succeeded"
				output = `"https://cdn.test/img.png"`
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":"pred-1","status":"` + status + `","output":` + output + `}`))
		}},
	)
	t.Cleanup(srv.Close)

	opts, err := image.NewOptions("black-forest-labs/flux-schnell")
	if err != nil {
		t.Fatal(err)
	}
	m, err := replicate.NewImageModel(&replicate.ImageModelConfig{
		APIKey:         model.NewAPIKey("test-key"),
		DefaultOptions: opts,
		BaseURL:        srv.URL,
		PollInterval:   10 * time.Millisecond,
		PollTimeout:    5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	req, _ := image.NewRequest("a serene mountain lake")
	out, err := m.Call(t.Context(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if out.Result == nil {
		t.Fatal("nil result")
	}
}
