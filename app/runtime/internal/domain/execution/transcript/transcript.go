// Package transcript defines the canonical execution transcript and the
// run-timeline boundary invariant used by history, rollback, and fork. The
// records are transport-neutral domain values; persistence and protocol
// projection are adapter concerns.
package transcript

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// --- run timeline (the rollback / fork boundary invariant) ---
//
// A session's runs form a wall-clock timeline: each turn opens with a ROOT run
// (a runs.start), optionally interleaved with subagent runs it spawns (carrying
// a SpawnedByItemID). A run's resume continuations are NOT separate nodes — they
// share the run's stable id and collapse into its one record. sessions.rollback
// and sessions.fork both cut this timeline at a run boundary — keeping a run
// (with its subagents) and dropping/copying from the next root on. That boundary
// math is a domain invariant of the run log, so it lives here (wire-free) rather
// than in the protocol adapter; adapters only map these canonical values and
// sentinels to their external representation. See
// doc/EXECUTION_CENTERED_ARCHITECTURE.md.

// Boundary-resolution errors. The adapter maps them to protocol errors.
var (
	// ErrRunNotFound means the boundary run id isn't in the timeline.
	ErrRunNotFound = errors.New("run not found in timeline")
	// ErrNotRoot means a root-only boundary (rollback) addressed a subagent run.
	// Fork is lax and never returns this.
	ErrNotRoot = errors.New("run is not a root run")
)

// RunNode is one run's position in a session's timeline.
type RunNode struct {
	ID              string
	SpawnedByItemID string    // non-empty: a subagent run
	CreatedAt       time.Time // wall-clock turn order
	Mark            int       // chat history message watermark; -1 when unknown
}

// IsRoot reports whether the run opens a turn (a runs.start) rather than a
// subagent run.
func (n RunNode) IsRoot() bool { return n.SpawnedByItemID == "" }

// Timeline is the domain view of a session's run log. It owns boundary math for
// fork/rollback: callers lift wire/store records into [RunNode] values, then
// ask the timeline where the inclusive-keep split lands.
type Timeline []RunNode

func TimelineFromRuns(runs []Run) Timeline {
	nodes := make(Timeline, len(runs))
	for i, run := range runs {
		nodes[i] = RunNode{
			ID: run.ID, SpawnedByItemID: run.SpawnedByItemID,
			CreatedAt: run.CreatedAt, Mark: run.MessageMark,
		}
	}
	return nodes
}

func OpeningInputs(items []Item) map[string][]ContentBlock {
	out := make(map[string][]ContentBlock)
	for _, item := range items {
		if item.Kind != UserMessage {
			continue
		}
		if _, exists := out[item.RunID]; exists {
			continue
		}
		out[item.RunID] = slices.Clone(item.Content)
	}
	return out
}

// Boundary is the inclusive-keep split of a timeline at a run:
//
//   - KeepMark: the watermark to keep — the Mark of the last kept run (the last
//     node before the first root run after it), so the run and its subagents are
//     kept. -1 when that watermark is unknown (in-flight / pre-watermark), which
//     the caller clamps.
//   - Dropped: the runs at/after the boundary, in timeline order — the next root
//     run plus everything after it (its subagent runs) included.
//   - BoundaryTime: the first dropped root run's CreatedAt — the cut-off that
//     attributes subagent child sessions to dropped runs. Zero when nothing is
//     dropped (or the whole timeline is dropped).
type Boundary struct {
	KeepMark     int
	Dropped      []RunNode
	BoundaryTime time.Time
}

// DroppedRunIDs returns the dropped timeline node ids in boundary order.
func (b Boundary) DroppedRunIDs() []string {
	ids := make([]string, len(b.Dropped))
	for i, node := range b.Dropped {
		ids[i] = node.ID
	}
	return ids
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
			// Keep through t[k-1] (runID + its subagents); drop from the next
			// root on.
			return Boundary{
				KeepMark:     t[k-1].Mark,
				Dropped:      slices.Clone(t[k:]),
				BoundaryTime: t[k].CreatedAt,
			}, nil
		}
	}
	// No root run after runID — its turn (incl. subagents) is the latest, so
	// there is nothing to drop / everything up to it is copied.
	return Boundary{KeepMark: t[len(t)-1].Mark}, nil
}
