package anthropic_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/modeltest"
	"github.com/Tangerg/lynx/models/protocol/anthropic"
)

func TestChat_BehaviorConformance(t *testing.T) {
	streamCase := func(t *testing.T) modeltest.StreamBehaviorCase {
		t.Helper()
		server, lifecycle := modeltest.NewBlockingServer(t, writeAnthropicBehaviorChunk)
		return modeltest.StreamBehaviorCase{Streamer: newAnthropicBehaviorChat(t, server.URL), Lifecycle: lifecycle}
	}
	modeltest.ChatBehaviorSuite{
		Request: newProtocolChatRequest,
		CallCancellation: func(t *testing.T) modeltest.CallBehaviorCase {
			t.Helper()
			server, lifecycle := modeltest.NewBlockingServer(t, nil)
			return modeltest.CallBehaviorCase{Model: newAnthropicBehaviorChat(t, server.URL), Lifecycle: lifecycle}
		},
		StreamCancellation: streamCase,
		EarlyStop:          streamCase,
		FirstError: func(t *testing.T) corechat.Streamer {
			t.Helper()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "text/event-stream")
				writeAnthropicBehaviorEvent(writer, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"before"}}`)
				fmt.Fprint(writer, "event: content_block_delta\ndata: {\n\n")
				writeAnthropicBehaviorEvent(writer, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"after"}}`)
			}))
			t.Cleanup(server.Close)
			return newAnthropicBehaviorChat(t, server.URL)
		},
	}.Run(t)
}

func newAnthropicBehaviorChat(t *testing.T, baseURL string) *anthropic.Chat {
	t.Helper()
	adapter, err := anthropic.NewChat(anthropic.ChatConfig{
		APIKey:         "test-key",
		DefaultOptions: corechat.Options{Model: "claude-opus-4-6"},
		BaseURL:        baseURL,
	})
	if err != nil {
		t.Fatalf("NewChat: %v", err)
	}
	return adapter
}

func writeAnthropicBehaviorChunk(writer http.ResponseWriter) {
	writer.Header().Set("Content-Type", "text/event-stream")
	writeAnthropicBehaviorEvent(writer, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ready"}}`)
	writer.(http.Flusher).Flush()
}

func writeAnthropicBehaviorEvent(writer http.ResponseWriter, data string) {
	fmt.Fprintf(writer, "event: content_block_delta\ndata: %s\n\n", data)
}
