package modeltest

import (
	"fmt"
	"net/http"
	"net/http/httptest"
)

// OpenAISSEServer returns an httptest.Server that streams `chunks` as
// OpenAI-shaped Server-Sent Events:
//
//	data: <chunk-1>\n\n
//	data: <chunk-2>\n\n
//	...
//	data: [DONE]\n\n
//
// Each chunk should be a JSON-encoded `ChatCompletionChunk` body.
// The server is registered with t.Cleanup so callers don't have to
// defer Close().
//
// Used by every OpenAI-compatible vendor (openai / azureopenai /
// deepseek / moonshot / openrouter / xai / groq / together / fireworks /
// perplexity / alibaba / zhipu / minimax / ...).
func OpenAISSEServer(chunks []string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "stream unsupported", http.StatusInternalServerError)
			return
		}
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	return srv
}

// AnthropicEvent is a single named SSE event for Anthropic's
// multi-event-type streaming protocol.
type AnthropicEvent struct {
	Event string
	Data  string
}

// AnthropicSSEServer returns an httptest.Server that streams `events`
// as Anthropic-shaped SSE:
//
//	event: message_start\ndata: {...}\n\n
//	event: content_block_delta\ndata: {...}\n\n
//	...
//	event: message_stop\ndata: {...}\n\n
//
// Anthropic uses named events rather than a single sentinel; the
// caller is responsible for providing the right sequence.
func AnthropicSSEServer(events []AnthropicEvent) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "stream unsupported", http.StatusInternalServerError)
			return
		}
		for _, e := range events {
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Event, e.Data)
			flusher.Flush()
		}
	}))
	return srv
}

func JSONServer(status int, body string, inspections ...func(request *http.Request)) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		for _, inspect := range inspections {
			inspect(request)
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(status)
		fmt.Fprint(writer, body)
	}))
	return server
}

func BinaryServer(status int, contentType string, body []byte, inspections ...func(request *http.Request)) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		for _, inspect := range inspections {
			inspect(request)
		}
		writer.Header().Set("Content-Type", contentType)
		writer.WriteHeader(status)
		_, _ = writer.Write(body)
	}))
	return server
}
