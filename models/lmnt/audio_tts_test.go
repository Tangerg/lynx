package lmnt_test

import (
	"encoding/base64"
	"net/http"
	"testing"

	tts "github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/lmnt"
)

func TestAudioTTSModel_Call_Mock(t *testing.T) {
	audioB64 := base64.StdEncoding.EncodeToString([]byte("FAKE-WAV"))
	body := `{"audio":"` + audioB64 + `","seed":42,"sample_rate":24000,"duration":1.0,"durations":[]}`
	srv := testutil.JSONServer(http.StatusOK, body)
	t.Cleanup(srv.Close)

	opts, err := tts.NewOptions("aurora")
	if err != nil {
		t.Fatal(err)
	}
	opts.Voice = "lily"
	m, err := lmnt.NewAudioTTSModel(lmnt.AudioTTSModelConfig{
		APIKey:         "test-key",
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
