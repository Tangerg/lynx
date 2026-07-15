package midjourney_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/midjourney"
)

func TestImageModel_Call_Mock(t *testing.T) {
	var polls testutil.PollCounter

	srv := testutil.MuxServer(
		testutil.Route{Method: "POST", Contains: "/imagine", Handle: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"task_id":"mj-1"}`))
		}},
		testutil.Route{Method: "GET", Contains: "/fetch/", Handle: func(w http.ResponseWriter, r *http.Request) {
			n := polls.Inc()
			status := "PENDING"
			img := ""
			if n >= 2 {
				status = "SUCCESS"
				img = "https://cdn.test/mj.png"
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"` + status + `","image_url":"` + img + `","progress":"100%"}`))
		}},
	)
	t.Cleanup(srv.Close)

	opts, err := image.NewOptions("midjourney-6")
	if err != nil {
		t.Fatal(err)
	}
	m, err := midjourney.NewImageModel(midjourney.ImageModelConfig{
		APIKey:         "test-key",
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
	if out.First() == nil {
		t.Fatal("nil result")
	}
}
