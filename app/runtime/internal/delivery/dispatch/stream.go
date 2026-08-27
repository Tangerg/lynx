package dispatch

import "github.com/Tangerg/scope/app/runtime/internal/delivery/transport"

// StreamFrame is one ready-to-write downstream notification on a streaming
// method's event stream. The dispatch produces these from domain events so
// every transport writes them identically. SSEID drives Last-Event-Id replay;
// "" marks an ephemeral frame (no replay) — e.g. all workspace events.
type StreamFrame struct {
	Notification transport.Message
	SSEID        string
}
