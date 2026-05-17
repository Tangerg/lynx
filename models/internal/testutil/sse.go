package testutil

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

// JSONServer returns an httptest.Server that responds to every request
// with the given status code + body. Used for non-streaming endpoints
// (chat.Call, embedding.Call, image.Call, etc.).
//
// The optional inspect callback runs on every request, letting tests
// assert that the outgoing request shape (URL / method / headers /
// body) matches expectations.
func JSONServer(status int, body string, inspect ...func(r *http.Request)) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, f := range inspect {
			f(r)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	return srv
}

// BinaryServer is the analogue of JSONServer for non-JSON payloads
// (TTS audio, image bytes, etc.). The body is written as-is with the
// supplied Content-Type.
func BinaryServer(status int, contentType string, body []byte, inspect ...func(r *http.Request)) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, f := range inspect {
			f(r)
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		w.Write(body)
	}))
	return srv
}
