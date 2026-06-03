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
// frame to keep the connection alive through proxies (TRANSPORT §7/§14).
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
	w.Header().Set("X-Method", "stream") // TRANSPORT §16 — fixed label for long-lived stream
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

	// conn identifies the SSE connection (TRANSPORT §6.1 / §7). Browsers
	// can't set headers on EventSource, so it rides the query string;
	// runs.start/resume/subscribe name the same value via X-Conn-Id to
	// route their run's events here.
	connID := strings.TrimSpace(r.URL.Query().Get("conn"))

	client := &clientConn{
		send: make(chan transport.Message, 64),
		done: make(chan struct{}),
	}
	dereg := s.clients.register(connID, client)
	defer dereg()

	// Last-Event-Id replay (TRANSPORT §9). Reverse-proxy preserves it as
	// a request header; EventSource sends it automatically on reconnect.
	// Replay is scoped to the runs this conn is subscribed to.
	if lastEventID := strings.TrimSpace(r.Header.Get("Last-Event-Id")); lastEventID != "" {
		s.replay(ctx, sw, connID, lastEventID)
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

// replay pushes every buffered notification newer than lastEventID to a
// freshly-reconnected client, scoped to the runs that conn is subscribed
// to (TRANSPORT §8/§9). Errors (e.g. client already gone) fall through to
// the main loop which will see the same ctx cancel.
func (s *Server) replay(ctx context.Context, sw *sse.Writer, connID, lastEventID string) {
	runIDs := s.clients.runsForConn(connID)
	if len(runIDs) == 0 {
		return
	}

	s.streams.mu.Lock()
	bufs := make([]*streamBuffer, 0, len(runIDs))
	for _, runID := range runIDs {
		if buf, ok := s.streams.streams[runID]; ok {
			bufs = append(bufs, buf)
		}
	}
	s.streams.mu.Unlock()

	// eventIds are globally monotonic, so merge this conn's run buffers'
	// tails and replay in global order — a single Last-Event-Id resumes
	// the connection linearly even when its runs interleaved.
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
