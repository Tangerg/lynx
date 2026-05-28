package http

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// heartbeatInterval is how often the SSE writer pushes a comment
// frame to keep the connection alive through proxies (API.md §2.1).
const heartbeatInterval = 15 * time.Second

// handleStream serves GET /v1/rpc/stream — the SSE notification
// fan-out. Every active client receives every server-emitted
// notification (per the current single-tenant model); the
// Last-Event-Id header drives replay of recently buffered events.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeTransportError(w, r, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	// SSE response headers — these MUST precede WriteHeader.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
	w.Header().Set("X-Lyra-Server", s.serverID)
	w.Header().Set("X-Lyra-Method", "stream") // §10.2 — fixed label for long-lived stream
	echoTraceID(w, r)
	w.WriteHeader(http.StatusOK)

	client := &clientConn{
		send: make(chan transport.Message, 64),
		done: make(chan struct{}),
	}
	dereg := s.clients.register(client)
	defer dereg()

	// Last-Event-Id replay (API.md §3.3 + §10.1). Reverse-proxy
	// preserves it as a request header; EventSource sends it
	// automatically on reconnect.
	lastEventID := strings.TrimSpace(r.Header.Get("Last-Event-Id"))
	if lastEventID != "" {
		s.replay(w, flusher, lastEventID)
	}

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-client.send:
			if !ok {
				return
			}
			if err := writeSSE(w, msg); err != nil {
				return
			}
			flusher.Flush()
		case <-ticker.C:
			// Comment-only frame — invisible to event handlers but
			// keeps idle proxies from killing the connection.
			if _, err := fmt.Fprint(w, ":heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		case <-client.done:
			return
		}
	}
}

// replay pushes every buffered notification newer than lastEventID
// to a freshly-reconnected client. Errors (e.g. client already gone)
// fall through to the main loop which will see the same ctx cancel.
func (s *Server) replay(w http.ResponseWriter, flusher http.Flusher, lastEventID string) {
	s.streams.mu.Lock()
	bufs := make([]*streamBuffer, 0, len(s.streams.streams))
	for _, buf := range s.streams.streams {
		bufs = append(bufs, buf)
	}
	s.streams.mu.Unlock()

	for _, buf := range bufs {
		for _, rec := range buf.since(lastEventID) {
			if err := writeSSEWithID(w, rec.eventID, rec.msg); err != nil {
				return
			}
		}
	}
	flusher.Flush()
}

// writeSSE marshals msg as a single SSE frame. Notifications get an
// `id:` line when their params carry a stream eventId so client
// EventSource can resume on reconnect.
func writeSSE(w http.ResponseWriter, msg transport.Message) error {
	body, err := transport.EncodeMessage(msg)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", body); err != nil {
		return err
	}
	return nil
}

// writeSSEWithID is like writeSSE but stamps the SSE id field.
func writeSSEWithID(w http.ResponseWriter, eventID string, msg transport.Message) error {
	body, err := transport.EncodeMessage(msg)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "id: %s\ndata: %s\n\n", eventID, body); err != nil {
		return err
	}
	return nil
}
