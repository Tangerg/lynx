package hume_test

import (
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/audio/tts"
	"github.com/Tangerg/lynx/models/hume"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

func TestAudioTTSModel_Call_Mock(t *testing.T) {
	audioB64 := base64.StdEncoding.EncodeToString([]byte("FAKE-MP3"))
	body := `{"generations":[{"generation_id":"g1","audio":"` + audioB64 + `","duration":1.0,"file_size":8,"snippets":[],"encoding":{"format":"mp3","sample_rate":44100}}]}`
	srv := testutil.JSONServer(http.StatusOK, body)
	t.Cleanup(srv.Close)

	opts, err := tts.NewOptions("octave-tts")
	if err != nil {
		t.Fatal(err)
	}
	opts.Voice = "test-voice"
	m, err := hume.NewAudioTTSModel(&hume.AudioTTSModelConfig{
		APIKey:         model.NewAPIKey("test-key"),
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
	if m.Metadata().Provider != hume.Provider {
		t.Errorf("provider = %q", m.Metadata().Provider)
	}
}
