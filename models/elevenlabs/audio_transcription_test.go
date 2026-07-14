package elevenlabs_test

import (
	"net/http"
	"testing"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/audio/transcription"
	"github.com/Tangerg/lynx/models/elevenlabs"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

const elevenSTTJSON = `{
  "language_code": "en",
  "language_probability": 0.99,
  "text": "hello world",
  "words": []
}`

func TestAudioTranscriptionModel_Call_Mock(t *testing.T) {
	srv := testutil.JSONServer(http.StatusOK, elevenSTTJSON)
	t.Cleanup(srv.Close)

	opts, err := transcription.NewOptions("scribe_v1")
	if err != nil {
		t.Fatal(err)
	}
	m, err := elevenlabs.NewAudioTranscriptionModel(elevenlabs.AudioTranscriptionModelConfig{
		APIKey:         model.NewAPIKey("test-key"),
		DefaultOptions: opts,
		BaseURL:        srv.URL,
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
	if out.Result == nil || out.Result.Text != "hello world" {
		t.Fatalf("text = %q; want 'hello world'", out.Result.Text)
	}
}
