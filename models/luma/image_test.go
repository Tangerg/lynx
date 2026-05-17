package luma_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/image"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/luma"
)

func TestImageModel_Call_Mock(t *testing.T) {
	var polls testutil.PollCounter

	srv := testutil.MuxServer(
		testutil.Route{Method: "POST", Contains: "/generations/image", Handle: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":"gen-1","state":"queued"}`))
		}},
		testutil.Route{Method: "GET", Contains: "/generations/", Handle: func(w http.ResponseWriter, r *http.Request) {
			n := polls.Inc()
			state := "dreaming"
			img := ""
			if n >= 2 {
				state = "completed"
				img = "https://cdn.test/img.png"
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":"gen-1","state":"` + state + `","assets":{"image":"` + img + `"}}`))
		}},
	)
	t.Cleanup(srv.Close)

	opts, err := image.NewOptions("photon-1")
	if err != nil {
		t.Fatal(err)
	}
	m, err := luma.NewImageModel(&luma.ImageModelConfig{
		ApiKey:         model.NewApiKey("test-key"),
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
