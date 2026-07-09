// Package transcript defines the durable Item history — the authoritative
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
// on neither delivery/protocol nor any backend.
package transcript

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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
	// Mark is the per-run chat history message watermark — the message count
	// captured when the run finished (post-compaction). sessions.rollback /
	// fork{fromRunId} truncate the message log to it. -1 means unknown: a run
	// still in flight, or one persisted before this field existed.
	Mark int
}

// Store is the durable Item history. Implementations must be safe for
// concurrent use. Consumer-side abstraction: the runtime + RPC server
// depend on it; back it with the sqlite TranscriptStore
// (internal/infra/storage/sqlite).
type Store interface {
	// AppendItem records one completed Item. List returns items in
	// append order.
	AppendItem(ctx context.Context, it Item) error

	// PutRun records (or replaces) a RunRef keyed by (SessionID, RunID).
	PutRun(ctx context.Context, r Run) error

	// List returns sessionID's items (append order) plus the RunRefs
	// those items belong to (for run-tree reconstruction, §10.3).
	List(ctx context.Context, sessionID string) ([]Item, []Run, error)

	// ListRuns returns just sessionID's RunRefs (no items) — the cheap path for
	// consumers that only need the run records (e.g. usage aggregation), so they
	// don't load every item blob just to discard it.
	ListRuns(ctx context.Context, sessionID string) ([]Run, error)

	// DeleteRun removes one run's record and its items (sessions.rollback drops
	// runs after the kept boundary). Idempotent — unknown run is not an error.
	DeleteRun(ctx context.Context, sessionID, runID string) error

	// DeleteSession removes every item + run for a session (sessions.rollback
	// purges the subagent child sessions a dropped run spawned). Idempotent.
	DeleteSession(ctx context.Context, sessionID string) error
}

// --- run timeline (the rollback / fork boundary invariant) ---
//
// A session's runs form a wall-clock timeline: each turn opens with a ROOT run
// (a runs.start), optionally followed by continuations (runs.resume, carrying a
// ParentRunID) and subagent runs (carrying a SpawnedByItemID). sessions.rollback
// and sessions.fork both cut this timeline at a run boundary — keeping a run's
// whole continuation chain and dropping/copying from the next root on. That
// boundary math is a domain invariant of the run log, so it lives here (wire-
// free) rather than in the protocol adapter; the adapter only lifts the
// structured fields out of the opaque Run.Blob and maps these sentinels to wire
// errors. See doc/GREENFIELD_ARCHITECTURE.md (F1).

// Boundary-resolution errors. The adapter maps them to protocol errors.
var (
	// ErrRunNotFound means the boundary run id isn't in the timeline.
	ErrRunNotFound = errors.New("run not found in timeline")
	// ErrNotRoot means a root-only boundary (rollback) addressed a continuation
	// or subagent run. Fork is lax and never returns this.
	ErrNotRoot = errors.New("run is not a root run")
)

// RunNode is one run's position in a session's timeline — the structured fields
// the boundary computation reasons over, lifted out of the opaque [Run.Blob] by
// the caller (which owns wire parsing). Wire-free by design.
type RunNode struct {
	ID              string
	ParentRunID     string    // non-empty: a resume continuation
	SpawnedByItemID string    // non-empty: a subagent run
	CreatedAt       time.Time // wall-clock turn order
	Mark            int       // chat history message watermark; -1 when unknown
}

// IsRoot reports whether the run opens a turn (a runs.start) rather than a
// continuation (runs.resume) or a subagent run.
func (n RunNode) IsRoot() bool { return n.ParentRunID == "" && n.SpawnedByItemID == "" }

// Timeline is the domain view of a session's run log. It owns boundary math for
// fork/rollback: callers lift wire/store records into [RunNode] values, then
// ask the timeline where the inclusive-keep split lands.
type Timeline []RunNode

// Boundary is the inclusive-keep split of a timeline at a run:
//
//   - KeepMark: the watermark to keep — the Mark of the kept run's chain
//     terminal (the last run before the first root run after it), so the run's
//     own continuation chain is kept. -1 when that watermark is unknown
//     (in-flight / pre-watermark), which the caller clamps.
//   - Dropped: the runs at/after the boundary, in timeline order — the next root
//     run plus everything after it (continuations, subagent runs) included.
//   - BoundaryTime: the first dropped root run's CreatedAt — the cut-off that
//     attributes subagent child sessions to dropped runs. Zero when nothing is
//     dropped (or the whole timeline is dropped).
type Boundary struct {
	KeepMark     int
	Dropped      []RunNode
	BoundaryTime time.Time
}

// BoundaryAt computes the inclusive-keep split of this timeline at runID. It
// orders a copy by CreatedAt and leaves the timeline untouched. runID==""
// drops every run (KeepMark 0 — clear to empty). requireRoot rejects a non-root
// runID with [ErrNotRoot] (rollback addresses root runs only; fork passes
// false). An unknown runID is [ErrRunNotFound].
func (tl Timeline) BoundaryAt(runID string, requireRoot bool) (Boundary, error) {
	t := slices.Clone([]RunNode(tl))
	slices.SortStableFunc(t, func(a, b RunNode) int { return a.CreatedAt.Compare(b.CreatedAt) })

	if runID == "" {
		return Boundary{Dropped: t}, nil
	}
	idx := slices.IndexFunc(t, func(n RunNode) bool { return n.ID == runID })
	if idx < 0 {
		return Boundary{}, ErrRunNotFound
	}
	if requireRoot && !t[idx].IsRoot() {
		return Boundary{}, fmt.Errorf("%w: %q", ErrNotRoot, runID)
	}
	for k := idx + 1; k < len(t); k++ {
		if t[k].IsRoot() {
			// Keep through t[k-1] (runID's chain terminal); drop from the next
			// root on.
			return Boundary{
				KeepMark:     t[k-1].Mark,
				Dropped:      slices.Clone(t[k:]),
				BoundaryTime: t[k].CreatedAt,
			}, nil
		}
	}
	// No root run after runID — its turn (incl. continuations) is the latest, so
	// there is nothing to drop / everything up to it is copied.
	return Boundary{KeepMark: t[len(t)-1].Mark}, nil
}
