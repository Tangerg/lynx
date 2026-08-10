package sqlite_test

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/sqlite"
)

func TestToolInvocationJournalAllowsOneLogicalCallAcrossContinuationSegments(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewToolInvocationStore(db)
	startedAt := time.Now().UTC()
	segments := []string{"segment_before_wait", "segment_after_answer"}
	for index, segmentID := range segments {
		if err := store.StartToolInvocation(
			t.Context(), "session_1", "run_1", segmentID,
			"logical_call_1", "item_1", startedAt,
		); err != nil {
			t.Fatalf("start %s: %v", segmentID, err)
		}
		finish := store.CompleteToolInvocation
		if index == 0 {
			finish = store.MarkToolInvocationIncomplete
		}
		if err := finish(
			t.Context(), "session_1", "run_1", segmentID, "logical_call_1", "item_1",
			startedAt, startedAt.Add(time.Millisecond),
		); err != nil {
			t.Fatalf("complete %s: %v", segmentID, err)
		}
	}
	var rows int
	if err := db.QueryRowContext(t.Context(), `
		SELECT count(*) FROM tool_invocations
		 WHERE call_id = ? AND item_id = ? AND state IN ('incomplete', 'completed')
	`, "logical_call_1", "item_1").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Fatalf("journal rows = %d, want 2 segment-owned attempts", rows)
	}
}
