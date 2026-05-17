package replicate_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/audio/tts"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/replicate"
)

func TestAudioTTSModel_Call_Mock(t *testing.T) {
	var polls testutil.PollCounter

	// We register the audio download path on the same server — the
	// poll-success response will point at /audio.bin (same origin).
	var audioURL string
	srv := testutil.MuxServer(
		testutil.Route{Method: "POST", Contains: "/predictions", Handle: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":"pred-tts","status":"starting","urls":{"get":"/v1/predictions/pred-tts"}}`))
		}},
		testutil.Route{Method: "GET", Contains: "/predictions/", Handle: func(w http.ResponseWriter, r *http.Request) {
			n := polls.Inc()
			status := "processing"
			output := "null"
			if n >= 2 {
				status = "succeeded"
				output = fmt.Sprintf("%q", audioURL)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":"pred-tts","status":"` + status + `","output":` + output + `}`))
		}},
		testutil.Route{Method: "GET", Contains: "/audio.bin", Handle: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Write([]byte("FAKE-MP3"))
		}},
	)
	t.Cleanup(srv.Close)
	audioURL = srv.URL + "/audio.bin"

	opts, err := tts.NewOptions("minimax/speech-02-hd")
	if err != nil {
		t.Fatal(err)
	}
	opts.Voice = "voice-1"
	m, err := replicate.NewAudioTTSModel(&replicate.AudioTTSModelConfig{
		ApiKey:         model.NewApiKey("test-key"),
		DefaultOptions: opts,
		BaseURL:        srv.URL,
		PollInterval:   10 * time.Millisecond,
		PollTimeout:    5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	req, _ := tts.NewRequest("hello world")
	out, err := m.Call(t.Context(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if out.Result == nil {
		t.Fatal("nil result")
	}
}
