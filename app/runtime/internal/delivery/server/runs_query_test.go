package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/admission"
	"github.com/Tangerg/lynx/app/runtime/internal/application/queries"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// fakeInterruptReader backs the query coordinator's interrupt read for the
// ListOpenInterrupts wire-projection test.
type fakeInterruptReader struct {
	sessionID string
	pending   []runs.Pending
	err       error
}

func (r *fakeInterruptReader) List(_ context.Context, sessionID string) ([]runs.Pending, error) {
	r.sessionID = sessionID
	return r.pending, r.err
}

func (r *fakeInterruptReader) ListPage(ctx context.Context, sessionID, _ string, _ int64, _ string, _ int) ([]runs.Pending, error) {
	return r.List(ctx, sessionID)
}

func (r *fakeInterruptReader) Get(_ context.Context, runID string) (runs.Pending, bool, error) {
	for _, pending := range r.pending {
		if pending.RootRunID == runID {
			return pending, true, r.err
		}
	}
	return runs.Pending{}, false, r.err
}

// TestListRunsPublishesTheWholeHistoryNewestFirst is runs.list's delivery half: the
// page is every root run of the session — including the ones that ended — ordered
// newest first, and each row carries the position a client renders.
//
// Until this read covered history, "what did this session do" was only answerable by
// replaying items.list, which is the timeline and not the accounting.
func TestListRunsPublishesTheWholeHistoryNewestFirst(t *testing.T) {
	s, rt := rollbackHarness(t)
	putRun(t, rt, "ses_1", "run_old", 10, 1)
	putRun(t, rt, "ses_1", "run_new", 20, 2)
	putRun(t, rt, "ses_other", "run_elsewhere", 30, 1)

	page, err := s.ListRuns(t.Context(), protocol.ListRunsRequest{SessionID: "ses_1"})
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(page.Data) != 2 || page.Data[0].ID != "run_new" || page.Data[1].ID != "run_old" {
		t.Fatalf("page = %+v, want run_new then run_old", page.Data)
	}
	if page.Data[0].Status != protocol.RunStatusFinished || page.Data[0].Outcome == nil {
		t.Fatalf("finished run = %+v, want a status and the outcome explaining it", page.Data[0])
	}

	// A filter that excludes everything is an empty page, not an error: nothing is
	// wrong with asking whether this session has a run waiting on someone.
	waiting, err := s.ListRuns(t.Context(), protocol.ListRunsRequest{
		SessionID: "ses_1", Statuses: []protocol.RunStatus{protocol.RunStatusWaiting},
	})
	if err != nil || len(waiting.Data) != 0 {
		t.Fatalf("waiting page = %+v, %v; want empty", waiting, err)
	}
}

// The dispatcher owns capability authorization; once it accepts the request, the
// server must preserve both descendant filters instead of silently answering the
// root/exact-run defaults.
func TestDescendantFiltersReachTheDurableQueries(t *testing.T) {
	s, rt := rollbackHarness(t)
	putSession(t, rt, "ses_1")
	putRun(t, rt, "ses_1", "run_root", 10, 1)
	putChildRun(t, rt, "ses_1", "run_child", 20, 2)
	putUserItem(t, rt, "ses_1", "run_root", "item_root", "root")
	putUserItem(t, rt, "ses_1", "run_child", "item_child", "child")

	roots, err := s.ListRuns(t.Context(), protocol.ListRunsRequest{SessionID: "ses_1"})
	if err != nil || len(roots.Data) != 1 || roots.Data[0].ID != "run_root" {
		t.Fatalf("root run page = (%+v, %v), want root only", roots, err)
	}
	tree, err := s.ListRuns(t.Context(), protocol.ListRunsRequest{
		SessionID: "ses_1", IncludeDescendants: true,
	})
	if err != nil {
		t.Fatalf("descendant run page: %v", err)
	}
	if len(tree.Data) != 2 || tree.Data[0].ID != "run_child" || tree.Data[1].ID != "run_root" {
		t.Fatalf("descendant run page = %+v, want child then root", tree.Data)
	}

	exact, err := s.ListItems(t.Context(), protocol.ListItemsRequest{
		Scope: protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: "run_root"},
	})
	if err != nil || len(exact.Data) != 1 || exact.Data[0].ID != "item_root" {
		t.Fatalf("exact root items = (%+v, %v), want root item only", exact, err)
	}
	subtree, err := s.ListItems(t.Context(), protocol.ListItemsRequest{
		Scope: protocol.ItemListScope{
			Type: protocol.ItemScopeRun, RunID: "run_root", IncludeDescendants: true,
		},
	})
	if err != nil {
		t.Fatalf("root subtree items: %v", err)
	}
	if len(subtree.Data) != 2 ||
		subtree.Data[0].ID != "item_root" ||
		subtree.Data[1].ID != "item_child" {
		t.Fatalf("root subtree items = %+v, want root and child", subtree.Data)
	}
	if len(subtree.Runs) != 2 ||
		subtree.Runs[0].ID != "run_child" ||
		subtree.Runs[1].ID != "run_root" {
		t.Fatalf("root subtree summaries = %+v, want connected child/root tree", subtree.Runs)
	}
}

// TestListRunsRefusesAStatusThatIsNotOne keeps a filter value the vocabulary does
// not define from reaching the durable read. Dropping it would widen the page to
// every status while the client believed it had narrowed it.
func TestListRunsRefusesAStatusThatIsNotOne(t *testing.T) {
	s, _ := rollbackHarness(t)

	_, err := s.ListRuns(t.Context(), protocol.ListRunsRequest{
		Statuses: []protocol.RunStatus{"halfway"},
	})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("ListRuns = %v, want invalid_params", err)
	}
}

// TestListRunsRefusesAnEmptyOrRepeatedStatusFilter proves the request SHAPE states
// the rule, not a handler: omitting statuses means every status, so an empty array is
// the one thing it cannot mean, and a repeat asks a set for something a set has no
// way to answer.
func TestListRunsRefusesAnEmptyOrRepeatedStatusFilter(t *testing.T) {
	if err := (protocol.ListRunsRequest{Statuses: []protocol.RunStatus{}}).ValidateWire(); err == nil {
		t.Error("an empty status filter validated")
	}
	if err := (protocol.ListRunsRequest{Statuses: []protocol.RunStatus{
		protocol.RunStatusRunning, protocol.RunStatusRunning,
	}}).ValidateWire(); err == nil {
		t.Error("a repeated status validated")
	}
	if err := (protocol.ListRunsRequest{}).ValidateWire(); err != nil {
		t.Errorf("an absent status filter = %v, want the whole history", err)
	}
}

// TestGetRunResolvesARunWithoutItsSession is runs.get's reason to exist: a client
// holding a runId from an event or a link asks what the run is without first
// discovering where it lives, and an id nobody owns is run_not_found rather than an
// empty shape the client would have to interpret.
func TestGetRunResolvesARunWithoutItsSession(t *testing.T) {
	s, rt := rollbackHarness(t)
	putRun(t, rt, "ses_1", "run_1", 10, 1)

	ref, err := s.GetRun(t.Context(), protocol.GetRunRequest{RunID: "run_1"})
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if ref.ID != "run_1" || ref.SessionID != "ses_1" || ref.Status != protocol.RunStatusFinished {
		t.Fatalf("run = %+v, want the finished run and the session it belongs to", ref)
	}

	if _, err := s.GetRun(t.Context(), protocol.GetRunRequest{RunID: "run_absent"}); !errors.Is(err, protocol.ErrRunNotFound) {
		t.Fatalf("GetRun(absent) = %v, want run_not_found", err)
	}
}

func TestChildRunReadsRequireNegotiatedSubagents(t *testing.T) {
	s, rt := rollbackHarness(t)
	putSession(t, rt, "ses_1")
	putRun(t, rt, "ses_1", "run_root", 10, 1)
	putChildRun(t, rt, "ses_1", "run_child", 11, 2)
	putUserItem(t, rt, "ses_1", "run_child", "it_child", "delegated")

	_, err := s.GetRun(t.Context(), protocol.GetRunRequest{RunID: "run_child"})
	assertSubagentCapabilityGap(t, "GetRun(child)", err)

	_, err = s.ListItems(t.Context(), protocol.ListItemsRequest{
		Scope: protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: "run_child"},
	})
	assertSubagentCapabilityGap(t, "ListItems(child)", err)

	// A session timeline is intentionally complete even for a Minimal client: the
	// Item shape is stable core, and hiding child history would falsify recovery.
	sessionPage, err := s.ListItems(t.Context(), protocol.ListItemsRequest{
		Scope: protocol.ItemListScope{Type: protocol.ItemScopeSession, SessionID: "ses_1"},
	})
	if err != nil || len(sessionPage.Data) != 1 || sessionPage.Data[0].RunID != "run_child" {
		t.Fatalf("ListItems(session) = (%+v, %v), want complete child history", sessionPage, err)
	}
	if len(sessionPage.Runs) != 2 ||
		sessionPage.Runs[0].ID != "run_child" ||
		sessionPage.Runs[0].SpawnedByItemID != "item_spawn" ||
		sessionPage.Runs[0].ParentRunID != "run_root" ||
		sessionPage.Runs[0].RootRunID != "run_root" ||
		sessionPage.Runs[1].ID != "run_root" {
		t.Fatalf("ListItems(session) run summaries = %+v, want child and its root ancestor", sessionPage.Runs)
	}

	optedIn := withClientCapabilities(protocol.ClientCapabilities{
		Features: map[string]protocol.FeaturePreference{
			protocol.FeatureSubagents: {Enabled: true},
		},
	})
	child, err := s.GetRun(optedIn, protocol.GetRunRequest{RunID: "run_child"})
	if err != nil || child.ID != "run_child" || child.RootRunID != "run_root" {
		t.Fatalf("GetRun(child, opted in) = (%+v, %v), want child lineage", child, err)
	}
	childItems, err := s.ListItems(optedIn, protocol.ListItemsRequest{
		Scope: protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: "run_child"},
	})
	if err != nil || len(childItems.Data) != 1 || childItems.Data[0].RunID != "run_child" {
		t.Fatalf("ListItems(child, opted in) = (%+v, %v), want child history", childItems, err)
	}
}

func TestChildRunCannotBecomeAnIndependentSubscriptionRoot(t *testing.T) {
	s, rt := rollbackHarness(t)
	putRun(t, rt, "ses_1", "run_root", 10, 1)
	putChildRun(t, rt, "ses_1", "run_child", 11, 2)

	_, _, err := s.SubscribeRun(t.Context(), protocol.SubscribeRunRequest{
		RunID: "run_child", SegmentID: "seg_child",
	})
	if !errors.Is(err, protocol.ErrRunNotRoot) {
		t.Fatalf("SubscribeRun(child) = %v, want run_not_root", err)
	}
}

func assertSubagentCapabilityGap(t *testing.T, operation string, err error) {
	t.Helper()
	var gap *protocol.CapabilityGap
	if !errors.As(err, &gap) {
		t.Fatalf("%s = %v, want typed capability gap", operation, err)
	}
	want := protocol.CapabilityRequirement{
		Type: protocol.RequirementFeature,
		Name: protocol.FeatureSubagents,
	}
	if len(gap.Requirements) != 1 || gap.Requirements[0] != want {
		t.Fatalf("%s requirements = %+v, want [%+v]", operation, gap.Requirements, want)
	}
}

func putChildRun(t *testing.T, rt *stubRuntime, sessionID, runID string, atUnix int64, mark int) {
	t.Helper()
	outcome := run.OutcomeCompleted
	if err := rt.runs.Restore(t.Context(), transcript.Run{
		SessionID: sessionID, ID: runID, SpawnedByItemID: "item_spawn",
		ParentRunID: "run_root", RootRunID: "run_root",
		State: run.Completed, Outcome: &outcome,
		CreatedAt: time.Unix(atUnix, 0).UTC(), FinishedAt: time.Unix(atUnix, 0).UTC(),
		UpdatedAt: time.Unix(atUnix, 0).UTC(), MessageMark: mark,
	}); err != nil {
		t.Fatalf("put child run %s: %v", runID, err)
	}
}

// TestListItemsReadsTheScopeItWasGiven is items.list's delivery half: the scope's
// tag decides which collection is read, and a subject that does not exist is refused
// in the words of the thing that is missing. A page of items also carries only the
// runs its own items belong to — a client merging pages rebuilds what it has read,
// and a long session does not ship its whole run list on every page.
func TestListItemsReadsTheScopeItWasGiven(t *testing.T) {
	s, rt := rollbackHarness(t)
	putSession(t, rt, "ses_1")
	putRun(t, rt, "ses_1", "run_1", 10, 1)
	putRun(t, rt, "ses_1", "run_2", 20, 2)
	putUserItem(t, rt, "ses_1", "run_1", "it_1", "first")
	putUserItem(t, rt, "ses_1", "run_2", "it_2", "second")

	whole, err := s.ListItems(t.Context(), protocol.ListItemsRequest{
		Scope: protocol.ItemListScope{Type: protocol.ItemScopeSession, SessionID: "ses_1"},
	})
	if err != nil {
		t.Fatalf("session scope: %v", err)
	}
	if len(whole.Data) != 2 || len(whole.Runs) != 2 {
		t.Fatalf("session page = %d items / %d runs, want both", len(whole.Data), len(whole.Runs))
	}

	scoped, err := s.ListItems(t.Context(), protocol.ListItemsRequest{
		Scope: protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: "run_2"},
	})
	if err != nil {
		t.Fatalf("run scope: %v", err)
	}
	if len(scoped.Data) != 1 || scoped.Data[0].ID != "it_2" {
		t.Fatalf("run page = %+v, want run_2's item", scoped.Data)
	}
	if len(scoped.Runs) != 1 || scoped.Runs[0].ID != "run_2" {
		t.Fatalf("run page runs = %+v, want only run_2", scoped.Runs)
	}
}

// TestListItemsRefusesAScopeItCannotServe covers the four ways a scope is wrong. The
// two not-found cases are the contract's explicit refusal to answer a bad id with an
// empty page: the client's next move differs, so the two subjects get two errors.
func TestListItemsRefusesAScopeItCannotServe(t *testing.T) {
	s, rt := rollbackHarness(t)
	putSession(t, rt, "ses_1")
	putRun(t, rt, "ses_1", "run_1", 10, 1)

	tests := []struct {
		name  string
		in    protocol.ListItemsRequest
		wants error
	}{{
		name:  "no scope at all",
		in:    protocol.ListItemsRequest{},
		wants: protocol.ErrInvalidParams,
	}, {
		name: "a scope kind the union does not define",
		in: protocol.ListItemsRequest{
			Scope: protocol.ItemListScope{Type: "everything"},
		},
		wants: protocol.ErrInvalidParams,
	}, {
		name: "a session scope with no session",
		in: protocol.ListItemsRequest{
			Scope: protocol.ItemListScope{Type: protocol.ItemScopeSession},
		},
		wants: protocol.ErrInvalidParams,
	}, {
		name: "an order that is not a direction",
		in: protocol.ListItemsRequest{
			Scope: protocol.ItemListScope{Type: protocol.ItemScopeSession, SessionID: "ses_1"},
			Order: "sideways",
		},
		wants: protocol.ErrInvalidParams,
	}, {
		name: "a session that does not exist",
		in: protocol.ListItemsRequest{
			Scope: protocol.ItemListScope{Type: protocol.ItemScopeSession, SessionID: "ses_gone"},
		},
		wants: protocol.ErrSessionNotFound,
	}, {
		name: "a run that does not exist",
		in: protocol.ListItemsRequest{
			Scope: protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: "run_gone"},
		},
		wants: protocol.ErrRunNotFound,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.ListItems(t.Context(), tt.in); !errors.Is(err, tt.wants) {
				t.Fatalf("ListItems = %v, want %v", err, tt.wants)
			}
		})
	}
}

// TestListItemsReadsBackwardWhenAsked is the tail-first read: the same durable
// sequence from the other end, which is what a long session's first screen needs.
func TestListItemsReadsBackwardWhenAsked(t *testing.T) {
	s, rt := rollbackHarness(t)
	putSession(t, rt, "ses_1")
	putRun(t, rt, "ses_1", "run_1", 10, 1)
	putUserItem(t, rt, "ses_1", "run_1", "it_1", "first")
	putUserItem(t, rt, "ses_1", "run_1", "it_2", "second")

	page, err := s.ListItems(t.Context(), protocol.ListItemsRequest{
		Scope: protocol.ItemListScope{Type: protocol.ItemScopeSession, SessionID: "ses_1"},
		Order: protocol.ItemOrderDesc,
	})
	if err != nil {
		t.Fatalf("descending page: %v", err)
	}
	if len(page.Data) != 2 || page.Data[0].ID != "it_2" {
		t.Fatalf("descending page = %+v, want the newest item first", page.Data)
	}
}

func TestSessionStatesPreservesInterruptReadFailure(t *testing.T) {
	want := errors.New("interrupt store unavailable")
	reader := &fakeInterruptReader{err: want}
	coordinator := sessions.New(sessions.Dependencies{Interrupts: reader, Admissions: new(admission.Gate)})
	if _, err := coordinator.Activities(t.Context(), []string{"ses_1", "ses_2"}); !errors.Is(err, want) {
		t.Fatalf("Activities error = %v, want interrupt read failure", err)
	}
}

func TestSessionStatesDoNotQueryInterruptsForActiveRun(t *testing.T) {
	reader := &fakeInterruptReader{err: errors.New("must not be read")}
	gate := &admission.Gate{}
	if _, ok := gate.AcquireSession("ses_1"); !ok {
		t.Fatal("AcquireSession rejected an empty registry")
	}
	coordinator := sessions.New(sessions.Dependencies{Interrupts: reader, Admissions: gate})
	activities, err := coordinator.Activities(t.Context(), []string{"ses_1"})
	if err != nil || activities["ses_1"] != sessions.ActivityRunning {
		t.Fatalf("Activities = (%q, %v), want running", activities["ses_1"], err)
	}
}

func TestListInterruptsProjectsToWire(t *testing.T) {
	created := time.Date(2026, 7, 5, 11, 0, 0, 0, time.UTC)
	arguments, err := tool.ArgumentsFromMap(map[string]any{"command": "go test ./...", "description": "Run server tests"})
	if err != nil {
		t.Fatalf("tool arguments: %v", err)
	}
	reader := &fakeInterruptReader{pending: []runs.Pending{
		{
			RootRunID: "run_waiting",
			SessionID: "ses_1",
			Interrupts: []transcript.Interrupt{{
				ItemID: "item_1", RunID: "run_child", Kind: interrupt.Approval,
				Approval: &transcript.Approval{
					Tool: transcript.ToolInvocation{Name: "shell", Arguments: arguments},
					Risk: tool.RiskHigh, Reason: "Runs commands in the workspace.", Rememberable: true,
				},
			}},
			CreatedAt: created,
		},
	}}
	s := &Server{queries: queries.New(queries.Dependencies{Interrupts: reader})}

	got, err := s.ListInterrupts(context.Background(), protocol.ListInterruptsRequest{SessionID: "ses_1"})
	if err != nil {
		t.Fatalf("list open interrupts: %v", err)
	}
	if reader.sessionID != "ses_1" {
		t.Fatalf("read session = %q, want ses_1", reader.sessionID)
	}
	if len(got.Data) != 1 {
		t.Fatalf("open interrupts = %+v, want one typed record", got.Data)
	}
	open := got.Data[0]
	if open.RootRunID != "run_waiting" || open.SessionID != "ses_1" || !open.CreatedAt.Equal(created) || len(open.Interrupts) != 1 {
		t.Fatalf("wire open interrupt = %+v", open)
	}
	interrupt := open.Interrupts[0]
	if interrupt.Type != protocol.InterruptApproval || interrupt.ItemID != "item_1" || interrupt.RunID != "run_child" ||
		interrupt.Payload == nil || interrupt.Payload.Tool == nil || !interrupt.Payload.Rememberable {
		t.Fatalf("wire interrupt = %+v, want typed approval payload", interrupt)
	}
	if interrupt.Payload.Tool.Name != "shell" || interrupt.Payload.Tool.Arguments["command"] != "go test ./..." {
		t.Fatalf("wire interrupt tool = %+v", interrupt.Payload.Tool)
	}
	if interrupt.Payload.Risk != protocol.ApprovalRiskHigh || interrupt.Payload.Reason != "Runs commands in the workspace." {
		t.Fatalf("wire interrupt risk/reason = %q/%q", interrupt.Payload.Risk, interrupt.Payload.Reason)
	}
}
