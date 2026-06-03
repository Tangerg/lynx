package openai_test

import (
	"net/http"
	"testing"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/audio/transcription"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/openai"
	"github.com/Tangerg/lynx/pkg/mime"
)

func TestAudioTranscriptionModel_Call_Mock(t *testing.T) {
	srv := testutil.JSONServer(http.StatusOK, `{"text":"hello world"}`)
	t.Cleanup(srv.Close)

	opts, err := transcription.NewOptions("whisper-1")
	if err != nil {
		t.Fatal(err)
	}
	m, err := openai.NewAudioTranscriptionModel(openai.AudioTranscriptionModelConfig{
		APIKey:         model.NewAPIKey("test-key"),
		DefaultOptions: opts,
		RequestOptions: []option.RequestOption{option.WithBaseURL(srv.URL)},
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
