// Package transcript defines the canonical execution transcript and the
// Run-timeline boundary invariant used by history, rollback, and fork. The
// records are transport-neutral domain values; persistence and presentation
// are concerns outside this package.
package transcript

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
)

// --- run timeline (the rollback / fork boundary invariant) ---
//
// A Session's Runs form a wall-clock timeline: each root Run opens one execution,
// optionally followed by child Runs it spawns (carrying
// a SpawnedByItemID). A run's resume continuations are NOT separate nodes — they
// share the run's stable id and collapse into its one record. Rollback and fork
// both cut this timeline at a run boundary, so keeping a run
// (with its child Runs) and dropping/copying from the next root on. That boundary
// math is a domain invariant of the Run log; callers only map these canonical
// values and sentinels to their own representation.

// Boundary-resolution errors.
var (
	// ErrRunNotFound means the boundary run id isn't in the timeline.
	ErrRunNotFound = errors.New("run not found in timeline")
	// ErrNotRoot means a root-only boundary (rollback) addressed a child Run.
	// Fork is lax and never returns this.
	ErrNotRoot = errors.New("run is not a root run")
)

// RunNode is one run's position in a session's timeline.
type RunNode struct {
	ID              string
	SpawnedByItemID string    // non-empty: a child Run
	CreatedAt       time.Time // wall-clock Run order
	MessageMark     int       // conversation message watermark; -1 when unknown
}

// IsRoot reports whether the Run opens an execution rather than representing a
// delegated child.
func (n RunNode) IsRoot() bool { return n.SpawnedByItemID == "" }

// Timeline is the domain view of a session's run log. It owns boundary math for
// fork/rollback: callers lift source records into [RunNode] values, then
// ask the timeline where the inclusive-keep split lands.
type Timeline []RunNode

// TimelineFromRuns projects durable Runs into their boundary-resolution value.
func TimelineFromRuns(runs []run.Run) Timeline {
	nodes := make(Timeline, len(runs))
	for i, current := range runs {
		lineage := current.Lineage()
		nodes[i] = RunNode{
			ID: current.ID(), SpawnedByItemID: lineage.SpawnedByItemID,
			CreatedAt: current.CreatedAt(), MessageMark: current.MessageMark(),
		}
	}
	return nodes
}

// OpeningUserMessagesByRun returns the first user message recorded for each Run.
func OpeningUserMessagesByRun(items []Item) map[string][]ContentBlock {
	out := make(map[string][]ContentBlock)
	for _, item := range items {
		if item.Kind() != UserMessage {
			continue
		}
		if _, exists := out[item.RunID()]; exists {
			continue
		}
		out[item.RunID()] = item.Content()
	}
	return out
}

// Boundary is the inclusive-keep split of a timeline at a run:
//
//   - KeepMessageMark: the watermark to keep — the MessageMark of the last kept run (the last
//     node before the first root run after it), so the run and its child Runs are
//     kept. -1 when that watermark is unknown (in-flight / pre-watermark), which
//     the caller clamps.
//   - KeepRunID: the run that watermark belongs to — the boundary's identity for
//     the Session Plan recorded per run, which unlike the message log has
//     no watermark of its own to seek to. It is deliberately the SAME node
//     KeepMessageMark comes from: two answers to "where does this boundary sit" is one
//     answer too many. Empty when nothing is kept (the whole timeline is dropped),
//     which is a boundary before any run wrote anything.
//   - Dropped: the runs at/after the boundary, in timeline order — the next root
//     run plus everything after it (its child Runs) included.
//   - BoundaryTime: the first dropped root run's CreatedAt — the cut-off that
//     attributes child sessions to dropped Runs. Zero when nothing is
//     dropped (or the whole timeline is dropped).
type Boundary struct {
	KeepMessageMark int
	KeepRunID       string
	Dropped         []RunNode
	BoundaryTime    time.Time
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
// drops every run (KeepMessageMark 0 — clear to empty). requireRoot rejects a non-root
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
			// Keep through t[k-1] (runID + its child Runs); drop from the next
			// root on.
			return Boundary{
				KeepMessageMark: t[k-1].MessageMark,
				KeepRunID:       t[k-1].ID,
				Dropped:         slices.Clone(t[k:]),
				BoundaryTime:    t[k].CreatedAt,
			}, nil
		}
	}
	// No root Run after runID — its tree is the latest, so
	// there is nothing to drop / everything up to it is copied.
	return Boundary{KeepMessageMark: t[len(t)-1].MessageMark, KeepRunID: t[len(t)-1].ID}, nil
}
