package deepgram_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/audio/tts"
	"github.com/Tangerg/lynx/models/deepgram"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

func TestAudioTTSModel_Call_Mock(t *testing.T) {
	// Deepgram /speak returns raw audio bytes.
	srv := testutil.BinaryServer(200, "audio/mpeg", []byte("FAKE-MP3"))
	t.Cleanup(srv.Close)

	opts, err := tts.NewOptions("aura-asteria-en")
	if err != nil {
		t.Fatal(err)
	}
	m, err := deepgram.NewAudioTTSModel(&deepgram.AudioTTSModelConfig{
		ApiKey:         model.NewApiKey("test-key"),
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
	if m.Metadata().Provider != deepgram.Provider {
		t.Errorf("provider = %q", m.Metadata().Provider)
	}
}
