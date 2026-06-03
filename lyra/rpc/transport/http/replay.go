package http

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// replayWindow is how long an event stays in the per-stream ring
// buffer for Last-Event-Id resume (API.md §3.3 — "重放窗口：30s").
const replayWindow = 30 * time.Second

// streamRecord is one buffered notification waiting for replay.
type streamRecord struct {
	eventID string
	msg     transport.Message
	at      time.Time
}

// streamBuffer is a per-runId ring buffer keyed by eventId. Entries
// older than replayWindow drop on each append.
//
// API.md v4 §3.1: stream identifier is the resource id (runId /
// taskId), not a separate streamHandle.
type streamBuffer struct {
	mu    sync.Mutex
	items []streamRecord
}

// append adds one record and evicts entries past the window.
func (b *streamBuffer) append(r streamRecord) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.items = append(b.items, r)
	cutoff := time.Now().Add(-replayWindow)
	idx := 0
	for ; idx < len(b.items); idx++ {
		if b.items[idx].at.After(cutoff) {
			break
		}
	}
	if idx > 0 {
		b.items = b.items[idx:]
	}
}

// since returns every record with eventId strictly greater than the
// supplied lastID. Implementation uses a linear scan since the buffer
// is bounded by the 30s window — typically <100 entries even on
// busy streams.
func (b *streamBuffer) since(lastID string) []streamRecord {
	b.mu.Lock()
	defer b.mu.Unlock()
	if lastID == "" {
		out := make([]streamRecord, len(b.items))
		copy(out, b.items)
		return out
	}
	out := make([]streamRecord, 0, len(b.items))
	for _, rec := range b.items {
		if compareEventID(rec.eventID, lastID) > 0 {
			out = append(out, rec)
		}
	}
	return out
}

// streamRegistry tracks every active run's replay buffer, keyed by
// runId (API.md v4 §3.1).
type streamRegistry struct {
	mu      sync.Mutex
	streams map[string]*streamBuffer
}

func newStreamRegistry() *streamRegistry {
	return &streamRegistry{streams: map[string]*streamBuffer{}}
}

// open returns (or creates) the buffer for a runId.
func (r *streamRegistry) open(runID string) *streamBuffer {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.streams[runID]
	if !ok {
		b = &streamBuffer{}
		r.streams[runID] = b
	}
	return b
}

// compareEventID compares two eventIds numerically. Both are
// evt_<zero-padded-decimal> (TRANSPORT.md §9.1) from the server's global
// counter, so we strip the prefix and parse the decimal; a successful
// parse is the common case, with a defensive lexical fallback for
// malformed input (the fixed-width padding makes lexical order agree with
// numeric anyway).
func compareEventID(a, b string) int {
	an, aerr := strconv.ParseUint(strings.TrimPrefix(a, protocol.IDPrefixEvent), 10, 64)
	bn, berr := strconv.ParseUint(strings.TrimPrefix(b, protocol.IDPrefixEvent), 10, 64)
	if aerr == nil && berr == nil {
		switch {
		case an < bn:
			return -1
		case an > bn:
			return 1
		default:
			return 0
		}
	}
	// Either side wasn't a clean uint — fall back to lex compare to
	// avoid silently treating them as equal.
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
