package google_test

import (
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/Tangerg/lynx/core/model"
	tts "github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/models/google"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

func TestAudioTTSModel_Call_Mock(t *testing.T) {
	// Gemini 2.5 TTS routes audio through GenerateContent with inline
	// data — Part.inlineData.{mimeType, data} carries the PCM bytes.
	audioB64 := base64.StdEncoding.EncodeToString([]byte("FAKE-WAV"))
	body := `{
  "candidates": [{
    "content": {"role": "model", "parts": [{"inlineData": {"mimeType": "audio/L16;rate=24000", "data": "` + audioB64 + `"}}]},
    "finishReason": "STOP"
  }],
  "usageMetadata": {"promptTokenCount": 4, "candidatesTokenCount": 0, "totalTokenCount": 4}
}`
	srv := testutil.JSONServer(http.StatusOK, body)
	t.Cleanup(srv.Close)

	opts, err := tts.NewOptions("gemini-2.5-flash-preview-tts")
	if err != nil {
		t.Fatal(err)
	}
	m, err := google.NewAudioTTSModel(google.AudioTTSModelConfig{
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
}
