package ollama_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/internal/conformance"
	"github.com/Tangerg/lynx/models/ollama"
)

func TestChat_BehaviorConformance(t *testing.T) {
	streamCase := func(t *testing.T) conformance.StreamBehaviorCase {
		t.Helper()
		server, lifecycle := conformance.NewBlockingServer(t, writeOllamaBehaviorChunk)
		return conformance.StreamBehaviorCase{Streamer: newOllamaBehaviorChat(t, server.URL), Lifecycle: lifecycle}
	}
	conformance.ChatBehaviorSuite{
		Request: newProtocolChatRequest,
		CallCancellation: func(t *testing.T) conformance.CallBehaviorCase {
			t.Helper()
			server, lifecycle := conformance.NewBlockingServer(t, nil)
			return conformance.CallBehaviorCase{Model: newOllamaBehaviorChat(t, server.URL), Lifecycle: lifecycle}
		},
		StreamCancellation: streamCase,
		EarlyStop:          streamCase,
		FirstError: func(t *testing.T) corechat.Streamer {
			t.Helper()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/x-ndjson")
				fmt.Fprintln(writer, `{"model":"qwen3:8b","message":{"role":"assistant","content":"before"},"done":false}`)
				fmt.Fprintln(writer, `{`)
				fmt.Fprintln(writer, `{"model":"qwen3:8b","message":{"role":"assistant","content":"after"},"done":false}`)
			}))
			t.Cleanup(server.Close)
			return newOllamaBehaviorChat(t, server.URL)
		},
	}.Run(t)
}

func newOllamaBehaviorChat(t *testing.T, baseURL string) *ollama.Chat {
	t.Helper()
	adapter, err := ollama.NewChat(ollama.ChatConfig{
		DefaultOptions: corechat.Options{Model: "qwen3:8b"},
		BaseURL:        baseURL,
	})
	if err != nil {
		t.Fatalf("NewChat: %v", err)
	}
	return adapter
}

func writeOllamaBehaviorChunk(writer http.ResponseWriter) {
	writer.Header().Set("Content-Type", "application/x-ndjson")
	fmt.Fprintln(writer, `{"model":"qwen3:8b","message":{"role":"assistant","content":"ready"},"done":false}`)
	writer.(http.Flusher).Flush()
}
