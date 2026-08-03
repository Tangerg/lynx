package deepgram_test

import (
	"net/http"
	"testing"

	"github.com/Tangerg/lynx/core/modeltest"
	tts "github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/models/deepgram"
)

func TestAudioTTSModel_Call_Mock(t *testing.T) {
	// Deepgram /speak returns raw audio bytes.
	srv := modeltest.MuxServer(modeltest.Route{Method: "POST", Contains: "/speak", Handle: func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("encoding") != "linear16" || query.Get("container") != "wav" || query.Get("speed") != "1.2" {
			t.Errorf("query = %v", query)
		}
		w.Header().Set("Content-Type", "audio/wav")
		w.Write([]byte("FAKE-WAV"))
	}})
	t.Cleanup(srv.Close)

	opts, err := tts.NewOptions("aura-asteria-en")
	if err != nil {
		t.Fatal(err)
	}
	opts.OutputFormat = "wav"
	opts.Speed = 1.2
	m, err := deepgram.NewAudioTTSModel(deepgram.AudioTTSModelConfig{
		APIKey:         "test-key",
		DefaultOptions: opts,
		BaseURL:        srv.URL,
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
