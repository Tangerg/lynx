package openai_test

import (
	"net/http"
	"testing"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/modeltest"
	"github.com/Tangerg/lynx/core/transcription"
	"github.com/Tangerg/lynx/models/protocol/openai"
)

// Audio translation reuses the transcription.Model interface — input
// is non-English audio, output is the English translation.
func TestAudioTranslationModel_Call_Mock(t *testing.T) {
	srv := modeltest.JSONServer(http.StatusOK, `{"text":"good morning"}`)
	t.Cleanup(srv.Close)

	opts, err := transcription.NewOptions("whisper-1")
	if err != nil {
		t.Fatal(err)
	}
	m, err := openai.NewAudioTranslationModel(openai.AudioTranslationModelConfig{
		Provider:       "openai",
		APIKey:         "test-key",
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
	if out.Output == nil || out.Output.Text != "good morning" {
		t.Fatalf("text = %v; want 'good morning'", out.Output)
	}
}
