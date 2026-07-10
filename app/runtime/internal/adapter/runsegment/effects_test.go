package runsegment

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

func TestAfterLivePersistsTranscriptAndPublishesFileChange(t *testing.T) {
	stores := &fakeStores{transcript: &fakeTranscript{}, mark: 7}
	var published FilesChanged
	effects := New(Config{
		Stores: stores,
		PublishFileChanges: func(cwd string, paths []string) {
			published = FilesChanged{Cwd: cwd, Paths: paths}
		},
	})

	effects.AfterLive(context.Background(), Event{
		Item: &transcript.Item{SessionID: "ses_1", RunID: "run_1", ItemID: "item_1", Blob: []byte(`{"id":"item_1"}`)},
		Run: &RunRecord{
			Run:      transcript.Run{SessionID: "ses_1", RunID: "run_1", Blob: []byte(`{"id":"run_1"}`), Mark: -1},
			Terminal: true,
		},
		FilesChanged: &FilesChanged{Cwd: "/work", Paths: []string{"a.go"}},
	})

	if len(stores.transcript.items) != 1 || stores.transcript.items[0].ItemID != "item_1" {
		t.Fatalf("items = %+v, want item_1", stores.transcript.items)
	}
	if len(stores.transcript.runs) != 1 || stores.transcript.runs[0].Mark != 7 {
		t.Fatalf("runs = %+v, want one terminal run with mark 7", stores.transcript.runs)
	}
	if published.Cwd != "/work" || len(published.Paths) != 1 || published.Paths[0] != "a.go" {
		t.Fatalf("published = %+v, want /work [a.go]", published)
	}
}

func TestBeforeLiveRecordsInterruptWithProcessID(t *testing.T) {
	stores := &fakeStores{interrupts: &fakeInterrupts{}}
	effects := New(Config{Stores: stores, Processes: fakeProcess{processID: "proc_1"}})

	effects.BeforeLive(context.Background(), Event{Interrupt: &Interrupt{
		RunID:    "run_1",
		Handle:   turn.TurnHandle{SessionID: "ses_1", TurnID: "turn_1"},
		Provider: "anthropic",
		Model:    "claude",
		Payload:  []byte(`[{"id":"int_1"}]`),
		DrainedTools: []interrupts.DrainedTool{{
			ItemID: "tool_1",
			Name:   "ask_user",
		}},
	}})

	got := stores.interrupts.pending
	if got.ParentRunID != "run_1" || got.ProcessID != "proc_1" || got.Provider != "anthropic" || got.Model != "claude" {
		t.Fatalf("pending = %+v", got)
	}
	if string(got.Interrupts) != `[{"id":"int_1"}]` || len(got.DrainedTools) != 1 {
		t.Fatalf("pending payload = %s drained=%+v", got.Interrupts, got.DrainedTools)
	}
}

func TestBeforeLiveRejectsUnresumableInterrupt(t *testing.T) {
	want := errors.New("process snapshot unavailable")
	stores := &fakeStores{interrupts: &fakeInterrupts{}}
	effects := New(Config{Stores: stores, Processes: fakeProcess{err: want}})

	err := effects.BeforeLive(context.Background(), Event{Interrupt: &Interrupt{
		RunID:  "run_1",
		Handle: turn.TurnHandle{SessionID: "ses_1", TurnID: "turn_1"},
	}})
	if !errors.Is(err, want) {
		t.Fatalf("BeforeLive err = %v, want %v", err, want)
	}
	if stores.interrupts.pending.ParentRunID != "" {
		t.Fatalf("unresumable interrupt was persisted: %+v", stores.interrupts.pending)
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
	title      string
}

func (s *fakeStores) Interrupts() InterruptStore                        { return s.interrupts }
func (s *fakeStores) Session() SessionStore                             { return s.session }
func (s *fakeStores) Transcript() TranscriptStore                       { return s.transcript }
func (s *fakeStores) MessageCount(context.Context, string) (int, error) { return s.mark, nil }
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
