package azureopenai_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model"
	tts "github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/models/azureopenai"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

func TestAudioTTSModel_Call_Mock(t *testing.T) {
	srv := testutil.BinaryServer(200, "audio/mpeg", []byte("FAKE-MP3"))
	t.Cleanup(srv.Close)

	opts, err := tts.NewOptions("tts-1-deployment")
	if err != nil {
		t.Fatal(err)
	}
	opts.Voice = "alloy"
	m, err := azureopenai.NewAudioTTSModel(azureopenai.AudioTTSModelConfig{
		APIKey:         model.NewAPIKey("test-key"),
		Endpoint:       srv.URL,
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
