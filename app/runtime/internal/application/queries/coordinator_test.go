package queries

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/component/keyset"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

type fakeTranscript struct {
	items []transcript.SequencedItem

	session       string
	run           string
	order         transcript.SequenceOrder
	afterSequence int64
	limit         int
}

func (f *fakeTranscript) PageSessionItems(_ context.Context, sessionID string, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	f.session = sessionID
	return f.page(order, fromSequence, limit, func(transcript.Item) bool { return true })
}

func (f *fakeTranscript) PageRunItems(_ context.Context, runID string, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	f.run = runID
	return f.page(order, fromSequence, limit, func(item transcript.Item) bool { return item.RunID == runID })
}

// page seeks the way the store does: zero is no anchor in either direction, and
// newest-first is the same sequence read from the other end.
func (f *fakeTranscript) page(order transcript.SequenceOrder, fromSequence int64, limit int, keep func(transcript.Item) bool) ([]transcript.SequencedItem, error) {
	f.order, f.afterSequence, f.limit = order, fromSequence, limit
	rows := slices.Clone(f.items)
	if order == transcript.NewestFirst {
		slices.Reverse(rows)
	}
	var out []transcript.SequencedItem
	for _, entry := range rows {
		if !keep(entry.Item) {
			continue
		}
		if fromSequence > 0 {
			if order == transcript.NewestFirst && entry.Sequence >= fromSequence {
				continue
			}
			if order == transcript.OldestFirst && entry.Sequence <= fromSequence {
				continue
			}
		}
		if limit > 0 && len(out) == limit {
			break
		}
		out = append(out, entry)
	}
	return out, nil
}

// fakeSessions answers only whether a session exists. Every session an item test
// names exists unless it is listed as missing.
type fakeSessions struct{ missing []string }

func (f *fakeSessions) Exists(_ context.Context, sessionID string) (bool, error) {
	return !slices.Contains(f.missing, sessionID), nil
}

// fakeRuns is the Run record the item page threads its items against, and the
// durable history the run page seeks through. Both seek the way the store does:
// an empty id is no anchor — the first page — and anything else is strictly past
// the last row of the page before it, which for the history means EARLIER.
type fakeRuns struct {
	runs []transcript.Run
	// history is newest first, the order the store returns.
	history []transcript.Run

	session         string
	requested       []string
	statuses        []execution.RunStatus
	beforeCreatedAt int64
	beforeRunID     string
	limit           int
}

func (f *fakeRuns) Run(_ context.Context, runID string) (transcript.Run, bool, error) {
	for _, run := range f.history {
		if run.ID == runID {
			return run, true, nil
		}
	}
	return transcript.Run{}, false, nil
}

func (f *fakeRuns) PageRuns(_ context.Context, sessionID string, statuses []execution.RunStatus, beforeCreatedAt int64, beforeRunID string, limit int) ([]transcript.Run, error) {
	f.session, f.statuses = sessionID, statuses
	f.beforeCreatedAt, f.beforeRunID, f.limit = beforeCreatedAt, beforeRunID, limit
	var out []transcript.Run
	for _, run := range f.history {
		if !seeksBefore(run.CreatedAt.UnixNano(), run.ID, beforeCreatedAt, beforeRunID) {
			continue
		}
		if len(statuses) > 0 && !slices.Contains(statuses, run.State.Status()) {
			continue
		}
		if limit > 0 && len(out) == limit {
			break
		}
		out = append(out, run)
	}
	return out, nil
}

func (f *fakeRuns) RunsByID(_ context.Context, runIDs []string) ([]transcript.Run, error) {
	f.requested = runIDs
	var out []transcript.Run
	for _, run := range f.runs {
		if slices.Contains(runIDs, run.ID) {
			out = append(out, run)
		}
	}
	return out, nil
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
		if !seeksPast(pending.CreatedAt.UnixNano(), pending.RootRunID, afterCreatedAt, afterRunID) {
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

// seeksBefore is the same rule for a newest-first read, where continuing means
// going back in time. An empty id is the first page: unlike a timestamp, it cannot
// be confused with a position.
func seeksBefore(at int64, id string, beforeAt int64, beforeID string) bool {
	if beforeID == "" {
		return true
	}
	return at < beforeAt || (at == beforeAt && id < beforeID)
}

// sequencedItems builds a session's items, every one belonging to run_1, so a page
// of them has a run to be threaded onto.
func sequencedItems(count int) []transcript.SequencedItem {
	out := make([]transcript.SequencedItem, 0, count)
	for i := 1; i <= count; i++ {
		out = append(out, transcript.SequencedItem{
			Sequence: int64(i),
			Item:     transcript.Item{ID: "it_" + strconv.Itoa(i), RunID: "run_1"},
		})
	}
	return out
}

func TestCoordinatorReadsDelegateToProjections(t *testing.T) {
	ctx := context.Background()
	tx := &fakeTranscript{items: sequencedItems(1)}
	runs := &fakeRuns{runs: []transcript.Run{{ID: "run_1"}}}
	ints := &fakeInterrupts{pending: []interrupts.Pending{{RootRunID: "run_1"}}}
	c := New(Dependencies{Transcript: tx, Interrupts: ints, Runs: runs, Sessions: &fakeSessions{}})

	page, err := c.ListItemPage(ctx, SessionItems("ses_1"), transcript.OldestFirst, "", 0)
	if err != nil || len(page.Items) != 1 || len(page.Runs) != 1 || tx.session != "ses_1" {
		t.Fatalf("ListItemPage items=%d runs=%d session=%q err=%v", len(page.Items), len(page.Runs), tx.session, err)
	}
	if !slices.Equal(runs.requested, []string{"run_1"}) {
		t.Fatalf("threaded runs = %v, want only the run the page's items belong to", runs.requested)
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
	c := New(Dependencies{Transcript: tx, Runs: &fakeRuns{}, Sessions: &fakeSessions{}})

	first, err := c.ListItemPage(ctx, SessionItems("ses_1"), transcript.OldestFirst, "", 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if tx.limit != 3 {
		t.Fatalf("store asked for %d rows, want the page plus one", tx.limit)
	}
	if len(first.Items) != 2 || first.Items[0].ID != "it_1" || first.NextCursor == "" {
		t.Fatalf("first page = %+v, want two items and a cursor", first)
	}

	second, err := c.ListItemPage(ctx, SessionItems("ses_1"), transcript.OldestFirst, first.NextCursor, 2)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if tx.afterSequence != 2 {
		t.Fatalf("second page sought past %d, want the first page's last position", tx.afterSequence)
	}
	if len(second.Items) != 2 || second.Items[0].ID != "it_3" {
		t.Fatalf("second page = %+v, want it_3 onward", second)
	}

	last, err := c.ListItemPage(ctx, SessionItems("ses_1"), transcript.OldestFirst, second.NextCursor, 2)
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
	c := New(Dependencies{
		Transcript: tx,
		Runs:       &fakeRuns{history: []transcript.Run{{ID: "run_1"}}},
		Sessions:   &fakeSessions{},
	})

	other, err := c.ListItemPage(ctx, SessionItems("ses_other"), transcript.OldestFirst, "", 2)
	if err != nil {
		t.Fatalf("other session page: %v", err)
	}
	if _, err := c.ListItemPage(ctx, SessionItems("ses_1"), transcript.OldestFirst, other.NextCursor, 2); !errors.Is(err, keyset.ErrInvalidCursor) {
		t.Fatalf("cross-session cursor err = %v, want ErrInvalidCursor", err)
	}
	if _, err := c.ListItemPage(ctx, SessionItems("ses_1"), transcript.OldestFirst, "not-a-cursor", 2); !errors.Is(err, keyset.ErrInvalidCursor) {
		t.Fatalf("damaged cursor err = %v, want ErrInvalidCursor", err)
	}

	// Direction is part of the query, not a display preference applied afterwards: an
	// anchor from a forward page names a position a backward page never reaches.
	forward, err := c.ListItemPage(ctx, SessionItems("ses_1"), transcript.OldestFirst, "", 2)
	if err != nil {
		t.Fatalf("forward page: %v", err)
	}
	if _, err := c.ListItemPage(ctx, SessionItems("ses_1"), transcript.NewestFirst, forward.NextCursor, 2); !errors.Is(err, keyset.ErrInvalidCursor) {
		t.Fatalf("reversed-direction cursor err = %v, want ErrInvalidCursor", err)
	}

	// A run scope is a different collection from the session that contains it, even
	// when every item in the session belongs to that run.
	runScoped, err := c.ListItemPage(ctx, RunItems("run_1"), transcript.OldestFirst, "", 2)
	if err != nil {
		t.Fatalf("run page: %v", err)
	}
	if _, err := c.ListItemPage(ctx, SessionItems("ses_1"), transcript.OldestFirst, runScoped.NextCursor, 2); !errors.Is(err, keyset.ErrInvalidCursor) {
		t.Fatalf("run cursor on the session page err = %v, want ErrInvalidCursor", err)
	}
}

// TestListItemPageWalksBackwardFromTheTail is items.list's other direction: the same
// durable sequence read from the end. A long session's first screen is its tail, and
// paging forward to reach it would read everything before it first.
func TestListItemPageWalksBackwardFromTheTail(t *testing.T) {
	ctx := context.Background()
	tx := &fakeTranscript{items: sequencedItems(5)}
	c := New(Dependencies{Transcript: tx, Runs: &fakeRuns{}, Sessions: &fakeSessions{}})

	first, err := c.ListItemPage(ctx, SessionItems("ses_1"), transcript.NewestFirst, "", 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first.Items) != 2 || first.Items[0].ID != "it_5" || first.Items[1].ID != "it_4" {
		t.Fatalf("first page = %+v, want the last two items newest first", first.Items)
	}

	second, err := c.ListItemPage(ctx, SessionItems("ses_1"), transcript.NewestFirst, first.NextCursor, 2)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if tx.afterSequence != 4 {
		t.Fatalf("second page sought from %d, want the first page's last position", tx.afterSequence)
	}
	if len(second.Items) != 2 || second.Items[0].ID != "it_3" {
		t.Fatalf("second page = %+v, want it_3 backwards", second.Items)
	}
}

// TestListItemPageScopedToARunReadsOnlyThatRun pins the run scope: the items of one
// run, resolved from the run id alone. A caller holding a runId does not have to
// discover the session to read what that run did.
func TestListItemPageScopedToARunReadsOnlyThatRun(t *testing.T) {
	ctx := context.Background()
	items := sequencedItems(3)
	items[2].Item.RunID = "run_2"
	tx := &fakeTranscript{items: items}
	runs := &fakeRuns{
		runs:    []transcript.Run{{ID: "run_1"}, {ID: "run_2"}},
		history: []transcript.Run{{ID: "run_1"}, {ID: "run_2"}},
	}
	c := New(Dependencies{Transcript: tx, Runs: runs, Sessions: &fakeSessions{}})

	page, err := c.ListItemPage(ctx, RunItems("run_2"), transcript.OldestFirst, "", 0)
	if err != nil {
		t.Fatalf("run page: %v", err)
	}
	if tx.run != "run_2" || tx.session != "" {
		t.Fatalf("read run=%q session=%q, want only the run scope", tx.run, tx.session)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "it_3" {
		t.Fatalf("page = %+v, want only run_2's item", page.Items)
	}
	// The page carries the runs its own items reference — not the session's list.
	if !slices.Equal(runs.requested, []string{"run_2"}) {
		t.Fatalf("threaded runs = %v, want only run_2", runs.requested)
	}
}

// TestListItemPageRefusesAScopeThatNamesNothing keeps an empty page from standing in
// for a wrong id. "This session has no items" and "there is no such session" are
// different facts, and a client that cannot tell them apart will show an empty
// timeline for a typo.
func TestListItemPageRefusesAScopeThatNamesNothing(t *testing.T) {
	ctx := context.Background()
	c := New(Dependencies{
		Transcript: &fakeTranscript{items: sequencedItems(3)},
		Runs:       &fakeRuns{history: []transcript.Run{{ID: "run_1"}}},
		Sessions:   &fakeSessions{missing: []string{"ses_gone"}},
	})

	if _, err := c.ListItemPage(ctx, SessionItems("ses_gone"), transcript.OldestFirst, "", 0); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("missing session err = %v, want session.ErrNotFound", err)
	}
	if _, err := c.ListItemPage(ctx, RunItems("run_gone"), transcript.OldestFirst, "", 0); !errors.Is(err, transcript.ErrRunNotFound) {
		t.Fatalf("missing run err = %v, want transcript.ErrRunNotFound", err)
	}
}

func TestListItemPageRejectsANegativeLimit(t *testing.T) {
	c := New(Dependencies{Transcript: &fakeTranscript{items: sequencedItems(1)}, Runs: &fakeRuns{}, Sessions: &fakeSessions{}})
	if _, err := c.ListItemPage(context.Background(), SessionItems("ses_1"), transcript.OldestFirst, "", -1); err == nil {
		t.Fatal("negative limit returned no error")
	}
}

// history builds the run page's rows in the order the store returns them: newest
// admission first, one nanosecond apart. States cycle through the three lifecycle
// positions so a status filter has something to exclude.
func history(sessionID string, ids ...string) []transcript.Run {
	states := [...]execution.RunState{execution.Running, execution.Interrupted, execution.Completed}
	out := make([]transcript.Run, 0, len(ids))
	for i, id := range ids {
		state := states[i%len(states)]
		run := transcript.Run{
			ID: id, SessionID: sessionID, State: state,
			CreatedAt: time.Unix(0, int64(len(ids)-i)).UTC(),
		}
		if state.IsTerminal() {
			run.Outcome = new(execution.OutcomeCompleted)
		}
		out = append(out, run)
	}
	return out
}

func parked(sessionID string, ids ...string) []interrupts.Pending {
	out := make([]interrupts.Pending, 0, len(ids))
	for i, id := range ids {
		out = append(out, interrupts.Pending{
			RootRunID: id, SessionID: sessionID, CreatedAt: time.Unix(0, int64(i+1)).UTC(),
		})
	}
	return out
}

// TestListRunPageWalksBackwardThroughHistory covers runs.list's query properties:
// the order is fixed (admission descending, tie-broken by id), the next page seeks
// strictly EARLIER than the last row rather than re-reading it, and "there is more"
// is only claimed when the over-fetch found it.
//
// Newest first is the direction the contract fixes, and it is the one a client
// needs: the run it is looking for is almost always the last one, and paging
// forward from the beginning of a long session would reach it last.
func TestListRunPageWalksBackwardThroughHistory(t *testing.T) {
	ctx := context.Background()
	runs := &fakeRuns{history: history("ses_1", "run_3", "run_2", "run_1")}
	c := New(Dependencies{Transcript: &fakeTranscript{}, Runs: runs, Sessions: &fakeSessions{}})

	first, err := c.ListRunPage(ctx, "ses_1", nil, "", 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if runs.limit != 3 {
		t.Fatalf("store asked for %d rows, want the page plus one", runs.limit)
	}
	if len(first.Rows) != 2 || first.Rows[0].ID != "run_3" || first.NextCursor == "" {
		t.Fatalf("first page = %+v, want the two newest runs and a cursor", first.Rows)
	}

	second, err := c.ListRunPage(ctx, "ses_1", nil, first.NextCursor, 2)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if runs.beforeRunID != "run_2" {
		t.Fatalf("second page sought before %q, want the first page's last row", runs.beforeRunID)
	}
	if len(second.Rows) != 1 || second.Rows[0].ID != "run_1" || second.NextCursor != "" {
		t.Fatalf("second page = %+v, want the tail and no cursor", second.Rows)
	}
}

// TestListRunPageReturnsEveryStatusUntilFiltered pins the default: the read is the
// whole history, not the work in progress. A page that hid finished runs would make
// "what did this session cost" unanswerable from the run record, which is the one
// place that knows.
func TestListRunPageReturnsEveryStatusUntilFiltered(t *testing.T) {
	ctx := context.Background()
	runs := &fakeRuns{history: history("ses_1", "run_3", "run_2", "run_1")}
	c := New(Dependencies{Transcript: &fakeTranscript{}, Runs: runs, Sessions: &fakeSessions{}})

	all, err := c.ListRunPage(ctx, "ses_1", nil, "", 0)
	if err != nil || len(all.Rows) != 3 {
		t.Fatalf("unfiltered page = %d rows, %v; want every status", len(all.Rows), err)
	}

	// A filter is normalized before it selects rows OR mints a cursor: the same set
	// asked for in a different order is the same query, and it must page as one.
	filtered, err := c.ListRunPage(ctx, "ses_1", []execution.RunStatus{
		execution.StatusWaiting, execution.StatusRunning, execution.StatusWaiting,
	}, "", 0)
	if err != nil {
		t.Fatalf("filtered page: %v", err)
	}
	if want := []execution.RunStatus{execution.StatusRunning, execution.StatusWaiting}; !slices.Equal(runs.statuses, want) {
		t.Fatalf("store filtered on %v, want the normalized %v", runs.statuses, want)
	}
	if len(filtered.Rows) != 2 {
		t.Fatalf("filtered page = %d rows, want the running and waiting ones", len(filtered.Rows))
	}
}

// TestListRunPageRefusesACursorFromAnotherQuery is runs.list's half of the cursor
// binding: an anchor is only meaningful against the ordering AND the filter that
// produced it. Continuing from a foreign one silently pages against positions this
// query never enumerated — the client is handed rows it already has, or none at
// all, with nothing to say why.
func TestListRunPageRefusesACursorFromAnotherQuery(t *testing.T) {
	ctx := context.Background()
	c := New(Dependencies{
		Transcript: &fakeTranscript{items: sequencedItems(5)},
		Runs:       &fakeRuns{history: history("ses_1", "run_3", "run_2", "run_1")},
		Interrupts: &fakeInterrupts{pending: parked("ses_1", "run_1", "run_2", "run_3")},
		Sessions:   &fakeSessions{},
	})

	otherSession, err := c.ListRunPage(ctx, "ses_other", nil, "", 2)
	if err != nil {
		t.Fatalf("other session page: %v", err)
	}
	if _, err := c.ListRunPage(ctx, "ses_1", nil, otherSession.NextCursor, 2); !errors.Is(err, keyset.ErrInvalidCursor) {
		t.Fatalf("cross-session cursor err = %v, want ErrInvalidCursor", err)
	}

	// Changing the status filter changes which rows exist, so the anchor no longer
	// names a position in the collection being paged.
	unfiltered, err := c.ListRunPage(ctx, "ses_1", nil, "", 2)
	if err != nil {
		t.Fatalf("unfiltered page: %v", err)
	}
	if _, err := c.ListRunPage(ctx, "ses_1", []execution.RunStatus{execution.StatusRunning}, unfiltered.NextCursor, 2); !errors.Is(err, keyset.ErrInvalidCursor) {
		t.Fatalf("cross-filter cursor err = %v, want ErrInvalidCursor", err)
	}

	// The interrupt page is scoped the same way and ordered by a timestamp too, so
	// only the query namespace tells the two apart.
	interruptPage, err := c.ListPendingInterruptPage(ctx, "ses_1", "", 2)
	if err != nil {
		t.Fatalf("interrupt page: %v", err)
	}
	if _, err := c.ListRunPage(ctx, "ses_1", nil, interruptPage.NextCursor, 2); !errors.Is(err, keyset.ErrInvalidCursor) {
		t.Fatalf("cross-query cursor err = %v, want ErrInvalidCursor", err)
	}

	itemPage, err := c.ListItemPage(ctx, SessionItems("ses_1"), transcript.OldestFirst, "", 2)
	if err != nil {
		t.Fatalf("item page: %v", err)
	}
	if _, err := c.ListRunPage(ctx, "ses_1", nil, itemPage.NextCursor, 2); !errors.Is(err, keyset.ErrInvalidCursor) {
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
		Runs:       &fakeRuns{history: history("ses_1", "run_3", "run_2", "run_1")},
		Interrupts: ints,
		Sessions:   &fakeSessions{},
	})

	first, err := c.ListPendingInterruptPage(ctx, "ses_1", "", 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if ints.limit != 3 {
		t.Fatalf("store asked for %d rows, want the page plus one", ints.limit)
	}
	if len(first.Rows) != 2 || first.Rows[0].RootRunID != "run_1" || first.NextCursor == "" {
		t.Fatalf("first page = %+v, want two pending sets and a cursor", first.Rows)
	}

	second, err := c.ListPendingInterruptPage(ctx, "ses_1", first.NextCursor, 2)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if ints.afterRunID != "run_2" {
		t.Fatalf("second page sought past %q, want the first page's last row", ints.afterRunID)
	}
	if len(second.Rows) != 1 || second.Rows[0].RootRunID != "run_3" || second.NextCursor != "" {
		t.Fatalf("second page = %+v, want the tail and no cursor", second.Rows)
	}
	if _, err := c.ListPendingInterruptPage(ctx, "ses_1", first.NextCursor+"x", 2); !errors.Is(err, keyset.ErrInvalidCursor) {
		t.Fatalf("damaged cursor err = %v, want ErrInvalidCursor", err)
	}

	// The run page is scoped and ordered the same way, so only the query namespace
	// separates the two — in both directions.
	runPage, err := c.ListRunPage(ctx, "ses_1", nil, "", 2)
	if err != nil {
		t.Fatalf("run page: %v", err)
	}
	if _, err := c.ListPendingInterruptPage(ctx, "ses_1", runPage.NextCursor, 2); !errors.Is(err, keyset.ErrInvalidCursor) {
		t.Fatalf("run cursor on the interrupt page err = %v, want ErrInvalidCursor", err)
	}
}
