package http

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/sse"

	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// heartbeatInterval is how often the SSE writer pushes a comment
// frame to keep the connection alive through proxies (API.md §2.1).
const heartbeatInterval = 15 * time.Second

// handleStream serves GET /v2/rpc/stream — the SSE notification
// fan-out. Every active client receives every server-emitted
// notification (per the current single-tenant model); the
// Last-Event-Id header drives replay of recently buffered events.
//
// Frame encoding goes through github.com/Tangerg/sse — it owns the
// WHATWG §9.2 wire format (multi-line data, CR/LF stripping in id,
// auto-flush after each write).
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	// Set Lyra-specific + proxy hints before NewHTTPWriter — the
	// library will then add Content-Type / Connection itself and
	// leave our stricter Cache-Control intact (it only fills in
	// no-cache when the header is unset).
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Server", s.serverID)
	w.Header().Set("X-Method", "stream") // §10.2 — fixed label for long-lived stream
	echoTraceID(w, r)

	sw, err := sse.NewHTTPWriter(w)
	if err != nil {
		writeFlatError(w, r, http.StatusInternalServerError, "streaming unsupported", false)
		return
	}
	ctx := r.Context()

	// Flush headers + a no-op comment immediately so the browser
	// EventSource sees 200 OK and fires `open` without waiting for
	// the first event (which, on an idle session, could be ≥15s
	// away when the heartbeat ticker fires).
	if err := sw.Comment(ctx, "open"); err != nil {
		return
	}

	client := &clientConn{
		send: make(chan transport.Message, 64),
		done: make(chan struct{}),
	}
	dereg := s.clients.register(client)
	defer dereg()

	// Last-Event-Id replay (API.md §3.3 + §10.1). Reverse-proxy
	// preserves it as a request header; EventSource sends it
	// automatically on reconnect.
	if lastEventID := strings.TrimSpace(r.Header.Get("Last-Event-Id")); lastEventID != "" {
		s.replay(ctx, sw, lastEventID)
	}

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-client.send:
			if !ok {
				return
			}
			if err := writeSSEMessage(ctx, sw, "", msg); err != nil {
				return
			}
		case <-ticker.C:
			// Comment frame — invisible to event handlers but keeps
			// idle proxies from killing the connection.
			if err := sw.Comment(ctx, "heartbeat"); err != nil {
				return
			}
		case <-ctx.Done():
			return
		case <-client.done:
			return
		}
	}
}

// replay pushes every buffered notification newer than lastEventID
// to a freshly-reconnected client. Errors (e.g. client already gone)
// fall through to the main loop which will see the same ctx cancel.
func (s *Server) replay(ctx context.Context, sw *sse.Writer, lastEventID string) {
	s.streams.mu.Lock()
	bufs := make([]*streamBuffer, 0, len(s.streams.streams))
	for _, buf := range s.streams.streams {
		bufs = append(bufs, buf)
	}
	s.streams.mu.Unlock()

	// eventIds are globally monotonic, so merge every per-run buffer's
	// tail and replay in global order — a single Last-Event-Id resumes the
	// whole connection linearly even when runs interleaved.
	var recs []streamRecord
	for _, buf := range bufs {
		recs = append(recs, buf.since(lastEventID)...)
	}
	slices.SortFunc(recs, func(a, b streamRecord) int {
		return compareEventID(a.eventID, b.eventID)
	})
	for _, rec := range recs {
		if err := writeSSEMessage(ctx, sw, rec.eventID, rec.msg); err != nil {
			return
		}
	}
}

// writeSSEMessage encodes msg as JSON and emits one SSE frame.
// eventID is optional — when non-empty it becomes the SSE `id:` line
// so EventSource can resume on reconnect via Last-Event-Id.
func writeSSEMessage(ctx context.Context, sw *sse.Writer, eventID string, msg transport.Message) error {
	body, err := transport.EncodeMessage(msg)
	if err != nil {
		return err
	}
	return sw.Message(ctx, sse.Message{ID: eventID, Data: body})
}
