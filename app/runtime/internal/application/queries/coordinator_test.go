package queries

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/component/keyset"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

type fakeTranscript struct {
	items []transcript.SequencedItem

	session       string
	afterSequence int64
	limit         int
}

func (f *fakeTranscript) PageItems(_ context.Context, sessionID string, afterSequence int64, limit int) ([]transcript.SequencedItem, error) {
	f.session = sessionID
	f.afterSequence = afterSequence
	f.limit = limit
	var out []transcript.SequencedItem
	for _, entry := range f.items {
		if entry.Sequence <= afterSequence {
			continue
		}
		if limit > 0 && len(out) == limit {
			break
		}
		out = append(out, entry)
	}
	return out, nil
}

// fakeRuns is the Run record the item page threads its items against, and the
// running projection the run page seeks through. Both seek the way the store does:
// a (0, "") anchor is no anchor — the first page — and anything else is strictly
// past the last row of the page before it.
type fakeRuns struct {
	runs    []transcript.Run
	running []transcript.Run

	session        string
	afterStartedAt int64
	afterRunID     string
	limit          int
}

func (f *fakeRuns) ListRunning(_ context.Context, sessionID string, afterStartedAt int64, afterRunID string, limit int) ([]transcript.Run, error) {
	f.session, f.afterStartedAt, f.afterRunID, f.limit = sessionID, afterStartedAt, afterRunID, limit
	var out []transcript.Run
	for _, run := range f.running {
		if !seeksPast(run.CreatedAt.UnixNano(), run.ID, afterStartedAt, afterRunID) {
			continue
		}
		if limit > 0 && len(out) == limit {
			break
		}
		out = append(out, run)
	}
	return out, nil
}

func (f *fakeRuns) ListRuns(_ context.Context, sessionID string) ([]transcript.Run, error) {
	f.session = sessionID
	return f.runs, nil
}

type fakeInterrupts struct {
	pending []interrupts.Pending

	session        string
	afterCreatedAt int64
	afterRunID     string
	limit          int
}

func (f *fakeInterrupts) ListPage(_ context.Context, sessionID string, afterCreatedAt int64, afterRunID string, limit int) ([]interrupts.Pending, error) {
	f.session, f.afterCreatedAt, f.afterRunID, f.limit = sessionID, afterCreatedAt, afterRunID, limit
	var out []interrupts.Pending
	for _, pending := range f.pending {
		if !seeksPast(pending.CreatedAt.UnixNano(), pending.RunID, afterCreatedAt, afterRunID) {
			continue
		}
		if limit > 0 && len(out) == limit {
			break
		}
		out = append(out, pending)
	}
	return out, nil
}

// seeksPast is the store's own seek predicate: order by (timestamp, id), and treat
// a zero pair as the first page rather than as a position before every row.
func seeksPast(at int64, id string, afterAt int64, afterID string) bool {
	if afterAt == 0 && afterID == "" {
		return true
	}
	return at > afterAt || (at == afterAt && id > afterID)
}

func sequencedItems(count int) []transcript.SequencedItem {
	out := make([]transcript.SequencedItem, 0, count)
	for i := 1; i <= count; i++ {
		out = append(out, transcript.SequencedItem{
			Sequence: int64(i),
			Item:     transcript.Item{ID: "it_" + strconv.Itoa(i)},
		})
	}
	return out
}

func TestCoordinatorReadsDelegateToProjections(t *testing.T) {
	ctx := context.Background()
	tx := &fakeTranscript{items: sequencedItems(1)}
	runs := &fakeRuns{runs: []transcript.Run{{ID: "run_1"}}}
	ints := &fakeInterrupts{pending: []interrupts.Pending{{RunID: "run_1"}}}
	c := New(Dependencies{Transcript: tx, Interrupts: ints, Runs: runs})

	page, err := c.ListItemPage(ctx, "ses_1", "", 0)
	if err != nil || len(page.Items) != 1 || len(page.Runs) != 1 || tx.session != "ses_1" || runs.session != "ses_1" {
		t.Fatalf("ListItemPage items=%d runs=%d session=%q/%q err=%v", len(page.Items), len(page.Runs), tx.session, runs.session, err)
	}

	pending, err := c.ListPendingInterruptPage(ctx, "ses_2", "", 0)
	if err != nil || len(pending.Rows) != 1 || ints.session != "ses_2" {
		t.Fatalf("ListPendingInterruptPage pending=%d session=%q err=%v", len(pending.Rows), ints.session, err)
	}
}

// The page is cut by the query, not after the fact: the read asks for exactly one
// row more than it will return, which is both how "there is more" is known and
// what keeps a long session's history out of memory.
// It is also items.list's fixed-order and next-page-direction fixture.
func TestListItemPageBoundsTheQueryAndSeeksPastTheAnchor(t *testing.T) {
	ctx := context.Background()
	tx := &fakeTranscript{items: sequencedItems(5)}
	c := New(Dependencies{Transcript: tx, Runs: &fakeRuns{}})

	first, err := c.ListItemPage(ctx, "ses_1", "", 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if tx.limit != 3 {
		t.Fatalf("store asked for %d rows, want the page plus one", tx.limit)
	}
	if len(first.Items) != 2 || first.Items[0].ID != "it_1" || first.NextCursor == "" {
		t.Fatalf("first page = %+v, want two items and a cursor", first)
	}

	second, err := c.ListItemPage(ctx, "ses_1", first.NextCursor, 2)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if tx.afterSequence != 2 {
		t.Fatalf("second page sought past %d, want the first page's last position", tx.afterSequence)
	}
	if len(second.Items) != 2 || second.Items[0].ID != "it_3" {
		t.Fatalf("second page = %+v, want it_3 onward", second)
	}

	last, err := c.ListItemPage(ctx, "ses_1", second.NextCursor, 2)
	if err != nil {
		t.Fatalf("last page: %v", err)
	}
	if len(last.Items) != 1 || last.NextCursor != "" {
		t.Fatalf("last page = %+v, want the tail and no cursor", last)
	}
}

// A cursor from another session would page this one against positions it never
// enumerated. Restarting from the top instead of refusing would hand the client
// rows it had already read, as if they were new. It is items.list's cursor-binding
// fixture.
func TestListItemPageRefusesAForeignCursor(t *testing.T) {
	ctx := context.Background()
	tx := &fakeTranscript{items: sequencedItems(5)}
	c := New(Dependencies{Transcript: tx, Runs: &fakeRuns{}})

	other, err := c.ListItemPage(ctx, "ses_other", "", 2)
	if err != nil {
		t.Fatalf("other session page: %v", err)
	}
	if _, err := c.ListItemPage(ctx, "ses_1", other.NextCursor, 2); !errors.Is(err, keyset.ErrInvalidCursor) {
		t.Fatalf("cross-session cursor err = %v, want ErrInvalidCursor", err)
	}
	if _, err := c.ListItemPage(ctx, "ses_1", "not-a-cursor", 2); !errors.Is(err, keyset.ErrInvalidCursor) {
		t.Fatalf("damaged cursor err = %v, want ErrInvalidCursor", err)
	}
}

func TestListItemPageRejectsANegativeLimit(t *testing.T) {
	c := New(Dependencies{Transcript: &fakeTranscript{items: sequencedItems(1)}, Runs: &fakeRuns{}})
	if _, err := c.ListItemPage(context.Background(), "ses_1", "", -1); err == nil {
		t.Fatal("negative limit returned no error")
	}
}

// running builds a page of admitted runs one nanosecond apart, so the order is
// total and the seek has something unambiguous to land past.
func running(sessionID string, ids ...string) []transcript.Run {
	out := make([]transcript.Run, 0, len(ids))
	for i, id := range ids {
		out = append(out, transcript.Run{
			ID: id, SessionID: sessionID, State: execution.Running,
			CreatedAt: time.Unix(0, int64(i+1)).UTC(),
		})
	}
	return out
}

func parked(sessionID string, ids ...string) []interrupts.Pending {
	out := make([]interrupts.Pending, 0, len(ids))
	for i, id := range ids {
		out = append(out, interrupts.Pending{
			RunID: id, SessionID: sessionID, CreatedAt: time.Unix(0, int64(i+1)).UTC(),
		})
	}
	return out
}

// TestListRunningRunsPagesInAdmissionOrder covers runs.list's query properties: the order is fixed (admission, tie-broken by id), the next page seeks
// strictly past the last row rather than re-reading it, and "there is more" is only
// claimed when the over-fetch found it.
func TestListRunningRunsPagesInAdmissionOrder(t *testing.T) {
	ctx := context.Background()
	runs := &fakeRuns{running: running("ses_1", "run_1", "run_2", "run_3")}
	c := New(Dependencies{Transcript: &fakeTranscript{}, Runs: runs})

	first, err := c.ListRunningRuns(ctx, "ses_1", "", 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if runs.limit != 3 {
		t.Fatalf("store asked for %d rows, want the page plus one", runs.limit)
	}
	if len(first.Rows) != 2 || first.Rows[0].ID != "run_1" || first.NextCursor == "" {
		t.Fatalf("first page = %+v, want two runs and a cursor", first.Rows)
	}

	second, err := c.ListRunningRuns(ctx, "ses_1", first.NextCursor, 2)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if runs.afterRunID != "run_2" {
		t.Fatalf("second page sought past %q, want the first page's last row", runs.afterRunID)
	}
	if len(second.Rows) != 1 || second.Rows[0].ID != "run_3" || second.NextCursor != "" {
		t.Fatalf("second page = %+v, want the tail and no cursor", second.Rows)
	}
}

// TestListRunningRunsRefusesACursorFromAnotherQuery is runs.list's half of the
// cursor binding: an anchor is only meaningful against the ordering that produced
// it. Continuing from a foreign one silently pages against positions this query
// never enumerated — the client is handed rows it already has, or none at all, with
// nothing to say why.
func TestListRunningRunsRefusesACursorFromAnotherQuery(t *testing.T) {
	ctx := context.Background()
	c := New(Dependencies{
		Transcript: &fakeTranscript{items: sequencedItems(5)},
		Runs:       &fakeRuns{running: running("ses_1", "run_1", "run_2", "run_3")},
		Interrupts: &fakeInterrupts{pending: parked("ses_1", "run_1", "run_2", "run_3")},
	})

	otherSession, err := c.ListRunningRuns(ctx, "ses_other", "", 2)
	if err != nil {
		t.Fatalf("other session page: %v", err)
	}
	if _, err := c.ListRunningRuns(ctx, "ses_1", otherSession.NextCursor, 2); !errors.Is(err, keyset.ErrInvalidCursor) {
		t.Fatalf("cross-session cursor err = %v, want ErrInvalidCursor", err)
	}

	// The interrupt page is scoped the same way and ordered by a timestamp too, so
	// only the query namespace tells the two apart.
	interruptPage, err := c.ListPendingInterruptPage(ctx, "ses_1", "", 2)
	if err != nil {
		t.Fatalf("interrupt page: %v", err)
	}
	if _, err := c.ListRunningRuns(ctx, "ses_1", interruptPage.NextCursor, 2); !errors.Is(err, keyset.ErrInvalidCursor) {
		t.Fatalf("cross-query cursor err = %v, want ErrInvalidCursor", err)
	}

	itemPage, err := c.ListItemPage(ctx, "ses_1", "", 2)
	if err != nil {
		t.Fatalf("item page: %v", err)
	}
	if _, err := c.ListRunningRuns(ctx, "ses_1", itemPage.NextCursor, 2); !errors.Is(err, keyset.ErrInvalidCursor) {
		t.Fatalf("item cursor on the run page err = %v, want ErrInvalidCursor", err)
	}
}

// TestListPendingInterruptPagePagesOldestFirst is the same three properties for
// runs.listOpenInterrupts, whose order the contract fixes as oldest first: a
// resumable run that keeps sinking below the page boundary is one nobody answers.
func TestListPendingInterruptPagePagesOldestFirst(t *testing.T) {
	ctx := context.Background()
	ints := &fakeInterrupts{pending: parked("ses_1", "run_1", "run_2", "run_3")}
	c := New(Dependencies{
		Transcript: &fakeTranscript{},
		Runs:       &fakeRuns{running: running("ses_1", "run_1", "run_2", "run_3")},
		Interrupts: ints,
	})

	first, err := c.ListPendingInterruptPage(ctx, "ses_1", "", 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if ints.limit != 3 {
		t.Fatalf("store asked for %d rows, want the page plus one", ints.limit)
	}
	if len(first.Rows) != 2 || first.Rows[0].RunID != "run_1" || first.NextCursor == "" {
		t.Fatalf("first page = %+v, want two pending sets and a cursor", first.Rows)
	}

	second, err := c.ListPendingInterruptPage(ctx, "ses_1", first.NextCursor, 2)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if ints.afterRunID != "run_2" {
		t.Fatalf("second page sought past %q, want the first page's last row", ints.afterRunID)
	}
	if len(second.Rows) != 1 || second.Rows[0].RunID != "run_3" || second.NextCursor != "" {
		t.Fatalf("second page = %+v, want the tail and no cursor", second.Rows)
	}
	if _, err := c.ListPendingInterruptPage(ctx, "ses_1", first.NextCursor+"x", 2); !errors.Is(err, keyset.ErrInvalidCursor) {
		t.Fatalf("damaged cursor err = %v, want ErrInvalidCursor", err)
	}

	// The run page is scoped and ordered the same way, so only the query namespace
	// separates the two — in both directions.
	runPage, err := c.ListRunningRuns(ctx, "ses_1", "", 2)
	if err != nil {
		t.Fatalf("run page: %v", err)
	}
	if _, err := c.ListPendingInterruptPage(ctx, "ses_1", runPage.NextCursor, 2); !errors.Is(err, keyset.ErrInvalidCursor) {
		t.Fatalf("run cursor on the interrupt page err = %v, want ErrInvalidCursor", err)
	}
}
