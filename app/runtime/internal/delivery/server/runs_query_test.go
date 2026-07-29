package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/admission"
	"github.com/Tangerg/lynx/app/runtime/internal/application/queries"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// fakeInterruptReader backs the query coordinator's interrupt read for the
// ListOpenInterrupts wire-projection test.
type fakeInterruptReader struct {
	sessionID string
	pending   []interrupts.Pending
	err       error
}

func (r *fakeInterruptReader) List(_ context.Context, sessionID string) ([]interrupts.Pending, error) {
	r.sessionID = sessionID
	return r.pending, r.err
}

func (r *fakeInterruptReader) ListPage(ctx context.Context, sessionID string, _ int64, _ string, _ int) ([]interrupts.Pending, error) {
	return r.List(ctx, sessionID)
}

func (r *fakeInterruptReader) Get(_ context.Context, runID string) (interrupts.Pending, bool, error) {
	for _, pending := range r.pending {
		if pending.RunID == runID {
			return pending, true, r.err
		}
	}
	return interrupts.Pending{}, false, r.err
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
	if err := (protocol.ListRunsRequest{Statuses: []protocol.RunStatus{}}).Validate(); err == nil {
		t.Error("an empty status filter validated")
	}
	if err := (protocol.ListRunsRequest{Statuses: []protocol.RunStatus{
		protocol.RunStatusRunning, protocol.RunStatusRunning,
	}}).Validate(); err == nil {
		t.Error("a repeated status validated")
	}
	if err := (protocol.ListRunsRequest{}).Validate(); err != nil {
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

func TestSessionStatesPreservesInterruptReadFailure(t *testing.T) {
	want := errors.New("interrupt store unavailable")
	reader := &fakeInterruptReader{err: want}
	coordinator := sessions.New(sessions.Dependencies{Interrupts: reader, Admissions: new(admission.Gate)})
	if _, err := coordinator.SessionStates(t.Context(), []string{"ses_1", "ses_2"}); !errors.Is(err, want) {
		t.Fatalf("SessionStates error = %v, want interrupt read failure", err)
	}
}

func TestSessionStatesDoNotQueryInterruptsForActiveRun(t *testing.T) {
	reader := &fakeInterruptReader{err: errors.New("must not be read")}
	gate := &admission.Gate{}
	if _, ok := gate.AcquireSession("ses_1"); !ok {
		t.Fatal("AcquireSession rejected an empty registry")
	}
	coordinator := sessions.New(sessions.Dependencies{Interrupts: reader, Admissions: gate})
	states, err := coordinator.SessionStates(t.Context(), []string{"ses_1"})
	if err != nil || states["ses_1"] != sessions.SessionRunning {
		t.Fatalf("SessionStates = (%q, %v), want running", states["ses_1"], err)
	}
}

func TestListOpenInterruptsProjectsToWire(t *testing.T) {
	created := time.Date(2026, 7, 5, 11, 0, 0, 0, time.UTC)
	arguments, err := tool.ArgumentsFromMap(map[string]any{"command": "go test ./..."})
	if err != nil {
		t.Fatalf("tool arguments: %v", err)
	}
	reader := &fakeInterruptReader{pending: []interrupts.Pending{
		{
			RunID:     "run_waiting",
			SessionID: "ses_1",
			Interrupts: []transcript.Interrupt{{
				ItemID: "item_1", Kind: execution.ApprovalInterrupt,
				Approval: &transcript.Approval{
					Tool: transcript.ToolInvocation{Name: "shell", Arguments: arguments},
					Risk: tool.RiskHigh, Reason: "Runs commands in the workspace.", Rememberable: true,
				},
			}},
			CreatedAt: created,
		},
	}}
	s := &Server{queries: queries.New(queries.Dependencies{Interrupts: reader})}

	got, err := s.ListOpenInterrupts(context.Background(), protocol.ListOpenInterruptsRequest{SessionID: "ses_1"})
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
	if open.RunID != "run_waiting" || open.SessionID != "ses_1" || !open.CreatedAt.Equal(created) || len(open.Interrupts) != 1 {
		t.Fatalf("wire open interrupt = %+v", open)
	}
	interrupt := open.Interrupts[0]
	if interrupt.Type != protocol.InterruptApproval || interrupt.ItemID != "item_1" || interrupt.Payload == nil || interrupt.Payload.Tool == nil || !interrupt.Payload.Rememberable {
		t.Fatalf("wire interrupt = %+v, want typed approval payload", interrupt)
	}
	if interrupt.Payload.Tool.Name != "shell" || interrupt.Payload.Tool.Arguments["command"] != "go test ./..." {
		t.Fatalf("wire interrupt tool = %+v", interrupt.Payload.Tool)
	}
	if interrupt.Payload.Risk != protocol.ApprovalRiskHigh || interrupt.Payload.Reason != "Runs commands in the workspace." {
		t.Fatalf("wire interrupt risk/reason = %q/%q", interrupt.Payload.Risk, interrupt.Payload.Reason)
	}
}
