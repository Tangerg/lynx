package http

import (
	"context"
	"net/http"
	"time"

	"github.com/Tangerg/sse"
	"go.opentelemetry.io/otel/attribute"

	"github.com/Tangerg/lynx/lyra/rpc/dispatch"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// heartbeatInterval is how often an idle streaming response emits an SSE
// comment frame to keep the connection alive through proxies (TRANSPORT
// §7/§14) — e.g. while a turn waits on a slow LLM round.
const heartbeatInterval = 15 * time.Second

// serveStream drives a streamable-HTTP response (TRANSPORT §6.4): the POST
// response body IS this call's event stream. The first SSE frame is the
// call's JSON-RPC response (carries the envelope id, NOT an SSE id: — a
// one-shot ack, not a replayable run event, §7); each subsequent frame is
// a notifications.run.event with SSE id: = eventId. The loop ends when the
// run stream closes (terminal run.finished → the hub closed the channel)
// or the client disconnects — a disconnect only detaches; the run keeps
// running server-side and the client resumes via runs.subscribe (§9.2).
func (s *Server) serveStream(w http.ResponseWriter, r *http.Request, resp *transport.Response, events <-chan protocol.RunEvent, methodLabel string) {
	// Proxy hints + observability headers before NewHTTPWriter — the
	// library adds Content-Type: text/event-stream itself and leaves our
	// stricter Cache-Control intact (it only fills no-cache when unset).
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Server", s.serverID)
	if methodLabel != "" {
		w.Header().Set("X-Method", methodLabel)
	}
	echoTraceID(w, r)

	sw, err := sse.NewHTTPWriter(w)
	if err != nil {
		writeFlatError(w, r, http.StatusInternalServerError, "streaming unsupported", false)
		return
	}
	ctx := r.Context()

	// First frame: this call's JSON-RPC response (the runId ack), no SSE id.
	if err := writeSSEMessage(ctx, sw, "", resp); err != nil {
		return
	}

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return // terminal run.finished — the hub closed the stream
			}
			notif, err := dispatch.EncodeRunEvent(ev)
			if err != nil {
				recordError(ctx, "rpc.encode-run-event", err,
					attribute.String("run.id", ev.RunID),
					attribute.String("run.event.id", ev.EventID),
				)
				continue
			}
			if err := writeSSEMessage(ctx, sw, ev.EventID, notif); err != nil {
				return // write failed — client gone; run continues server-side
			}
		case <-ticker.C:
			if err := sw.Comment(ctx, "heartbeat"); err != nil {
				return
			}
		case <-ctx.Done():
			return // client disconnect — detach only (TRANSPORT §6.4 / API §3)
		}
	}
}

// writeSSEMessage encodes msg as JSON and emits one SSE frame. eventID is
// the SSE `id:` line — set for run-event frames (drives Last-Event-Id
// resume), empty for the one-shot response ack frame.
func writeSSEMessage(ctx context.Context, sw *sse.Writer, eventID string, msg transport.Message) error {
	body, err := transport.EncodeMessage(msg)
	if err != nil {
		return err
	}
	return sw.Message(ctx, sse.Message{ID: eventID, Data: body})
}
