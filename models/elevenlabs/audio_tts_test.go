package elevenlabs_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model"
	tts "github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/models/elevenlabs"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

func TestAudioTTSModel_Call_Mock(t *testing.T) {
	// ElevenLabs returns raw audio bytes from /text-to-speech.
	srv := testutil.BinaryServer(200, "audio/mpeg", []byte("FAKE-MP3"))
	t.Cleanup(srv.Close)

	opts, err := tts.NewOptions("eleven_v3")
	if err != nil {
		t.Fatal(err)
	}
	opts.Voice = "voice-id-test"

	m, err := elevenlabs.NewAudioTTSModel(elevenlabs.AudioTTSModelConfig{
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
	if m.Metadata().Provider != elevenlabs.Provider {
		t.Errorf("provider = %q; want %q", m.Metadata().Provider, elevenlabs.Provider)
	}
}
