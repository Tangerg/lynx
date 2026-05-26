package google_test

import (
	"net/http"
	"testing"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/audio/transcription"
	"github.com/Tangerg/lynx/models/google"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/pkg/mime"
)

func TestAudioTranscriptionModel_Call_Mock(t *testing.T) {
	// Gemini transcription is multimodal chat: prompt + audio → text.
	body := `{
  "candidates": [{
    "content": {"role": "model", "parts": [{"text": "hello world"}]},
    "finishReason": "STOP"
  }],
  "usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 2, "totalTokenCount": 12}
}`
	srv := testutil.JSONServer(http.StatusOK, body)
	t.Cleanup(srv.Close)

	opts, err := transcription.NewOptions("gemini-2.0-flash")
	if err != nil {
		t.Fatal(err)
	}
	m, err := google.NewAudioTranscriptionModel(&google.AudioTranscriptionModelConfig{
		APIKey:         model.NewAPIKey("test-key"),
		DefaultOptions: opts,
		BaseURL:        srv.URL,
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
