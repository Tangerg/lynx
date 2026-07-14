package revai_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/audio/transcription"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/revai"
)

func TestAudioTranscriptionModel_Call_Mock(t *testing.T) {
	var polls testutil.PollCounter

	srv := testutil.MuxServer(
		// Order matters: more specific routes first.
		testutil.Route{Method: "GET", Contains: "/transcript", Handle: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("hello world\n"))
		}},
		testutil.Route{Method: "POST", Contains: "/jobs", Handle: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":"job-1","status":"in_progress","duration_seconds":1.0,"language":"en"}`))
		}},
		testutil.Route{Method: "GET", Contains: "/jobs/", Handle: func(w http.ResponseWriter, r *http.Request) {
			// Skip the /transcript variant — already caught above.
			if strings.HasSuffix(r.URL.Path, "/transcript") {
				http.NotFound(w, r)
				return
			}
			n := polls.Inc()
			status := "in_progress"
			if n >= 2 {
				status = "transcribed"
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":"job-1","status":"` + status + `","duration_seconds":1.0,"language":"en"}`))
		}},
	)
	t.Cleanup(srv.Close)

	opts, err := transcription.NewOptions("machine_v2")
	if err != nil {
		t.Fatal(err)
	}
	m, err := revai.NewAudioTranscriptionModel(revai.AudioTranscriptionModelConfig{
		APIKey:         model.NewAPIKey("test-key"),
		DefaultOptions: opts,
		BaseURL:        srv.URL,
		PollInterval:   10 * time.Millisecond,
		PollTimeout:    5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	audio, _ := media.NewBytes("audio/mpeg", []byte("FAKE-AUDIO"))
	req, _ := transcription.NewRequest(audio)
	out, err := m.Call(t.Context(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if out.Result == nil || !strings.Contains(out.Result.Text, "hello world") {
		t.Fatalf("text = %q; want to contain 'hello world'", out.Result.Text)
	}
}
