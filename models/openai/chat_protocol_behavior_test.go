package openai_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openai/openai-go/v3/option"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/models/internal/conformance"
	lynxopenai "github.com/Tangerg/lynx/models/openai"
)

func TestChat_BehaviorConformance(t *testing.T) {
	streamCase := func(t *testing.T) conformance.StreamBehaviorCase {
		t.Helper()
		server, lifecycle := conformance.NewBlockingServer(t, writeOpenAIBehaviorChunk)
		return conformance.StreamBehaviorCase{Streamer: newOpenAIBehaviorChat(t, server.URL), Lifecycle: lifecycle}
	}
	conformance.ChatBehaviorSuite{
		Request: newCoreChatRequest,
		CallCancellation: func(t *testing.T) conformance.CallBehaviorCase {
			t.Helper()
			server, lifecycle := conformance.NewBlockingServer(t, nil)
			return conformance.CallBehaviorCase{Model: newOpenAIBehaviorChat(t, server.URL), Lifecycle: lifecycle}
		},
		StreamCancellation: streamCase,
		EarlyStop:          streamCase,
		FirstError: func(t *testing.T) corechat.Streamer {
			t.Helper()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(writer, "data: {\"id\":\"before-error\",\"model\":\"gpt-5.2\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"before\"}}]}\n\n")
				fmt.Fprint(writer, "data: {\n\n")
				fmt.Fprint(writer, "data: {\"id\":\"after-error\",\"model\":\"gpt-5.2\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"after\"}}]}\n\n")
				fmt.Fprint(writer, "data: [DONE]\n\n")
			}))
			t.Cleanup(server.Close)
			return newOpenAIBehaviorChat(t, server.URL)
		},
	}.Run(t)
}

func newOpenAIBehaviorChat(t *testing.T, baseURL string) *lynxopenai.Chat {
	t.Helper()
	adapter, err := lynxopenai.NewChat(lynxopenai.ChatConfig{
		APIKey:         model.NewAPIKey("test-key"),
		DefaultOptions: corechat.Options{Model: "gpt-5.2"},
		RequestOptions: []option.RequestOption{option.WithBaseURL(baseURL)},
	})
	if err != nil {
		t.Fatalf("NewChat: %v", err)
	}
	return adapter
}

func writeOpenAIBehaviorChunk(writer http.ResponseWriter) {
	writer.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprint(writer, "data: {\"id\":\"lifecycle\",\"model\":\"gpt-5.2\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ready\"}}]}\n\n")
	writer.(http.Flusher).Flush()
}
