package openai_test

import (
	"testing"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/model"
	tts "github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/openai"
)

func TestAudioTTSModel_Call_Mock(t *testing.T) {
	// OpenAI TTS returns raw audio bytes (not JSON).
	canned := []byte("FAKE-AUDIO-BYTES-FOR-TEST")
	srv := testutil.BinaryServer(200, "audio/mpeg", canned)
	t.Cleanup(srv.Close)

	opts, err := tts.NewOptions("tts-1")
	if err != nil {
		t.Fatal(err)
	}
	opts.Voice = "alloy"
	m, err := openai.NewAudioTTSModel(openai.AudioTTSModelConfig{
		APIKey:         model.NewAPIKey("test-key"),
		DefaultOptions: opts,
		RequestOptions: []option.RequestOption{option.WithBaseURL(srv.URL)},
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

func TestAudioTTSModel_Metadata(t *testing.T) {
	srv := testutil.BinaryServer(200, "audio/mpeg", nil)
	t.Cleanup(srv.Close)
	opts, _ := tts.NewOptions("tts-1")
	opts.Voice = "alloy"
	m, _ := openai.NewAudioTTSModel(openai.AudioTTSModelConfig{
		APIKey:         model.NewAPIKey("test-key"),
		DefaultOptions: opts,
		RequestOptions: []option.RequestOption{option.WithBaseURL(srv.URL)},
	})
	if m.Metadata().Provider != openai.Provider {
		t.Errorf("provider = %q; want %q", m.Metadata().Provider, openai.Provider)
	}
}
