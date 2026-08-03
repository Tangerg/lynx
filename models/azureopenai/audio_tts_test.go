package azureopenai_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/modeltest"
	tts "github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/models/azureopenai"
)

func TestAudioTTSModel_Call_Mock(t *testing.T) {
	srv := modeltest.BinaryServer(200, "audio/mpeg", []byte("FAKE-MP3"))
	t.Cleanup(srv.Close)

	opts, err := tts.NewOptions("tts-1-deployment")
	if err != nil {
		t.Fatal(err)
	}
	opts.Voice = "alloy"
	m, err := azureopenai.NewAudioTTSModel(azureopenai.AudioTTSModelConfig{
		APIKey:         "test-key",
		BaseURL:        srv.URL + "/openai/v1/",
		DefaultOptions: opts,
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
