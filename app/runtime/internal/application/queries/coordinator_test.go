package queries

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/pagination"
)

type fakeTranscript struct {
	items []transcript.SequencedItem
	trees map[string][]string

	session       string
	run           string
	runTree       string
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

func (f *fakeTranscript) PageRunTreeItems(_ context.Context, runID string, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	f.runTree = runID
	runIDs := append([]string{runID}, f.trees[runID]...)
	return f.page(order, fromSequence, limit, func(item transcript.Item) bool {
		return slices.Contains(runIDs, item.RunID)
	})
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
	statuses        []run.Status
	descendants     bool
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

func (f *fakeRuns) PageRuns(_ context.Context, sessionID string, statuses []run.Status, includeDescendants bool, beforeCreatedAt int64, beforeRunID string, limit int) ([]transcript.Run, error) {
	f.session, f.statuses, f.descendants = sessionID, statuses, includeDescendants
	f.beforeCreatedAt, f.beforeRunID, f.limit = beforeCreatedAt, beforeRunID, limit
	var out []transcript.Run
	for _, run := range f.history {
		if !includeDescendants && run.Lineage().IsChild() {
			continue
		}
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

func (f *fakeRuns) RunsWithAncestors(_ context.Context, runIDs []string) ([]transcript.Run, error) {
	f.requested = runIDs
	wanted := slices.Clone(runIDs)
	for index := 0; index < len(wanted); index++ {
		for _, run := range f.runs {
			if run.ID == wanted[index] && run.ParentRunID != "" && !slices.Contains(wanted, run.ParentRunID) {
				wanted = append(wanted, run.ParentRunID)
			}
		}
	}
	var out []transcript.Run
	for _, run := range f.runs {
		if slices.Contains(wanted, run.ID) {
			out = append(out, run)
		}
	}
	return out, nil
}

type fakeInterrupts struct {
	pending []runs.Pending

	session        string
	rootRun        string
	afterCreatedAt int64
	afterRunID     string
	limit          int
}

func (f *fakeInterrupts) ListPage(_ context.Context, sessionID, rootRunID string, afterCreatedAt int64, afterRunID string, limit int) ([]runs.Pending, error) {
	f.session, f.rootRun = sessionID, rootRunID
	f.afterCreatedAt, f.afterRunID, f.limit = afterCreatedAt, afterRunID, limit
	var out []runs.Pending
	for _, pending := range f.pending {
		if !seeksPast(pending.CreatedAt.UnixNano(), pending.RootRunID, afterCreatedAt, afterRunID) {
			continue
		}
		if rootRunID != "" && pending.RootRunID != rootRunID {
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

func queryRunIDs(runs []transcript.Run) []string {
	out := make([]string, 0, len(runs))
	for _, run := range runs {
		out = append(out, run.ID)
	}
	return out
}

func TestCoordinatorReadsDelegateToProjections(t *testing.T) {
	ctx := context.Background()
	tx := &fakeTranscript{items: sequencedItems(1)}
	runStore := &fakeRuns{runs: []transcript.Run{{ID: "run_1"}}}
	ints := &fakeInterrupts{pending: []runs.Pending{{RootRunID: "run_1"}}}
	c := New(Dependencies{Transcript: tx, Interrupts: ints, Runs: runStore, Sessions: &fakeSessions{}})

	page, err := c.ListItemPage(ctx, SessionItems("ses_1"), transcript.OldestFirst, "", 0)
	if err != nil || len(page.Items) != 1 || len(page.Runs) != 1 || tx.session != "ses_1" {
		t.Fatalf("ListItemPage items=%d runs=%d session=%q err=%v", len(page.Items), len(page.Runs), tx.session, err)
	}
	if !slices.Equal(runStore.requested, []string{"run_1"}) {
		t.Fatalf("threaded runs = %v, want only the run the page's items belong to", runStore.requested)
	}

	pending, err := c.ListPendingInterruptPage(ctx, "ses_2", "", run.Capabilities{}, "", 0)
	if err != nil || len(pending.Rows) != 1 || ints.session != "ses_2" {
		t.Fatalf("ListPendingInterruptPage pending=%d session=%q err=%v", len(pending.Rows), ints.session, err)
	}
}

// The page is cut by the query, not after the fact: the read asks for exactly one
// row more than it will return, which is both how "there is more" is known and
// what keeps a long session's history out of memory.
// It is also the items cursor namespace's fixed-order and next-page-direction fixture.
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
// rows it had already read, as if they were new. It is the items cursor-binding
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
	if _, err := c.ListItemPage(ctx, SessionItems("ses_1"), transcript.OldestFirst, other.NextCursor, 2); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("cross-session cursor err = %v, want ErrInvalidCursor", err)
	}
	if _, err := c.ListItemPage(ctx, SessionItems("ses_1"), transcript.OldestFirst, "not-a-cursor", 2); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("damaged cursor err = %v, want ErrInvalidCursor", err)
	}

	// Direction is part of the query, not a display preference applied afterwards: an
	// anchor from a forward page names a position a backward page never reaches.
	forward, err := c.ListItemPage(ctx, SessionItems("ses_1"), transcript.OldestFirst, "", 2)
	if err != nil {
		t.Fatalf("forward page: %v", err)
	}
	if _, err := c.ListItemPage(ctx, SessionItems("ses_1"), transcript.NewestFirst, forward.NextCursor, 2); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("reversed-direction cursor err = %v, want ErrInvalidCursor", err)
	}

	// A run scope is a different collection from the session that contains it, even
	// when every item in the session belongs to that run.
	runScoped, err := c.ListItemPage(ctx, RunItems("run_1"), transcript.OldestFirst, "", 2)
	if err != nil {
		t.Fatalf("run page: %v", err)
	}
	if _, err := c.ListItemPage(ctx, SessionItems("ses_1"), transcript.OldestFirst, runScoped.NextCursor, 2); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("run cursor on the session page err = %v, want ErrInvalidCursor", err)
	}
}

// TestListItemPageWalksBackwardFromTheTail is the items read's other direction: the same
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

func TestListItemPageScopesASubtreeAndIncludesAncestors(t *testing.T) {
	root := transcript.Run{ID: "run_root"}
	child := transcript.Run{
		ID: "run_child", SpawnedByItemID: "item_spawn_child",
		ParentRunID: root.ID, RootRunID: root.ID,
	}
	grandchild := transcript.Run{
		ID: "run_grandchild", SpawnedByItemID: "item_spawn_grandchild",
		ParentRunID: child.ID, RootRunID: root.ID,
	}
	sibling := transcript.Run{
		ID: "run_sibling", SpawnedByItemID: "item_spawn_sibling",
		ParentRunID: root.ID, RootRunID: root.ID,
	}
	tx := &fakeTranscript{
		items: []transcript.SequencedItem{
			{Sequence: 1, Item: transcript.Item{ID: "item_root", RunID: root.ID}},
			{Sequence: 2, Item: transcript.Item{ID: "item_child", RunID: child.ID}},
			{Sequence: 3, Item: transcript.Item{ID: "item_grandchild", RunID: grandchild.ID}},
			{Sequence: 4, Item: transcript.Item{ID: "item_sibling", RunID: sibling.ID}},
		},
		trees: map[string][]string{child.ID: {grandchild.ID}},
	}
	runs := &fakeRuns{
		runs:    []transcript.Run{grandchild, child, root, sibling},
		history: []transcript.Run{grandchild, sibling, child, root},
	}
	c := New(Dependencies{Transcript: tx, Runs: runs, Sessions: &fakeSessions{}})

	page, err := c.ListItemPage(t.Context(), RunTreeItems(child.ID), transcript.OldestFirst, "", 0)
	if err != nil {
		t.Fatalf("subtree page: %v", err)
	}
	if tx.runTree != child.ID || tx.run != "" {
		t.Fatalf("read subtree=%q exact=%q, want only child subtree", tx.runTree, tx.run)
	}
	if len(page.Items) != 2 {
		t.Fatalf("subtree items = %+v, want child and grandchild only", page.Items)
	}
	if got := []string{page.Items[0].ID, page.Items[1].ID}; !slices.Equal(got, []string{"item_child", "item_grandchild"}) {
		t.Fatalf("subtree items = %v, want child and grandchild only", got)
	}
	if got := queryRunIDs(page.Runs); !slices.Equal(got, []string{grandchild.ID, child.ID, root.ID}) {
		t.Fatalf("page runs = %v, want direct runs plus ancestor closure", got)
	}
	if !slices.Equal(runs.requested, []string{child.ID, grandchild.ID}) {
		t.Fatalf("directly referenced runs = %v, want page item sources only", runs.requested)
	}

	first, err := c.ListItemPage(t.Context(), RunTreeItems(child.ID), transcript.OldestFirst, "", 1)
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first subtree page = (%+v, %v), want a cursor", first, err)
	}
	if _, err := c.ListItemPage(t.Context(), RunItems(child.ID), transcript.OldestFirst, first.NextCursor, 1); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("subtree cursor on exact-run page = %v, want ErrInvalidCursor", err)
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

func TestListItemPageRejectsAnUnknownOrder(t *testing.T) {
	tx := &fakeTranscript{items: sequencedItems(1)}
	c := New(Dependencies{Transcript: tx, Runs: &fakeRuns{}, Sessions: &fakeSessions{}})

	if _, err := c.ListItemPage(t.Context(), SessionItems("ses_1"), transcript.SequenceOrder("ascending"), "", 1); err == nil {
		t.Fatal("unknown order returned no error")
	}
	if tx.order != "" {
		t.Fatalf("unknown order reached transcript store as %q", tx.order)
	}
}

func TestSequenceAnchorRequiresAPositiveSequence(t *testing.T) {
	for _, anchor := range [][]string{{"0"}, {"-1"}, {"not-a-sequence"}, {"1", "extra"}} {
		if _, err := sequenceAnchor(anchor); !errors.Is(err, pagination.ErrInvalidCursor) {
			t.Fatalf("sequenceAnchor(%q) err = %v, want ErrInvalidCursor", anchor, err)
		}
	}
	if got, err := sequenceAnchor([]string{"1"}); err != nil || got != 1 {
		t.Fatalf("sequenceAnchor(1) = (%d, %v)", got, err)
	}
}

// testSessionRunHistory builds the run page's rows in the order the store
// returns them: newest admission first, one nanosecond apart. States cycle
// through the three lifecycle positions so a status filter has something to
// exclude.
func testSessionRunHistory(ids ...string) []transcript.Run {
	states := [...]run.State{run.Running, run.Waiting, run.Completed}
	out := make([]transcript.Run, 0, len(ids))
	for i, id := range ids {
		state := states[i%len(states)]
		record := transcript.Run{
			ID: id, SessionID: "ses_1", State: state,
			CreatedAt: time.Unix(0, int64(len(ids)-i)).UTC(),
		}
		if state.IsTerminal() {
			record.Outcome = new(run.OutcomeCompleted)
		}
		out = append(out, record)
	}
	return out
}

func testSessionPendingRuns(ids ...string) []runs.Pending {
	out := make([]runs.Pending, 0, len(ids))
	for i, id := range ids {
		out = append(out, runs.Pending{
			RootRunID: id, SessionID: "ses_1", CreatedAt: time.Unix(0, int64(i+1)).UTC(),
		})
	}
	return out
}

// TestListPendingInterruptPageRefusesACallerThatCannotFollowTheRun is the deferred
// half of the capabilities rule: a waiting set belongs to a run with a frozen contract,
// and a caller that cannot follow that contract is refused the set — never handed
// the parts it happens to understand.
//
// A trimmed set is worse than an error: the client would answer what it received,
// resume would consume it as the whole set, and the run would sit waiting on
// interrupts the client believes it resolved.
func TestListPendingInterruptPageRefusesACallerThatCannotFollowTheRun(t *testing.T) {
	ctx := context.Background()
	waiting := testSessionPendingRuns("run_1")
	waiting[0].Capabilities = run.Capabilities{
		InterruptKinds: []interrupt.Kind{interrupt.Approval, interrupt.Question},
	}
	c := New(Dependencies{
		Transcript: &fakeTranscript{},
		Runs:       &fakeRuns{history: []transcript.Run{{ID: "run_1"}}},
		Interrupts: &fakeInterrupts{pending: waiting},
		Sessions:   &fakeSessions{},
	})

	answersOnlyApprovals := run.Capabilities{
		InterruptKinds: []interrupt.Kind{interrupt.Approval},
	}
	if _, err := c.ListPendingInterruptPage(ctx, "ses_1", "", answersOnlyApprovals, "", 0); !errors.Is(err, run.ErrInsufficientCapabilities) {
		t.Fatalf("partial caller err = %v, want ErrInsufficientCapabilities", err)
	}

	full := run.Capabilities{
		InterruptKinds: []interrupt.Kind{interrupt.Approval, interrupt.Question},
	}
	page, err := c.ListPendingInterruptPage(ctx, "ses_1", "", full, "", 0)
	if err != nil || len(page.Rows) != 1 {
		t.Fatalf("covering caller = %d rows, %v; want the whole set", len(page.Rows), err)
	}
}

// TestListPendingInterruptPageFiltersByRootAndRefusesAChild pins the run filter: it
// narrows to one waiting tree, and a child id is a refusal rather than an empty page
// — the set the caller wants exists, under the root, so "nothing here" would send it
// looking in the wrong place.
func TestListPendingInterruptPageFiltersByRootAndRefusesAChild(t *testing.T) {
	ctx := context.Background()
	ints := &fakeInterrupts{pending: testSessionPendingRuns("run_1", "run_2")}
	c := New(Dependencies{
		Transcript: &fakeTranscript{},
		Runs: &fakeRuns{history: []transcript.Run{
			{ID: "run_1"},
			{
				ID: "run_child", SpawnedByItemID: "it_spawn",
				ParentRunID: "run_1", RootRunID: "run_1",
			},
		}},
		Interrupts: ints,
		Sessions:   &fakeSessions{},
	})

	page, err := c.ListPendingInterruptPage(ctx, "", "run_1", run.Capabilities{}, "", 0)
	if err != nil {
		t.Fatalf("root-filtered page: %v", err)
	}
	if ints.rootRun != "run_1" || len(page.Rows) != 1 || page.Rows[0].RootRunID != "run_1" {
		t.Fatalf("filtered page = %+v (asked %q), want only run_1's set", page.Rows, ints.rootRun)
	}

	if _, err := c.ListPendingInterruptPage(ctx, "", "run_child", run.Capabilities{}, "", 0); !errors.Is(err, transcript.ErrNotRoot) {
		t.Fatalf("child filter err = %v, want transcript.ErrNotRoot", err)
	}

	// The filter is part of the cursor's identity: the same anchor against a
	// different filter names a position in a collection it never enumerated.
	unfiltered, err := c.ListPendingInterruptPage(ctx, "", "", run.Capabilities{}, "", 1)
	if err != nil {
		t.Fatalf("unfiltered page: %v", err)
	}
	if _, err := c.ListPendingInterruptPage(ctx, "", "run_1", run.Capabilities{}, unfiltered.NextCursor, 1); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("cross-filter cursor err = %v, want ErrInvalidCursor", err)
	}
}

// TestListRunPageWalksBackwardThroughHistory covers the runs query properties:
// the order is fixed (admission descending, tie-broken by id), the next page seeks
// strictly EARLIER than the last row rather than re-reading it, and "there is more"
// is only claimed when the over-fetch found it.
//
// Newest first is the direction the contract fixes, and it is the one a client
// needs: the run it is looking for is almost always the last one, and paging
// forward from the beginning of a long session would reach it last.
func TestListRunPageWalksBackwardThroughHistory(t *testing.T) {
	ctx := context.Background()
	runs := &fakeRuns{history: testSessionRunHistory("run_3", "run_2", "run_1")}
	c := New(Dependencies{Transcript: &fakeTranscript{}, Runs: runs, Sessions: &fakeSessions{}})

	first, err := c.ListRunPage(ctx, RunPageFilter{SessionID: "ses_1"}, "", 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if runs.limit != 3 {
		t.Fatalf("store asked for %d rows, want the page plus one", runs.limit)
	}
	if len(first.Rows) != 2 || first.Rows[0].ID != "run_3" || first.NextCursor == "" {
		t.Fatalf("first page = %+v, want the two newest runs and a cursor", first.Rows)
	}

	second, err := c.ListRunPage(ctx, RunPageFilter{SessionID: "ses_1"}, first.NextCursor, 2)
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
	runs := &fakeRuns{history: testSessionRunHistory("run_3", "run_2", "run_1")}
	c := New(Dependencies{Transcript: &fakeTranscript{}, Runs: runs, Sessions: &fakeSessions{}})

	all, err := c.ListRunPage(ctx, RunPageFilter{SessionID: "ses_1"}, "", 0)
	if err != nil || len(all.Rows) != 3 {
		t.Fatalf("unfiltered page = %d rows, %v; want every status", len(all.Rows), err)
	}

	// A filter is normalized before it selects rows OR mints a cursor: the same set
	// asked for in a different order is the same query, and it must page as one.
	filtered, err := c.ListRunPage(ctx, RunPageFilter{
		SessionID: "ses_1",
		Statuses: []run.Status{
			run.StatusWaiting, run.StatusRunning, run.StatusWaiting,
		},
	}, "", 0)
	if err != nil {
		t.Fatalf("filtered page: %v", err)
	}
	if want := []run.Status{run.StatusRunning, run.StatusWaiting}; !slices.Equal(runs.statuses, want) {
		t.Fatalf("store filtered on %v, want the normalized %v", runs.statuses, want)
	}
	if len(filtered.Rows) != 2 {
		t.Fatalf("filtered page = %d rows, want the running and waiting ones", len(filtered.Rows))
	}
}

func TestListRunPageIncludesDescendantsAndBindsTheCursor(t *testing.T) {
	root := transcript.Run{ID: "run_root", CreatedAt: time.Unix(0, 1).UTC()}
	child := transcript.Run{
		ID: "run_child", SpawnedByItemID: "item_spawn_child",
		ParentRunID: root.ID, RootRunID: root.ID, CreatedAt: time.Unix(0, 2).UTC(),
	}
	grandchild := transcript.Run{
		ID: "run_grandchild", SpawnedByItemID: "item_spawn_grandchild",
		ParentRunID: child.ID, RootRunID: root.ID, CreatedAt: time.Unix(0, 3).UTC(),
	}
	runs := &fakeRuns{history: []transcript.Run{grandchild, child, root}}
	c := New(Dependencies{Transcript: &fakeTranscript{}, Runs: runs, Sessions: &fakeSessions{}})

	roots, err := c.ListRunPage(t.Context(), RunPageFilter{}, "", 0)
	if err != nil || !slices.Equal(queryRunIDs(roots.Rows), []string{root.ID}) {
		t.Fatalf("root page = (%v, %v), want only root", queryRunIDs(roots.Rows), err)
	}
	all, err := c.ListRunPage(t.Context(), RunPageFilter{IncludeDescendants: true}, "", 2)
	if err != nil {
		t.Fatalf("descendant page: %v", err)
	}
	if !runs.descendants || !slices.Equal(queryRunIDs(all.Rows), []string{grandchild.ID, child.ID}) || all.NextCursor == "" {
		t.Fatalf("descendant page = %+v, include=%t; want newest two and cursor", queryRunIDs(all.Rows), runs.descendants)
	}
	if _, err := c.ListRunPage(t.Context(), RunPageFilter{}, all.NextCursor, 2); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("descendant cursor on root page = %v, want ErrInvalidCursor", err)
	}
}

// TestListRunPageRefusesACursorFromAnotherQuery is the runs read's half of the cursor
// binding: an anchor is only meaningful against the ordering AND the filter that
// produced it. Continuing from a foreign one silently pages against positions this
// query never enumerated — the client is handed rows it already has, or none at
// all, with nothing to say why.
func TestListRunPageRefusesACursorFromAnotherQuery(t *testing.T) {
	ctx := context.Background()
	c := New(Dependencies{
		Transcript: &fakeTranscript{items: sequencedItems(5)},
		Runs:       &fakeRuns{history: testSessionRunHistory("run_3", "run_2", "run_1")},
		Interrupts: &fakeInterrupts{pending: testSessionPendingRuns("run_1", "run_2", "run_3")},
		Sessions:   &fakeSessions{},
	})

	otherSession, err := c.ListRunPage(ctx, RunPageFilter{SessionID: "ses_other"}, "", 2)
	if err != nil {
		t.Fatalf("other session page: %v", err)
	}
	if _, err := c.ListRunPage(ctx, RunPageFilter{SessionID: "ses_1"}, otherSession.NextCursor, 2); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("cross-session cursor err = %v, want ErrInvalidCursor", err)
	}

	// Changing the status filter changes which rows exist, so the anchor no longer
	// names a position in the collection being paged.
	unfiltered, err := c.ListRunPage(ctx, RunPageFilter{SessionID: "ses_1"}, "", 2)
	if err != nil {
		t.Fatalf("unfiltered page: %v", err)
	}
	if _, err := c.ListRunPage(ctx, RunPageFilter{
		SessionID: "ses_1",
		Statuses:  []run.Status{run.StatusRunning},
	}, unfiltered.NextCursor, 2); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("cross-filter cursor err = %v, want ErrInvalidCursor", err)
	}

	// The interrupt page is scoped the same way and ordered by a timestamp too, so
	// only the query namespace tells the two apart.
	interruptPage, err := c.ListPendingInterruptPage(ctx, "ses_1", "", run.Capabilities{}, "", 2)
	if err != nil {
		t.Fatalf("interrupt page: %v", err)
	}
	if _, err := c.ListRunPage(ctx, RunPageFilter{SessionID: "ses_1"}, interruptPage.NextCursor, 2); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("cross-query cursor err = %v, want ErrInvalidCursor", err)
	}

	itemPage, err := c.ListItemPage(ctx, SessionItems("ses_1"), transcript.OldestFirst, "", 2)
	if err != nil {
		t.Fatalf("item page: %v", err)
	}
	if _, err := c.ListRunPage(ctx, RunPageFilter{SessionID: "ses_1"}, itemPage.NextCursor, 2); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("item cursor on the run page err = %v, want ErrInvalidCursor", err)
	}
}

// TestListPendingInterruptPagePagesOldestFirst is the same three properties for
// the interrupts read, whose order the contract fixes as oldest first: a
// resumable run that keeps sinking below the page boundary is one nobody answers.
func TestListPendingInterruptPagePagesOldestFirst(t *testing.T) {
	ctx := context.Background()
	ints := &fakeInterrupts{pending: testSessionPendingRuns("run_1", "run_2", "run_3")}
	c := New(Dependencies{
		Transcript: &fakeTranscript{},
		Runs:       &fakeRuns{history: testSessionRunHistory("run_3", "run_2", "run_1")},
		Interrupts: ints,
		Sessions:   &fakeSessions{},
	})

	first, err := c.ListPendingInterruptPage(ctx, "ses_1", "", run.Capabilities{}, "", 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if ints.limit != 3 {
		t.Fatalf("store asked for %d rows, want the page plus one", ints.limit)
	}
	if len(first.Rows) != 2 || first.Rows[0].RootRunID != "run_1" || first.NextCursor == "" {
		t.Fatalf("first page = %+v, want two pending sets and a cursor", first.Rows)
	}

	second, err := c.ListPendingInterruptPage(ctx, "ses_1", "", run.Capabilities{}, first.NextCursor, 2)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if ints.afterRunID != "run_2" {
		t.Fatalf("second page sought past %q, want the first page's last row", ints.afterRunID)
	}
	if len(second.Rows) != 1 || second.Rows[0].RootRunID != "run_3" || second.NextCursor != "" {
		t.Fatalf("second page = %+v, want the tail and no cursor", second.Rows)
	}
	if _, err := c.ListPendingInterruptPage(ctx, "ses_1", "", run.Capabilities{}, first.NextCursor+"x", 2); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("damaged cursor err = %v, want ErrInvalidCursor", err)
	}

	// The run page is scoped and ordered the same way, so only the query namespace
	// separates the two — in both directions.
	runPage, err := c.ListRunPage(ctx, RunPageFilter{SessionID: "ses_1"}, "", 2)
	if err != nil {
		t.Fatalf("run page: %v", err)
	}
	if _, err := c.ListPendingInterruptPage(ctx, "ses_1", "", run.Capabilities{}, runPage.NextCursor, 2); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("run cursor on the interrupt page err = %v, want ErrInvalidCursor", err)
	}
}
