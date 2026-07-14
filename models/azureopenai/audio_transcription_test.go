package azureopenai_test

import (
	"net/http"
	"testing"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/transcription"
	"github.com/Tangerg/lynx/models/azureopenai"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

func TestAudioTranscriptionModel_Call_Mock(t *testing.T) {
	srv := testutil.JSONServer(http.StatusOK, `{"text":"hello world"}`)
	t.Cleanup(srv.Close)

	opts, err := transcription.NewOptions("whisper-deployment")
	if err != nil {
		t.Fatal(err)
	}
	m, err := azureopenai.NewAudioTranscriptionModel(azureopenai.AudioTranscriptionModelConfig{
		APIKey:         "test-key",
		Endpoint:       srv.URL,
		DefaultOptions: opts,
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
		t.Fatalf("text = %v; want 'hello world'", out.Result)
	}
}
