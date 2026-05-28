package assemblyai_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/audio/transcription"
	"github.com/Tangerg/lynx/models/assemblyai"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/pkg/mime"
)

func TestAudioTranscriptionModel_Call_Mock(t *testing.T) {
	var polls testutil.PollCounter

	srv := testutil.MuxServer(
		testutil.Route{Method: "POST", Contains: "/upload", Handle: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"upload_url":"https://cdn.test/audio.bin"}`))
		}},
		testutil.Route{Method: "POST", Contains: "/transcript", Handle: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":"job-1","status":"queued","audio_duration":0}`))
		}},
		testutil.Route{Method: "GET", Contains: "/transcript/", Handle: func(w http.ResponseWriter, r *http.Request) {
			n := polls.Inc()
			status := "processing"
			text := ""
			if n >= 2 {
				status = "completed"
				text = "hello world"
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":"job-1","status":"` + status + `","text":"` + text + `","confidence":0.95,"audio_duration":1}`))
		}},
	)
	t.Cleanup(srv.Close)

	opts, err := transcription.NewOptions("best")
	if err != nil {
		t.Fatal(err)
	}
	m, err := assemblyai.NewAudioTranscriptionModel(assemblyai.AudioTranscriptionModelConfig{
		APIKey:         model.NewAPIKey("test-key"),
		DefaultOptions: opts,
		BaseURL:        srv.URL,
		PollInterval:   10 * time.Millisecond,
		PollTimeout:    5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	audio, _ := media.NewMedia(mime.MustNew("audio", "mpeg"), []byte("FAKE-AUDIO"))
	req, _ := transcription.NewRequest(audio)
	out, err := m.Call(t.Context(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if out.Result == nil || out.Result.Text != "hello world" {
		t.Fatalf("text = %v; want 'hello world'", out.Result)
	}
}
