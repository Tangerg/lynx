package ollama_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/ollama"
)

// Ollama native chat uses application/x-ndjson — newline-delimited
// JSON, one ChatResponse per line. Final chunk has done=true.
func TestNativeChatModel_Call_Mock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		// Single response (stream=false → SDK still uses ndjson framing).
		fmt.Fprint(w, `{"model":"llama3.2","created_at":"2025-01-01T00:00:00Z","message":{"role":"assistant","content":"hello world"},"done":true,"done_reason":"stop","total_duration":1000000,"prompt_eval_count":3,"eval_count":2}`+"\n")
	}))
	t.Cleanup(srv.Close)

	opts, err := chat.NewOptions("llama3.2")
	if err != nil {
		t.Fatal(err)
	}
	m, err := ollama.NewNativeChatModel(ollama.NativeChatModelConfig{
		DefaultOptions: opts,
		BaseURL:        srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}

	req, _ := chat.NewRequest([]chat.Message{chat.NewUserMessage("hi")})
	resp, err := m.Call(t.Context(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Result.AssistantMessage.JoinedText() != "hello world" {
		t.Errorf("text = %q; want %q", resp.Result.AssistantMessage.JoinedText(), "hello world")
	}
	if m.Metadata().Provider != ollama.Provider {
		t.Errorf("provider = %q", m.Metadata().Provider)
	}
}

func TestNativeChatModel_Stream_Mock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, _ := w.(http.Flusher)
		chunks := []string{
			`{"model":"llama3.2","created_at":"2025-01-01T00:00:00Z","message":{"role":"assistant","content":"hello"},"done":false}`,
			`{"model":"llama3.2","created_at":"2025-01-01T00:00:00Z","message":{"role":"assistant","content":" world"},"done":false}`,
			`{"model":"llama3.2","created_at":"2025-01-01T00:00:00Z","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","total_duration":1000000,"prompt_eval_count":3,"eval_count":2}`,
		}
		for _, c := range chunks {
			fmt.Fprint(w, c+"\n")
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	t.Cleanup(srv.Close)

	opts, err := chat.NewOptions("llama3.2")
	if err != nil {
		t.Fatal(err)
	}
	m, err := ollama.NewNativeChatModel(ollama.NativeChatModelConfig{
		DefaultOptions: opts,
		BaseURL:        srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}

	req, _ := chat.NewRequest([]chat.Message{chat.NewUserMessage("hi")})
	resps, err := testutil.Collect(m.Stream(t.Context(), req))
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if len(resps) < 2 {
		t.Fatalf("got %d chunks; want at least 2", len(resps))
	}
}
