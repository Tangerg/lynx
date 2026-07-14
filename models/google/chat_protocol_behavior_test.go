package google_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/google"
	"github.com/Tangerg/lynx/models/internal/conformance"
)

func TestChat_BehaviorConformance(t *testing.T) {
	streamCase := func(t *testing.T) conformance.StreamBehaviorCase {
		t.Helper()
		server, lifecycle := conformance.NewBlockingServer(t, writeGoogleBehaviorChunk)
		return conformance.StreamBehaviorCase{Streamer: newGoogleBehaviorChat(t, server.URL), Lifecycle: lifecycle}
	}
	conformance.ChatBehaviorSuite{
		Request: newProtocolChatRequest,
		CallCancellation: func(t *testing.T) conformance.CallBehaviorCase {
			t.Helper()
			server, lifecycle := conformance.NewBlockingServer(t, nil)
			return conformance.CallBehaviorCase{Model: newGoogleBehaviorChat(t, server.URL), Lifecycle: lifecycle}
		},
		StreamCancellation: streamCase,
		EarlyStop:          streamCase,
		FirstError: func(t *testing.T) corechat.Streamer {
			t.Helper()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(writer, "data: {\"responseId\":\"before-error\",\"modelVersion\":\"gemini-3-pro-001\",\"candidates\":[{\"index\":0,\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"before\"}]}}]}\n\n")
				fmt.Fprint(writer, "data: {\n\n")
				fmt.Fprint(writer, "data: {\"responseId\":\"after-error\",\"modelVersion\":\"gemini-3-pro-001\",\"candidates\":[{\"index\":0,\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"after\"}]}}]}\n\n")
			}))
			t.Cleanup(server.Close)
			return newGoogleBehaviorChat(t, server.URL)
		},
	}.Run(t)
}

func newGoogleBehaviorChat(t *testing.T, baseURL string) *google.Chat {
	t.Helper()
	adapter, err := google.NewChat(google.ChatConfig{
		APIKey:         "test-key",
		DefaultOptions: corechat.Options{Model: "gemini-3-pro"},
		BaseURL:        baseURL,
	})
	if err != nil {
		t.Fatalf("NewChat: %v", err)
	}
	return adapter
}

func writeGoogleBehaviorChunk(writer http.ResponseWriter) {
	writer.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprint(writer, "data: {\"responseId\":\"lifecycle\",\"modelVersion\":\"gemini-3-pro-001\",\"candidates\":[{\"index\":0,\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"ready\"}]}}]}\n\n")
	writer.(http.Flusher).Flush()
}
