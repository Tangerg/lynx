package terminal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/programtest"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
	"github.com/Tangerg/lynx/app/cli/internal/promptqueue"
	"github.com/Tangerg/lynx/app/cli/internal/sessiontransfer"
	"github.com/Tangerg/lynx/app/cli/internal/workbench"
)

func TestRetiringSessionStateClearsOnlyTheRetiredSession(t *testing.T) {
	store, err := workbench.Open("", workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	queue := promptqueue.New()
	for _, sessionID := range []string{"retired", "active"} {
		if err := store.SaveDraft(sessionID, agent.Message{Text: sessionID + " draft"}); err != nil {
			t.Fatal(err)
		}
		if _, err := queue.Enqueue(sessionID, agent.Message{Text: sessionID + " queued"}); err != nil {
			t.Fatal(err)
		}
		approval := agent.Approval{
			RunID: sessionID + "_run", ItemID: sessionID + "_approval", Title: "Approve",
			Tool: &agent.ToolCall{Kind: agent.ToolRead, Name: "read", Path: "README.md", Status: agent.ToolRunning},
		}
		if err := store.StagePendingResume(sessionID, workbench.PendingResume{
			Command: agent.ResumeRun{
				CommandID: agent.CommandID("cli_" + map[string]string{
					"retired": "11111111111111111111111111111111",
					"active":  "22222222222222222222222222222222",
				}[sessionID]),
				RunID: approval.RunID,
				Answers: []agent.InterruptAnswer{{
					ItemID: approval.ItemID, Answer: agent.ApprovalAnswer{Decision: agent.ApprovalDeny},
				}},
			},
			Interactions: []agent.Interaction{approval},
		}); err != nil {
			t.Fatal(err)
		}
	}
	application := &app{workbench: store, queue: queue}

	discarded, err := application.retireSessionState("retired")
	if err != nil {
		t.Fatal(err)
	}
	if discarded != 1 {
		t.Fatalf("discarded queue entries = %d, want 1", discarded)
	}
	if draft, found, err := store.Draft("retired"); err != nil || found {
		t.Fatalf("retired draft = %+v, found %t, error %v", draft, found, err)
	}
	if entries := queue.Snapshot("retired").Entries; len(entries) != 0 {
		t.Fatalf("retired queue = %+v", entries)
	}
	if _, found := store.PendingResume("retired"); found {
		t.Fatal("retired pending resume remains")
	}
	if draft, found, err := store.Draft("active"); err != nil || !found || draft.Text != "active draft" {
		t.Fatalf("active draft = %+v, found %t, error %v", draft, found, err)
	}
	if entries := queue.Snapshot("active").Entries; len(entries) != 1 || entries[0].Message.Text != "active queued" {
		t.Fatalf("active queue = %+v", entries)
	}
	if _, found := store.PendingResume("active"); !found {
		t.Fatal("active pending resume was discarded")
	}
}

func TestRetiringSessionStateKeepsTheQueueWhenDurableRetirementFails(t *testing.T) {
	directory := t.TempDir()
	store, err := workbench.Open(directory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	const sessionID = "session"
	if err := store.SaveDraft(sessionID, agent.Message{Text: "keep authoring state"}); err != nil {
		t.Fatal(err)
	}
	queue := promptqueue.New()
	queued, err := queue.Enqueue(sessionID, agent.Message{Text: "keep queued prompt"})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(directory, "sessions"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("session state files = %d, %v", len(entries), err)
	}
	statePath := filepath.Join(directory, "sessions", entries[0].Name())
	if err := os.Remove(statePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(statePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(statePath, "blocker"), []byte("block deletion"), 0o600); err != nil {
		t.Fatal(err)
	}

	application := &app{workbench: store, queue: queue}
	discarded, err := application.retireSessionState(sessionID)
	if err == nil {
		t.Fatal("session retirement unexpectedly succeeded")
	}
	if discarded != 0 {
		t.Fatalf("failed retirement reported %d discarded prompts", discarded)
	}
	if got := queue.Snapshot(sessionID).Entries; len(got) != 1 || got[0].ID != queued.ID {
		t.Fatalf("failed retirement changed queue = %+v", got)
	}
	if draft, found, draftErr := store.Draft(sessionID); draftErr != nil || !found || draft.Text != "keep authoring state" {
		t.Fatalf("failed retirement changed draft = %+v, %v, %v", draft, found, draftErr)
	}
}

func TestParseRollbackArgumentPreservesTheInclusiveBoundaryAndScope(t *testing.T) {
	request, err := parseRollbackArgument("ses_1", "run_42 both")
	if err != nil {
		t.Fatal(err)
	}
	if request.SessionID != "ses_1" || request.ToRunID != "run_42" || request.Scope != agent.RestoreBoth {
		t.Fatalf("request = %+v", request)
	}
	all, err := parseRollbackArgument("ses_1", "all")
	if err != nil {
		t.Fatal(err)
	}
	if all.ToRunID != "" || all.Scope != agent.RestoreHistory {
		t.Fatalf("all request = %+v", all)
	}
	if _, err := parseRollbackArgument("ses_1", "all files"); err == nil {
		t.Fatal("file rollback without a boundary was accepted")
	}
}

func TestRollbackPreviewRejectsEverySessionRevisionChange(t *testing.T) {
	backend := mock.New()
	snapshot, err := backend.GetSession(t.Context(), "ses_demo_1")
	if err != nil {
		t.Fatal(err)
	}
	request := agent.RollbackSession{
		SessionID: snapshot.Session.ID, ToRunID: snapshot.Runs[0].ID, Scope: agent.RestoreFiles,
	}
	preview, err := previewRollback(snapshot, request)
	if err != nil {
		t.Fatal(err)
	}
	if err := preview.ValidateCommit(snapshot); err != nil {
		t.Fatalf("unchanged snapshot: %v", err)
	}
	snapshot.Session.Revision++
	if err := preview.ValidateCommit(snapshot); err == nil {
		t.Fatal("files-only rollback accepted a session changed after preview")
	}
}

func TestRollbackConfirmationSurvivesExtremeResizeAndRestoresOpeningInput(t *testing.T) {
	backend := mock.New()
	host, stop := runUIForSession(t, backend, "ses_demo_1")
	host.Shows(t, "Ask lyra")
	host.Type("/rollback all history")
	host.Press(input.Enter)
	host.Shows(t, "Rollback session")
	if !host.Resize(1, 1) || !host.Repaint() {
		t.Fatal("rollback dialog could not enter a minimal viewport")
	}
	if !host.Resize(96, 28) {
		t.Fatal("rollback dialog could not restore its viewport")
	}
	host.Shows(t, "Rollback session")
	host.Press(input.Down)
	host.Press(input.Enter)
	host.Shows(t, "1 runs removed")
	host.Shows(t, "Why is the cache expiry test flaky?")
	stop()
}

type blockedRollbackRuntime struct {
	*mock.Runtime

	entered chan struct{}
	release chan struct{}
}

func (runtime *blockedRollbackRuntime) RollbackSession(ctx context.Context, request agent.RollbackSession) (agent.RollbackResult, error) {
	select {
	case runtime.entered <- struct{}{}:
	default:
	}
	select {
	case <-runtime.release:
	case <-ctx.Done():
		return agent.RollbackResult{}, ctx.Err()
	}
	return runtime.Runtime.RollbackSession(ctx, request)
}

func TestRollbackKeepsRecoveredTextAndReportsItsPersistenceFailure(t *testing.T) {
	backend := &blockedRollbackRuntime{
		Runtime: mock.New(), entered: make(chan struct{}, 1), release: make(chan struct{}),
	}
	stateDirectory := t.TempDir()
	host, stop := runUIWithState(t, backend, "/tmp/lyra-cli-test", "ses_demo_1", stateDirectory)
	host.Shows(t, "Ask lyra")
	host.Type("/rollback all history")
	host.Press(input.Enter)
	host.Shows(t, "Rollback session")
	host.Press(input.Down)
	host.Press(input.Enter)
	select {
	case <-backend.entered:
	case <-time.After(time.Second):
		t.Fatal("rollback did not reach the runtime")
	}
	restoreWrites := blockSessionStateWrites(t, stateDirectory, "ses_demo_1")
	close(backend.release)

	host.Shows(t, "Why is the cache expiry test flaky?")
	host.Shows(t, "workbench:")
	restoreWrites()
	host.Type("!")
	awaitStoredDraft(t, stateDirectory, "ses_demo_1", agent.Message{Text: "Why is the cache expiry test flaky?!"})
	host.Shows(t, "rolled back 1 runs; restored text was not saved")
	host.Hides(t, "rolled back session · 1 runs removed")
	stop()
}

type importingTransfer struct{ runtime *mock.Runtime }

func (transfer importingTransfer) ExportSession(context.Context, sessiontransfer.ExportRequest) (sessiontransfer.Document, error) {
	return sessiontransfer.Document{}, errors.New("unexpected export")
}

func (transfer importingTransfer) ImportSession(ctx context.Context, request sessiontransfer.ImportRequest) (agent.Session, error) {
	if err := request.Validate(); err != nil {
		return agent.Session{}, err
	}
	return transfer.runtime.CreateSession(ctx, agent.CreateSession{Title: "Imported session", Workspace: "/tmp/lyra-imported"})
}

func TestImportRequiresConfirmationAndInstallsTheAuthoritativeSession(t *testing.T) {
	workspace := t.TempDir()
	artifact := filepath.Join(workspace, "portable.json")
	if err := os.WriteFile(artifact, []byte(`{"version":17}`), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := mock.New()
	host := programtest.New(t, 96, 28)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{
			Runtime: backend, Transfers: importingTransfer{runtime: backend}, Workspace: workspace, Host: host,
		})
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			if err := <-done; err != nil {
				t.Errorf("terminal session stopped with %v", err)
			}
		})
	}
	t.Cleanup(stop)

	host.Shows(t, "Ask lyra")
	host.Type("/import portable.json")
	host.Press(input.Enter)
	host.Shows(t, "Import session")
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("import confirmation did not survive a minimal viewport")
	}
	host.Shows(t, "Import session")
	host.Press(input.Down)
	host.Press(input.Enter)
	host.Shows(t, "Imported session")
	stop()
}

func TestConfirmationRejectsCallbacksFromAReplacedPresentation(t *testing.T) {
	transcript := testTranscriptView(t)
	application := &app{transcript: transcript}
	application.stack.SetBase(transcript)
	oldCalls, currentCalls := 0, 0
	application.confirmAction("Old confirmation", "Run the old action?", "Run old", func() { oldCalls++ })
	drawRoot(t, &application.stack, 96, 28)

	application.confirmAction("Current confirmation", "Run the current action?", "Run current", func() { currentCalls++ })
	application.stack.Handle(input.Key{Code: input.Down})
	application.stack.Handle(input.Key{Code: input.Enter})
	if oldCalls != 0 || currentCalls != 0 {
		t.Fatalf("stale presented confirmation executed callbacks: old=%d current=%d", oldCalls, currentCalls)
	}
	if application.confirmationDialog == nil || !application.confirmationDialog.Open() {
		t.Fatal("stale presented confirmation dismissed its replacement")
	}

	drawRoot(t, &application.stack, 96, 28)
	application.stack.Handle(input.Key{Code: input.Down})
	application.stack.Handle(input.Key{Code: input.Enter})
	if oldCalls != 0 || currentCalls != 1 {
		t.Fatalf("current confirmation callbacks: old=%d current=%d", oldCalls, currentCalls)
	}
}

func TestProjectionRetirementRejectsAPresentedConfirmationCallback(t *testing.T) {
	transcript := testTranscriptView(t)
	operations := newOperationOwner(t.Context())
	t.Cleanup(operations.Close)
	application := &app{
		transcript: transcript, operations: operations,
	}
	application.stack.SetBase(transcript)
	calls := 0
	application.confirmAction("Retired confirmation", "Run the retired action?", "Run", func() { calls++ })
	drawRoot(t, application, 96, 28)

	application.retireSessionContext()
	application.stack.Handle(input.Key{Code: input.Down})
	application.stack.Handle(input.Key{Code: input.Enter})
	if calls != 0 {
		t.Fatalf("retired projection confirmation executed %d callbacks", calls)
	}
	if application.confirmationDialog != nil || !application.stack.Empty() {
		t.Fatal("retired projection left its confirmation open")
	}
}

type steeringRuntime struct {
	*mock.Runtime

	mu      sync.Mutex
	request agent.SteerRun
	err     error
}

type blockedSteeringRuntime struct {
	*mock.Runtime

	entered chan agent.SteerRun
	release chan struct{}
	err     error
}

func (runtime *blockedSteeringRuntime) SteerRun(ctx context.Context, request agent.SteerRun) error {
	select {
	case runtime.entered <- request:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-runtime.release:
		return runtime.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (runtime *steeringRuntime) SteerRun(_ context.Context, request agent.SteerRun) error {
	runtime.mu.Lock()
	runtime.request = request
	runtime.mu.Unlock()
	return runtime.err
}

func (runtime *steeringRuntime) lastSteer() agent.SteerRun {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	request := runtime.request
	request.Message = request.Message.Clone()
	return request
}

type uncertainSteeringRuntime struct {
	*mock.Runtime

	mu       sync.Mutex
	requests []agent.SteerRun
}

func (runtime *uncertainSteeringRuntime) SteerRun(ctx context.Context, request agent.SteerRun) error {
	runtime.mu.Lock()
	runtime.requests = append(runtime.requests, request)
	attempt := len(runtime.requests)
	runtime.mu.Unlock()
	if attempt == 1 {
		if err := runtime.Runtime.SteerRun(ctx, request); err != nil {
			return err
		}
		return fmt.Errorf("lost steer acknowledgement: %w", context.DeadlineExceeded)
	}
	return nil
}

func (runtime *uncertainSteeringRuntime) steerAttempts() []agent.SteerRun {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return slices.Clone(runtime.requests)
}

func TestSteerTargetsTheObservedSegmentAndRestoresAttachmentsOnRefusal(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Event: agent.BlockStarted{Block: agent.Block{ID: "thinking", Kind: agent.BlockReasoning}}},
			{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	backend := &steeringRuntime{Runtime: base, err: agent.ErrStaleSegment}
	workspace := t.TempDir()
	attachment := filepath.Join(workspace, "notes.txt")
	if err := os.WriteFile(attachment, []byte("notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	host, stop := runUIWithWorkspace(t, backend, workspace)
	host.Shows(t, "Ask lyra")
	host.Type("start long work")
	host.Press(input.Enter)
	host.Shows(t, "thinking")
	host.Type("/attach notes.txt")
	host.Press(input.Enter)
	host.Shows(t, "attached notes.txt")
	host.Type("/steer focus on parsing")
	host.Press(input.Enter)
	host.Shows(t, "steer run failed")

	request := backend.lastSteer()
	if request.RunID == "" || request.SegmentID == "" || request.Message.Text != "focus on parsing" || len(request.Message.Attachments) != 1 {
		t.Fatalf("steer request = %+v", request)
	}
	host.Type("/attachments")
	host.Press(input.Enter)
	host.Shows(t, "notes.txt")
	stop()
}

func TestSteerReportsWhenRejectedAttachmentsCannotBePersisted(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Event: agent.BlockStarted{Block: agent.Block{ID: "thinking", Kind: agent.BlockReasoning}}},
			{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	backend := &blockedSteeringRuntime{
		Runtime: base, entered: make(chan agent.SteerRun, 1), release: make(chan struct{}), err: agent.ErrStaleSegment,
	}
	workspace := t.TempDir()
	stateDirectory := t.TempDir()
	attachmentPath := filepath.Join(workspace, "notes.txt")
	if err := os.WriteFile(attachmentPath, []byte("notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	host, stop := runUIWithState(t, backend, workspace, "", stateDirectory)
	host.Shows(t, "Ask lyra")
	sessionID := firstRuntimeSession(t, base)
	host.Type("start long work")
	host.Press(input.Enter)
	host.Shows(t, "thinking")
	host.Type("/attach notes.txt")
	host.Press(input.Enter)
	host.Shows(t, "attached notes.txt")
	var staged agent.Message
	host.Until(t, "the attachment draft to become durable", func() bool {
		var found bool
		var err error
		staged, found, err = storedDraft(stateDirectory, sessionID)
		return err == nil && found && len(staged.Attachments) == 1
	})

	host.Type("/steer focus on parsing")
	host.Press(input.Enter)
	select {
	case request := <-backend.entered:
		if request.Message.Text != "focus on parsing" || len(request.Message.Attachments) != 1 {
			t.Fatalf("steer request = %+v", request)
		}
	case <-time.After(time.Second):
		t.Fatal("steer did not reach the runtime")
	}
	host.Until(t, "the accepted steer draft ownership to be retired", func() bool {
		_, found, err := storedDraft(stateDirectory, sessionID)
		return err == nil && !found
	})
	restoreWrites := blockSessionStateWrites(t, stateDirectory, sessionID)
	close(backend.release)
	host.Shows(t, "workbench:")
	host.Shows(t, "@notes.txt")

	restoreWrites()
	host.Type("x")
	staged.Text = "x"
	awaitStoredDraft(t, stateDirectory, sessionID, staged)
	host.Shows(t, "steer run failed; restored attachments were not saved")
	stop()
}

func TestSteerConfirmsATimedOutAcknowledgementWithOneIdentity(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Event: agent.BlockStarted{Block: agent.Block{ID: "thinking", Kind: agent.BlockReasoning}}},
			{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	backend := &uncertainSteeringRuntime{Runtime: base}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("start steer confirmation")
	host.Press(input.Enter)
	host.Shows(t, "thinking")
	host.Type("/steer keep one identity")
	host.Press(input.Enter)
	host.Shows(t, "steer accepted")

	attempts := backend.steerAttempts()
	if len(attempts) != 2 || attempts[0].CommandID == "" || attempts[0].CommandID != attempts[1].CommandID ||
		attempts[0].RunID != attempts[1].RunID || attempts[0].SegmentID != attempts[1].SegmentID {
		t.Fatalf("steer confirmation attempts = %+v", attempts)
	}
	stop()
}
