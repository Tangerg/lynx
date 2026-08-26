package lmnt_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	tts "github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/models/lmnt"
)

func TestAudioTTSModel_Call_Mock(t *testing.T) {
	var requestBody struct {
		Text  string `json:"text"`
		Voice string `json:"voice"`
		Model string `json:"model"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("X-API-Key = %q", request.Header.Get("X-API-Key"))
		}
		if request.Header.Get("lmnt-version") != lmnt.CurrentAPIVersion {
			t.Errorf("lmnt-version = %q", request.Header.Get("lmnt-version"))
		}
		if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(writer, "bad request", http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "audio/wav")
		writer.Header().Set("request-id", "request-1")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("FAKE-WAV"))
	}))
	t.Cleanup(srv.Close)

	opts, err := tts.NewOptions(lmnt.ModelBlizzard)
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
	if out.Output == nil {
		t.Fatal("nil output")
	}
	if string(out.Output.Audio) != "FAKE-WAV" {
		t.Fatalf("audio = %q", out.Output.Audio)
	}
	if requestBody.Text != "hello world" || requestBody.Model != lmnt.ModelBlizzard || requestBody.Voice != "lily" {
		t.Fatalf("request = %#v", requestBody)
	}
}
