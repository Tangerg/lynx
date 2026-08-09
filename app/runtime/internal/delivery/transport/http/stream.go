package http

import (
	"iter"
	"net/http"
	"time"

	"github.com/Tangerg/sse"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/dispatch"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// heartbeatInterval is how often an idle streaming response emits an SSE
// comment frame to keep the connection alive through proxies (TRANSPORT
// §7/§14) — e.g. while a Run waits on a slow LLM round.
const heartbeatInterval = 15 * time.Second

// streamWriteTimeout bounds a single SSE frame write. ctx cancellation cannot
// interrupt an in-flight net/http Write (only the select between writes observes
// it), so a connected-but-stalled client — a full TCP window from a wedged proxy
// or paused reader — would otherwise park this goroutine (backing up the frame
// source feeding it) inside Write until TCP keep-alive tears the socket down hours later,
// also blocking graceful shutdown. A per-frame deadline (reset before every write,
// so arbitrarily long idle streams still live) makes one blocked write fail and
// detach instead.
const streamWriteTimeout = 30 * time.Second

// serveStream drives a streamable-HTTP response (TRANSPORT §6.4): the POST
// response body IS this call's event stream. The first SSE frame is the
// call's JSON-RPC response (carries the envelope id, NOT an SSE id: — a
// one-shot ack, not a replayable run event, §7); each subsequent frame is
// a notifications.run.event with SSE id: = eventId. The loop ends when the
// run stream ends (terminal segment.finished → the source sequence is drained)
// or the client disconnects — a disconnect only detaches; the run keeps
// running server-side and the client resumes via runs.subscribe (§9.2).
func (s *Server) serveStream(w http.ResponseWriter, r *http.Request, resp *transport.Response, events iter.Seq[dispatch.StreamFrame], methodLabel string) {
	// Proxy hints + observability headers before NewHTTPWriter — the
	// library adds Content-Type: text/event-stream itself and leaves our
	// stricter Cache-Control intact (it only fills no-cache when unset).
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("X-Accel-Buffering", "no")
	if methodLabel != "" {
		w.Header().Set("X-Method", methodLabel)
	}

	sw := sse.NewHTTPWriter(w)
	ctx := r.Context()
	// A fresh deadline per frame (see streamWriteTimeout) bounds each blocking Write
	// on the underlying connection. recordingResponseWriter.Unwrap lets the
	// controller reach it; if the platform can't set a deadline the write stays
	// unbounded (best effort), so the error is intentionally ignored.
	rc := http.NewResponseController(w)
	setWriteDeadline := func() { _ = rc.SetWriteDeadline(time.Now().Add(streamWriteTimeout)) }

	// First frame: this call's JSON-RPC response (the runId ack), no SSE id.
	setWriteDeadline()
	if err := writeSSEMessage(sw, "", resp); err != nil {
		return
	}

	// Bridge the synchronous frame sequence onto a channel this loop can wait on
	// alongside the heartbeat ticker and client cancellation — the one goroutine
	// the run-event chain needs, since the source blocks between frames and cannot
	// be selected on directly. Request-context cancellation unwinds the source (the
	// run subscription aborts / the workspace channel closes) so the range ends;
	// the send is ctx-guarded so a stalled write cannot strand this goroutine.
	frames := make(chan dispatch.StreamFrame)
	go func() {
		defer close(frames)
		for frame := range events {
			select {
			case frames <- frame:
			case <-ctx.Done():
				return
			}
		}
	}()

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case frame, ok := <-frames:
			if !ok {
				return // stream done — the bridge closed frames after the source drained
			}
			// The dispatch already encoded the frame (method + params) and set
			// its SSE id (replayable run events carry one; non-replayable run
			// events and workspace events don't). The transport just writes it.
			setWriteDeadline()
			if err := writeSSEMessage(sw, frame.SSEID, frame.Notification); err != nil {
				return // write failed — client gone; the source continues server-side
			}
		case <-ticker.C:
			setWriteDeadline()
			if err := sw.Comment("heartbeat"); err != nil {
				return
			}
		case <-ctx.Done():
			return // client disconnect — detach only (TRANSPORT §6.4 / API §3)
		}
	}
}

// writeSSEMessage encodes message as JSON and emits one SSE frame. eventID is
// the SSE `id:` line — set for run-event frames (drives Last-Event-Id
// resume), empty for the one-shot response ack frame.
func writeSSEMessage(sw *sse.Writer, eventID string, message transport.Message) error {
	body, err := transport.EncodeMessage(message)
	if err != nil {
		return err
	}
	return sw.Write(sse.Message{ID: eventID, Data: body})
}
