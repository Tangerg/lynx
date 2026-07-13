package runsegment

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// TestCommitEventPersistsTranscriptAndTerminalizes: a terminal commit persists
// the item + run projection (resolving the terminal message watermark) AND
// terminalizes the run-state — all through one CommitEvent, atomically inside the
// wired transactor.
func TestCommitEventPersistsTranscriptAndTerminalizes(t *testing.T) {
	stores := &fakeStores{transcript: &fakeTranscript{}, mark: 7}
	runState := &fakeRunState{}
	tx := &fakeTx{}
	effects := New(Config{Stores: stores, RunState: runState, Tx: tx.run})

	err := effects.CommitEvent(t.Context(), runs.EventCommit{
		RunID:     "run_1",
		SessionID: "ses_1",
		State:     runs.StateTerminalize,
		Outcome:   execution.OutcomeCompleted,
		Item:      &transcript.Item{SessionID: "ses_1", RunID: "run_1", ID: "item_1"},
		Run:       &transcript.Run{SessionID: "ses_1", ID: "run_1", MessageMark: -1},
	})
	if err != nil {
		t.Fatalf("CommitEvent: %v", err)
	}

	if len(stores.transcript.items) != 1 || stores.transcript.items[0].ID != "item_1" {
		t.Fatalf("items = %+v, want item_1", stores.transcript.items)
	}
	if len(stores.transcript.runs) != 1 || stores.transcript.runs[0].MessageMark != 7 {
		t.Fatalf("runs = %+v, want one run with resolved mark 7", stores.transcript.runs)
	}
	if len(runState.terminalized) != 1 || runState.terminalized[0] != "ses_1:run_1:completed" {
		t.Fatalf("terminalized = %v, want [ses_1:run_1:completed]", runState.terminalized)
	}
	if tx.calls != 1 {
		t.Fatalf("RunInTx calls = %d, want 1 (the whole commit is one transaction)", tx.calls)
	}
}

func TestCommitEventRejectsUnresolvedTerminalMessageWatermark(t *testing.T) {
	want := errors.New("message count unavailable")
	stores := &fakeStores{transcript: &fakeTranscript{}, markErr: want}
	runState := &fakeRunState{}
	effects := New(Config{Stores: stores, RunState: runState, Tx: new(fakeTx).run})

	err := effects.CommitEvent(t.Context(), runs.EventCommit{
		RunID:     "run_1",
		SessionID: "ses_1",
		State:     runs.StateTerminalize,
		Outcome:   execution.OutcomeCompleted,
		Run:       &transcript.Run{SessionID: "ses_1", ID: "run_1", MessageMark: -1},
	})
	if !errors.Is(err, want) {
		t.Fatalf("CommitEvent error = %v, want %v", err, want)
	}
	if len(stores.transcript.runs) != 0 {
		t.Fatalf("runs = %+v, want none after unresolved terminal watermark", stores.transcript.runs)
	}
	if len(runState.terminalized) != 0 {
		t.Fatalf("terminalized = %v, want none", runState.terminalized)
	}
}

func TestCommitOpeningAdmitsAndProjectsInOneTransaction(t *testing.T) {
	stores := &fakeStores{transcript: &fakeTranscript{}}
	runState := &fakeRunState{}
	tx := &fakeTx{}
	effects := New(Config{Stores: stores, RunState: runState, Tx: tx.run})
	draft := execution.RunDraft{RunID: "run_1", SessionID: "ses_1"}

	err := effects.CommitOpening(context.Background(), runs.OpeningCommit{
		Admit: &draft,
		Events: []runs.EventCommit{{
			RunID:     "run_1",
			SessionID: "ses_1",
			Run:       &transcript.Run{SessionID: "ses_1", ID: "run_1"},
		}},
	})
	if err != nil {
		t.Fatalf("CommitOpening: %v", err)
	}
	if tx.calls != 1 || len(runState.admitted) != 1 || len(stores.transcript.runs) != 1 {
		t.Fatalf("opening tx=%d admitted=%d runs=%d, want 1/1/1", tx.calls, len(runState.admitted), len(stores.transcript.runs))
	}
}

func TestCommitOpeningConsumesInterruptAndResumes(t *testing.T) {
	ints := &fakeInterrupts{pending: interrupts.Pending{RunID: "run_1", SessionID: "ses_1"}}
	stores := &fakeStores{interrupts: ints, transcript: &fakeTranscript{}}
	runState := &fakeRunState{}
	tx := &fakeTx{}
	effects := New(Config{Stores: stores, RunState: runState, Tx: tx.run})
	resume := execution.ResumeDraft{RunID: "run_1", SessionID: "ses_1"}

	err := effects.CommitOpening(context.Background(), runs.OpeningCommit{
		Resume: &resume,
		Events: []runs.EventCommit{{
			RunID:     "run_1",
			SessionID: "ses_1",
			Run:       &transcript.Run{SessionID: "ses_1", ID: "run_1"},
		}},
	})
	if err != nil {
		t.Fatalf("CommitOpening: %v", err)
	}
	if tx.calls != 1 || ints.pending.RunID != "" || len(runState.resumed) != 1 || len(stores.transcript.runs) != 1 {
		t.Fatalf("resume tx=%d pending=%+v resumed=%v runs=%d", tx.calls, ints.pending, runState.resumed, len(stores.transcript.runs))
	}
}

// TestCommitEventRecordsInterruptAndSuspends: a park commit resolves the
// interrupt's process id from the live turn, persists the resumable record, and
// suspends the run-state — atomically.
func TestCommitEventRecordsInterruptAndSuspends(t *testing.T) {
	stores := &fakeStores{interrupts: &fakeInterrupts{}}
	runState := &fakeRunState{}
	tx := &fakeTx{}
	effects := New(Config{Stores: stores, Processes: fakeProcess{processID: "proc_1"}, RunState: runState, Tx: tx.run})

	err := effects.CommitEvent(context.Background(), runs.EventCommit{
		RunID:     "run_1",
		SessionID: "ses_1",
		State:     runs.StateSuspend,
		Interrupt: &interrupts.Pending{
			RunID:        "run_1",
			SessionID:    "ses_1",
			TurnID:       "turn_1",
			Provider:     "anthropic",
			Model:        "claude",
			Interrupts:   []transcript.Interrupt{{ItemID: "int_1", Kind: transcript.QuestionInterrupt}},
			DrainedTools: []interrupts.DrainedTool{{ItemID: "tool_1", Name: "ask_user"}},
		},
	})
	if err != nil {
		t.Fatalf("CommitEvent: %v", err)
	}

	got := stores.interrupts.pending
	if got.RunID != "run_1" || got.ProcessID != "proc_1" || got.Provider != "anthropic" || got.Model != "claude" {
		t.Fatalf("pending = %+v", got)
	}
	if len(got.Interrupts) != 1 || got.Interrupts[0].ItemID != "int_1" || len(got.DrainedTools) != 1 {
		t.Fatalf("pending interrupts = %+v drained=%+v", got.Interrupts, got.DrainedTools)
	}
	if len(runState.suspended) != 1 || runState.suspended[0] != "ses_1:run_1" {
		t.Fatalf("suspended = %v, want [ses_1:run_1]", runState.suspended)
	}
}

// TestCommitEventRejectsUnresumableInterrupt: an unresolvable process id fails the
// commit before the transaction — nothing is persisted and the run-state is not
// suspended.
func TestCommitEventRejectsUnresumableInterrupt(t *testing.T) {
	want := errors.New("process snapshot unavailable")
	stores := &fakeStores{interrupts: &fakeInterrupts{}}
	runState := &fakeRunState{}
	tx := &fakeTx{}
	effects := New(Config{Stores: stores, Processes: fakeProcess{err: want}, RunState: runState, Tx: tx.run})

	err := effects.CommitEvent(context.Background(), runs.EventCommit{
		RunID:     "run_1",
		SessionID: "ses_1",
		State:     runs.StateSuspend,
		Interrupt: &interrupts.Pending{RunID: "run_1", SessionID: "ses_1", TurnID: "turn_1"},
	})
	if !errors.Is(err, want) {
		t.Fatalf("CommitEvent err = %v, want %v", err, want)
	}
	if stores.interrupts.pending.RunID != "" {
		t.Fatalf("unresumable interrupt was persisted: %+v", stores.interrupts.pending)
	}
	if len(runState.suspended) != 0 {
		t.Fatalf("suspended = %v, want none (commit rejected before the transaction)", runState.suspended)
	}
}

// TestNudgePublishesFileChange: the non-durable workspace nudge reaches the
// publisher.
func TestNudgePublishesFileChange(t *testing.T) {
	var published struct {
		cwd   string
		paths []string
	}
	effects := New(Config{PublishFileChanges: func(cwd string, paths []string) {
		published.cwd, published.paths = cwd, paths
	}})

	effects.Nudge("/work", []string{"a.go"})
	if published.cwd != "/work" || len(published.paths) != 1 || published.paths[0] != "a.go" {
		t.Fatalf("published = %+v, want /work [a.go]", published)
	}
}

func TestFinishRunsTerminalMaintenanceOnlyForTerminalRuns(t *testing.T) {
	renamed := make(chan string, 1)
	snapshotted := make(chan string, 1)
	stores := &fakeStores{
		session: &fakeSession{
			sess:    session.Session{ID: "ses_1", Cwd: "/repo"},
			renamed: renamed,
		},
		title: "Generated title",
	}
	effects := New(Config{
		Stores:      stores,
		Checkpoints: fakeCheckpoints{snapshotted: snapshotted},
	})

	effects.Finish(context.Background(), runs.Finish{SessionID: "ses_1", RunID: "run_1", OpeningUserText: "hello"})

	if got := waitString(t, snapshotted); got != "ses_1:/repo:run_1" {
		t.Fatalf("snapshot = %q", got)
	}
	if got := waitString(t, renamed); got != "Generated title" {
		t.Fatalf("title = %q", got)
	}

	effects.Finish(context.Background(), runs.Finish{SessionID: "ses_1", RunID: "run_2", Parked: true, OpeningUserText: "ignored"})
	select {
	case got := <-snapshotted:
		t.Fatalf("parked run must not snapshot, got %q", got)
	case got := <-renamed:
		t.Fatalf("parked run must not title, got %q", got)
	case <-time.After(20 * time.Millisecond):
	}
}

func waitString(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case got := <-ch:
		return got
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for async side effect")
		return ""
	}
}

type fakeStores struct {
	transcript *fakeTranscript
	interrupts *fakeInterrupts
	session    *fakeSession
	mark       int
	markErr    error
	title      string
}

func (s *fakeStores) Interrupts() InterruptStore  { return s.interrupts }
func (s *fakeStores) Session() SessionStore       { return s.session }
func (s *fakeStores) Transcript() TranscriptStore { return s.transcript }
func (s *fakeStores) MessageCount(context.Context, string) (int, error) {
	return s.mark, s.markErr
}
func (s *fakeStores) GenerateTitle(context.Context, string) (string, error) {
	return s.title, nil
}

type fakeProcess struct {
	processID string
	err       error
}

func (p fakeProcess) ProcessID(context.Context, turn.TurnHandle) (string, error) {
	return p.processID, p.err
}

// fakeRunState records the run-state transitions the commit applies.
type fakeRunState struct {
	admitted     []execution.RunDraft
	resumed      []string
	suspended    []string
	terminalized []string
}

func (r *fakeRunState) Admit(_ context.Context, draft execution.RunDraft) error {
	r.admitted = append(r.admitted, draft)
	return nil
}

func (r *fakeRunState) Resume(_ context.Context, draft execution.ResumeDraft) error {
	r.resumed = append(r.resumed, draft.SessionID)
	return nil
}

func (r *fakeRunState) Suspend(_ context.Context, sessionID, runID string) error {
	r.suspended = append(r.suspended, sessionID+":"+runID)
	return nil
}

func (r *fakeRunState) Terminalize(_ context.Context, sessionID, runID string, o execution.Outcome) error {
	r.terminalized = append(r.terminalized, sessionID+":"+runID+":"+o.String())
	return nil
}

// fakeTx records how many transactions the commit opens and runs the body inline.
type fakeTx struct{ calls int }

func (t *fakeTx) run(ctx context.Context, fn func(context.Context) error) error {
	t.calls++
	return fn(ctx)
}

type fakeTranscript struct {
	items []transcript.Item
	runs  []transcript.Run
}

func (s *fakeTranscript) AppendItem(_ context.Context, it transcript.Item) error {
	s.items = append(s.items, it)
	return nil
}

func (s *fakeTranscript) PutRun(_ context.Context, r transcript.Run) error {
	s.runs = append(s.runs, r)
	return nil
}

type fakeInterrupts struct {
	pending interrupts.Pending
}

func (s *fakeInterrupts) Put(_ context.Context, p interrupts.Pending) error {
	s.pending = p
	return nil
}

func (s *fakeInterrupts) Consume(_ context.Context, runID string) (interrupts.Pending, bool, error) {
	if s.pending.RunID != runID {
		return interrupts.Pending{}, false, nil
	}
	pending := s.pending
	s.pending = interrupts.Pending{}
	return pending, true, nil
}

type fakeSession struct {
	sess    session.Session
	renamed chan string
}

func (s *fakeSession) List(context.Context) ([]session.Session, error) { return nil, nil }

func (s *fakeSession) Get(_ context.Context, id string) (session.Session, error) {
	if id != s.sess.ID {
		return session.Session{}, session.ErrNotFound
	}
	return s.sess, nil
}

func (s *fakeSession) RenameIfUntitled(_ context.Context, id, title string) error {
	if id != s.sess.ID {
		return session.ErrNotFound
	}
	s.renamed <- title
	return nil
}

type fakeCheckpoints struct {
	snapshotted chan<- string
}

func (c fakeCheckpoints) Snapshot(_ context.Context, sessionID, cwd, runID string) error {
	c.snapshotted <- sessionID + ":" + cwd + ":" + runID
	return nil
}
