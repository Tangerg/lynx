package lifecycle

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

func TestResolveRollbackBoundary(t *testing.T) {
	t0 := time.Unix(10, 0)
	nodes := []transcript.RunNode{
		{ID: "run_3", CreatedAt: t0.Add(3 * time.Second), Mark: 9},
		{ID: "run_1", CreatedAt: t0, Mark: 3},
		{ID: "run_2", CreatedAt: t0.Add(time.Second), Mark: 6},
		{ID: "run_2_resume", ParentRunID: "run_2", CreatedAt: t0.Add(2 * time.Second), Mark: 7},
	}

	b, err := ResolveRollbackBoundary(nodes, "run_1")
	if err != nil {
		t.Fatalf("resolve rollback boundary: %v", err)
	}
	if b.KeepMark != 3 {
		t.Fatalf("KeepMark = %d, want 3", b.KeepMark)
	}
	wantDrop := []string{"run_2", "run_2_resume", "run_3"}
	if len(b.DropRunIDs) != len(wantDrop) {
		t.Fatalf("DropRunIDs = %v, want %v", b.DropRunIDs, wantDrop)
	}
	for i, want := range wantDrop {
		if b.DropRunIDs[i] != want {
			t.Fatalf("DropRunIDs = %v, want %v", b.DropRunIDs, wantDrop)
		}
	}
	if !b.BoundaryTime.Equal(t0.Add(time.Second)) {
		t.Fatalf("BoundaryTime = %v, want first dropped root time", b.BoundaryTime)
	}
}
