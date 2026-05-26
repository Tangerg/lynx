package deepgram_test

import (
	"net/http"
	"testing"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/audio/transcription"
	"github.com/Tangerg/lynx/models/deepgram"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/pkg/mime"
)

// Deepgram /listen response shape (simplified — full payload has
// channels/alternatives/words/etc.; the package picks out the top
// transcript via the standard JSON path).
const deepgramSTTJSON = `{
  "metadata": {"request_id":"abc","model_info":{"model-uuid":{"name":"nova-3","tier":"nova"}}},
  "results": {
    "channels": [{
      "alternatives": [{
        "transcript": "hello world",
        "confidence": 0.99,
        "words": []
      }]
    }]
  }
}`

func TestAudioTranscriptionModel_Call_Mock(t *testing.T) {
	srv := testutil.JSONServer(http.StatusOK, deepgramSTTJSON)
	t.Cleanup(srv.Close)

	opts, err := transcription.NewOptions("nova-3")
	if err != nil {
		t.Fatal(err)
	}
	m, err := deepgram.NewAudioTranscriptionModel(&deepgram.AudioTranscriptionModelConfig{
		APIKey:         model.NewAPIKey("test-key"),
		DefaultOptions: opts,
		BaseURL:        srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}

	audio, err := media.NewMedia(mime.MustNew("audio", "mpeg"), []byte("FAKE-AUDIO"))
	if err != nil {
		t.Fatal(err)
	}
	req, err := transcription.NewRequest(audio)
	if err != nil {
		t.Fatal(err)
	}
	out, err := m.Call(t.Context(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if out.Result == nil || out.Result.Text == "" {
		t.Fatal("empty transcript")
	}
}
