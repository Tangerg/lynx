package agui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// SSEEncoder writes AG-UI events as Server-Sent Events frames —
// the canonical wire form for AG-UI over HTTP. Each event becomes
// one SSE block:
//
//	event: <EventType>
//	data: <JSON-encoded event>
//	\n
//
// The blank line is the SSE record terminator; clients (including
// the AG-UI JS SDK) split on it.
//
// SSEEncoder is single-writer — concurrent calls require external
// synchronisation. The expected pattern is one encoder per HTTP
// response writer, owned by the goroutine draining the translator.
type SSEEncoder struct {
	w       *bufio.Writer
	flusher http.Flusher // optional; nil when the underlying writer doesn't support flush
}

// NewSSEEncoder wraps w with an SSE frame encoder. If w also
// implements http.Flusher, each Encode call flushes the buffer so
// browsers receive events in real time instead of waiting for
// the kernel buffer to fill. Pass a plain io.Writer (a bytes.Buffer
// in tests) for synchronous capture.
func NewSSEEncoder(w io.Writer) *SSEEncoder {
	enc := &SSEEncoder{w: bufio.NewWriter(w)}
	if f, ok := w.(http.Flusher); ok {
		enc.flusher = f
	}
	return enc
}

// Encode writes one event as an SSE frame. Failure to marshal or
// write surfaces — callers typically log + close the connection.
func (e *SSEEncoder) Encode(ev Event) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("agui: marshal %T: %w", ev, err)
	}
	if _, err := fmt.Fprintf(e.w, "event: %s\ndata: %s\n\n", ev.EventType(), payload); err != nil {
		return fmt.Errorf("agui: write SSE frame: %w", err)
	}
	if err := e.w.Flush(); err != nil {
		return fmt.Errorf("agui: buffered flush: %w", err)
	}
	if e.flusher != nil {
		e.flusher.Flush()
	}
	return nil
}

// EncodeAll is a convenience for translator outputs (which return
// 0..N events per input). Stops on first error so the caller can
// short-circuit the underlying connection.
func (e *SSEEncoder) EncodeAll(events []Event) error {
	for _, ev := range events {
		if err := e.Encode(ev); err != nil {
			return err
		}
	}
	return nil
}
