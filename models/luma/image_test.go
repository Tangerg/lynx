package luma_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Tangerg/scope/core/image"
	"github.com/Tangerg/scope/models/luma"
)

func TestImageModel_Call_Mock(t *testing.T) {
	var polls pollCounter

	var server *httptest.Server
	server = muxServer(
		route{Method: "POST", Contains: "/generations", Handle: func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer test-key" {
				t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"gen-1","created_at":"2026-07-31T08:00:00Z","model":"uni-1","state":"queued","type":"image","output":[]}`))
		}},
		route{Method: "GET", Contains: "/generations/", Handle: func(w http.ResponseWriter, r *http.Request) {
			n := polls.inc()
			state := "processing"
			output := "[]"
			if n >= 2 {
				state = "completed"
				output = fmt.Sprintf(`[{"type":"image","url":%q}]`, server.URL+"/output.png")
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":"gen-1","created_at":"2026-07-31T08:00:00Z","model":"uni-1","state":"` + state + `","type":"image","output":` + output + `}`))
		}},
		route{Method: "GET", Contains: "/output.png", Handle: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("PNG"))
		}},
	)
	t.Cleanup(server.Close)

	opts, err := image.NewOptions(luma.ModelUni1)
	if err != nil {
		t.Fatal(err)
	}
	m, err := luma.NewImageModel(luma.ImageModelConfig{
		APIKey:         "test-key",
		DefaultOptions: opts,
		BaseURL:        server.URL,
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
	if out.First() == nil || string(out.First().Media.Source.Bytes) != "PNG" {
		t.Fatalf("result = %#v", out.First())
	}
}
