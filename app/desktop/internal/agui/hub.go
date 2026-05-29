package agui

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"

	sdkevents "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
)

// streamHub routes server→client notifications to the right SSE stream.
// Keyed by the client-generated connection id (Lyra-Connection-Id header
// on POSTs / ?conn= on the SSE GET — see API.md §3.2). One active stream
// per connection; a re-subscribe replaces the previous channel.
type streamHub struct {
	mu   sync.Mutex
	subs map[string]chan []byte
}

var hub = &streamHub{subs: make(map[string]chan []byte)}

func (h *streamHub) subscribe(conn string) chan []byte {
	ch := make(chan []byte, 256)
	h.mu.Lock()
	h.subs[conn] = ch
	h.mu.Unlock()
	return ch
}

func (h *streamHub) unsubscribe(conn string, ch chan []byte) {
	h.mu.Lock()
	if h.subs[conn] == ch {
		delete(h.subs, conn)
	}
	h.mu.Unlock()
}

// push delivers a frame to the connection's stream, blocking until the SSE
// writer drains it or ctx cancels — so a fast producer doesn't drop events.
// Returns false if no stream is attached or ctx is done.
func (h *streamHub) push(ctx context.Context, conn string, frame []byte) bool {
	h.mu.Lock()
	ch := h.subs[conn]
	h.mu.Unlock()
	if ch == nil {
		return false
	}
	select {
	case ch <- frame:
		return true
	case <-ctx.Done():
		return false
	}
}

// Active-run cancellation registry — runId → cancel. Lets runs.cancel stop
// an in-flight run goroutine.
var (
	runsMu sync.Mutex
	runs   = make(map[string]context.CancelFunc)
)

func registerRun(runID string, cancel context.CancelFunc) {
	runsMu.Lock()
	runs[runID] = cancel
	runsMu.Unlock()
}

func cancelRun(runID string) bool {
	runsMu.Lock()
	cancel, ok := runs[runID]
	delete(runs, runID)
	runsMu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
}

func clearRun(runID string) {
	runsMu.Lock()
	delete(runs, runID)
	runsMu.Unlock()
}

var errStreamClosed = errors.New("stream closed")

// notificationFrame builds a JSON-RPC Notification (no id) ready to write
// into the SSE `data:` field.
func notificationFrame(method string, params any) []byte {
	p, _ := json.Marshal(params)
	frame, _ := json.Marshal(struct {
		JSONRPC string          `json:"jsonrpc"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}{JSONRPC: "2.0", Method: method, Params: p})
	return frame
}

type runEventParams struct {
	RunID   string          `json:"runId"`
	EventID string          `json:"eventId"`
	Ts      string          `json:"ts"`
	Event   json.RawMessage `json:"event"`
}

// startRunStream launches a run for the given connection and streams its
// AG-UI events as `notifications/run/event`, closing with
// `notifications/run/closed`. Returns the runId immediately (the Response
// to runs.start); the events flow asynchronously to the conn's SSE stream.
func startRunStream(conn string, params json.RawMessage) string {
	var p struct {
		SessionID string          `json:"sessionId"`
		RunID     string          `json:"runId"`
		Messages  []ClientMessage `json:"messages"`
	}
	_ = json.Unmarshal(params, &p)
	runID := p.RunID
	if runID == "" {
		runID = newID("run")
	}
	input := RunAgentInput{ThreadID: p.SessionID, RunID: runID, Messages: p.Messages}

	ctx, cancel := context.WithCancel(context.Background())
	registerRun(runID, cancel)

	go func() {
		defer clearRun(runID)
		seq := 0
		emit := func(ev sdkevents.Event) error {
			raw, err := ev.ToJSON()
			if err != nil {
				return err
			}
			seq++
			frame := notificationFrame("notifications/run/event", runEventParams{
				RunID:   runID,
				EventID: strconv.Itoa(seq),
				Ts:      time.Now().Format(time.RFC3339),
				Event:   raw,
			})
			if !hub.push(ctx, conn, frame) {
				return errStreamClosed
			}
			return nil
		}
		Run(ctx, input, emit)

		status := "ok"
		if ctx.Err() != nil {
			status = "cancelled"
		}
		// Deliver the close even after cancel (a fresh, bounded ctx so we
		// never block on a vanished consumer).
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer closeCancel()
		hub.push(closeCtx, conn, notificationFrame("notifications/run/closed", map[string]any{
			"runId":  runID,
			"status": status,
		}))
	}()
	return runID
}
