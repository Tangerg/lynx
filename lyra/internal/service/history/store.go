// Package history defines the durable Item history — the authoritative
// completed-Item log a session's items.list is served from (API.md §7.4
// / §10.3). It is the protocol's "Item is the only history primitive"
// (§0.1) made persistent: the runtime records every completed Item and
// the RunRef it belongs to as a run streams, so history hydration after a
// restart returns exactly what the live stream emitted — same ids, same
// runId, same text — rather than reconstructing items from chat messages.
//
// The store is transport-neutral: Items and Runs are carried as opaque
// wire blobs (marshaled protocol.Item / protocol.RunRef) plus the few
// fields the store needs to order and group them, so this package depends
// on neither rpc/protocol nor any backend.
package history

import (
	"context"
	"encoding/json"
	"time"
)

// Item is one persisted completed Item. Blob is the marshaled wire Item;
// the lifted fields let the store order (by append) and group (by RunID)
// without parsing the blob.
type Item struct {
	SessionID string
	RunID     string
	ItemID    string
	CreatedAt time.Time
	Blob      json.RawMessage
}

// Run is one persisted RunRef, upserted by (SessionID, RunID) as a run
// starts (status running) and then finishes (status finished + outcome).
// Blob is the marshaled wire RunRef.
type Run struct {
	SessionID string
	RunID     string
	UpdatedAt time.Time
	Blob      json.RawMessage
}

// Store is the durable Item history. Implementations must be safe for
// concurrent use. Consumer-side abstraction: the runtime + RPC server
// depend on it; back it with [storage.FileHistoryStore] or the sqlite
// implementation.
type Store interface {
	// AppendItem records one completed Item. List returns items in
	// append order.
	AppendItem(ctx context.Context, it Item) error

	// PutRun records (or replaces) a RunRef keyed by (SessionID, RunID).
	PutRun(ctx context.Context, r Run) error

	// List returns sessionID's items (append order) plus the RunRefs
	// those items belong to (for run-tree reconstruction, §10.3).
	List(ctx context.Context, sessionID string) ([]Item, []Run, error)
}
