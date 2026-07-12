package transcript_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func root(id string, atUnix int64, mark int) transcript.RunNode {
	return transcript.RunNode{ID: id, CreatedAt: time.Unix(atUnix, 0).UTC(), Mark: mark}
}

func sub(id, spawnedByItem string, atUnix int64, mark int) transcript.RunNode {
	return transcript.RunNode{ID: id, SpawnedByItemID: spawnedByItem, CreatedAt: time.Unix(atUnix, 0).UTC(), Mark: mark}
}

func runIDs(ns []transcript.RunNode) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = n.ID
	}
	return out
}

// TestBoundaryAt audits the inclusive-keep split: a kept root keeps its own
// subagents (so the watermark is the last kept node's, not the root's), the
// drop set is everything from the next root on, drop-all keeps nothing, and a
// non-root / unknown target errors under requireRoot. Input is given out of
// order to also exercise the internal CreatedAt sort.
func TestBoundaryAt(t *testing.T) {
	// Wall-clock order: R1 @1 mark2 → S1 (subagent of R1) @2 mark4 → R2 @3 mark6
	// → R3 @4 mark9. Deliberately shuffled to prove BoundaryAt sorts.
	nodes := []transcript.RunNode{
		root("R3", 4, 9),
		root("R1", 1, 2),
		root("R2", 3, 6),
		sub("S1", "item_r1", 2, 4),
	}
	timeline := transcript.Timeline(nodes)

	// Keep through R1 inclusive → keep R1+S1 (watermark 4, the last kept node),
	// drop R2+R3, boundary at R2's time.
	b, err := timeline.BoundaryAt("R1", true)
	if err != nil {
		t.Fatalf("R1: %v", err)
	}
	if b.KeepMark != 4 || len(b.Dropped) != 2 || b.Dropped[0].ID != "R2" || !b.BoundaryTime.Equal(time.Unix(3, 0).UTC()) {
		t.Fatalf("R1 split = keep%d drop%v boundary%v, want keep4 [R2 R3] @3", b.KeepMark, runIDs(b.Dropped), b.BoundaryTime.Unix())
	}
	if got := b.DroppedRunIDs(); len(got) != 2 || got[0] != "R2" || got[1] != "R3" {
		t.Fatalf("DroppedRunIDs = %v, want [R2 R3]", got)
	}

	// Keep through R2 → watermark 6, drop only R3.
	if b, _ := timeline.BoundaryAt("R2", true); b.KeepMark != 6 || len(b.Dropped) != 1 || b.Dropped[0].ID != "R3" {
		t.Fatalf("R2 split = keep%d drop%v, want keep6 [R3]", b.KeepMark, runIDs(b.Dropped))
	}

	// Keep through the latest root → nothing to drop.
	if b, _ := timeline.BoundaryAt("R3", true); len(b.Dropped) != 0 {
		t.Fatalf("R3 drop = %v, want none", runIDs(b.Dropped))
	}

	// Drop everything (empty target) → keep 0, drop all.
	if b, _ := timeline.BoundaryAt("", true); b.KeepMark != 0 || len(b.Dropped) != 4 || !b.BoundaryTime.IsZero() {
		t.Fatalf("drop-all = keep%d drop%d boundary%v, want keep0 drop4 zero", b.KeepMark, len(b.Dropped), b.BoundaryTime)
	}

	// A subagent target is not a root → ErrNotRoot (rollback's requireRoot).
	if _, err := timeline.BoundaryAt("S1", true); !errors.Is(err, transcript.ErrNotRoot) {
		t.Fatalf("S1 err = %v, want ErrNotRoot", err)
	}
	// Unknown target → ErrRunNotFound.
	if _, err := timeline.BoundaryAt("ghost", true); !errors.Is(err, transcript.ErrRunNotFound) {
		t.Fatalf("ghost err = %v, want ErrRunNotFound", err)
	}
	// Fork is lax: a subagent target is allowed (requireRoot=false).
	if _, err := timeline.BoundaryAt("S1", false); err != nil {
		t.Fatalf("S1 lax err = %v, want nil", err)
	}

	// BoundaryAt must not mutate the caller's slice order.
	if nodes[0].ID != "R3" {
		t.Fatalf("input slice was reordered: %v", runIDs(nodes))
	}
}
