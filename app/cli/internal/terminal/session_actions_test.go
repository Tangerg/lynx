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

	"github.com/Tangerg/scope/app/cli/internal/agent"
	"github.com/Tangerg/scope/app/cli/internal/agent/mock"
	"github.com/Tangerg/scope/app/cli/internal/promptqueue"
	"github.com/Tangerg/scope/app/cli/internal/runtimeprofile"
	"github.com/Tangerg/scope/app/cli/internal/sessiontransfer"
	"github.com/Tangerg/scope/app/cli/internal/workbench"
)

type postCommitSessionDeleteRuntime struct {
	agent.Runtime
	mu      sync.Mutex
	request agent.DeleteSession
	calls   int
	deleted chan struct{}
}

func (p *postCommitSessionDeleteRuntime) DeleteSession(ctx context.Context, request agent.DeleteSession) error {
	if err := p.Runtime.DeleteSession(ctx, request); err != nil {
		return err
	}
	p.mu.Lock()
	p.request = request
	p.calls++
	p.mu.Unlock()
	select {
	case p.deleted <- struct{}{}:
	default:
	}
	return errors.New("runtime cleanup failed after durable deletion")
}

func (p *postCommitSessionDeleteRuntime) deletion() (agent.DeleteSession, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.request, p.calls
}

func TestRetiringSessionStateClearsOnlyTheRetiredSession(t *testing.T) {
	store, err := workbench.Open("", workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	queue := promptqueue.New()
	for _, sessionID := range []string{"retired", "active"} {
		if saveDraftErr := store.SaveDraft(sessionID, agent.Message{Text: sessionID + " draft"}); saveDraftErr != nil {
			t.Fatal(saveDraftErr)
		}
		if _, enqueueErr := queue.Enqueue(sessionID, agent.Message{Text: sessionID + " queued"}); enqueueErr != nil {
			t.Fatal(enqueueErr)
		}
		approval := agent.Approval{
			RunID: sessionID + "_run", ItemID: sessionID + "_approval", Title: "Approve",
			Tool: &agent.ToolCall{Kind: agent.ToolRead, Name: "read", Path: "README.md", Status: agent.ToolRunning},
		}
		if stagePendingResumeErr := store.StagePendingResume(sessionID, workbench.PendingResume{
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
		}); stagePendingResumeErr != nil {
			t.Fatal(stagePendingResumeErr)
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

func TestRetiringSessionStateClearsTheQueueAfterDurableTombstone(t *testing.T) {
	directory := t.TempDir()
	store, err := workbench.Open(directory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	const sessionID = "session"
	if saveDraftErr := store.SaveDraft(sessionID, agent.Message{Text: "keep authoring state"}); saveDraftErr != nil {
		t.Fatal(saveDraftErr)
	}
	queue := promptqueue.New()
	_, err = queue.Enqueue(sessionID, agent.Message{Text: "keep queued prompt"})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(directory, "sessions"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("session state files = %d, %v", len(entries), err)
	}
	statePath := filepath.Join(directory, "sessions", entries[0].Name())
	if removeErr := os.Remove(statePath); removeErr != nil {
		t.Fatal(removeErr)
	}
	if mkdirErr := os.Mkdir(statePath, 0o700); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}
	if writeFileErr := os.WriteFile(filepath.Join(statePath, "blocker"), []byte("block deletion"), 0o600); writeFileErr != nil {
		t.Fatal(writeFileErr)
	}

	application := &app{workbench: store, queue: queue}
	discarded, err := application.retireSessionState(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if discarded != 1 {
		t.Fatalf("retirement reported %d discarded prompts", discarded)
	}
	if got := queue.Snapshot(sessionID).Entries; len(got) != 0 {
		t.Fatalf("retirement left queue = %+v", got)
	}
	if draft, found, draftErr := store.Draft(sessionID); draftErr != nil || found {
		t.Fatalf("retirement left draft = %+v, %v, %v", draft, found, draftErr)
	}
	if deletions := store.PendingSessionDeletions(); len(deletions) != 1 ||
		deletions[0].SessionID != sessionID || deletions[0].Phase != workbench.SessionDeletionConfirmed {
		t.Fatalf("retirement tombstone = %+v", deletions)
	}
}

func TestSessionCenterConvergesPostCommitDeleteFailureAndRetiresLocalState(t *testing.T) {
	base := mock.New()
	workspace := t.TempDir()
	target, err := base.CreateSession(t.Context(), agent.CreateSession{Title: "Post-commit delete target", Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	stateDirectory := t.TempDir()
	store, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if saveDraftErr := store.SaveDraft(target.ID, agent.Message{Text: "must not survive deletion"}); saveDraftErr != nil {
		t.Fatal(saveDraftErr)
	}
	backend := &postCommitSessionDeleteRuntime{Runtime: base, deleted: make(chan struct{}, 1)}
	host, stop := runUIFromConfig(t, Config{
		Runtime: backend, SessionID: "ses_demo_1", StateDirectory: stateDirectory,
	})
	host.Shows(t, "Ask lyra")
	host.Send(input.Key{Code: input.Character, Rune: 'r', Mods: input.Ctrl})
	host.Shows(t, "Sessions · Center")
	host.Type(target.Title)
	host.Shows(t, target.Title)
	host.Send(input.Key{Code: input.Character, Rune: 'd', Mods: input.Alt})
	host.Shows(t, "Delete session")
	for range 2 {
		host.Press(input.Down)
	}
	host.Press(input.Enter)
	awaitSignal(t, backend.deleted, "post-commit session deletion")
	host.Shows(t, "No loaded sessions")
	stop()

	request, calls := backend.deletion()
	if calls != 1 || request.SessionID != target.ID || request.CommandID == "" {
		t.Fatalf("runtime deletion = %+v, calls %d", request, calls)
	}
	if _, getSessionErr := base.GetSession(t.Context(), target.ID); !errors.Is(getSessionErr, agent.ErrSessionNotFound) {
		t.Fatalf("deleted session read = %v", getSessionErr)
	}
	reopened, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if draft, found, err := reopened.Draft(target.ID); err != nil || found {
		t.Fatalf("deleted session draft = %+v, %t, %v", draft, found, err)
	}
	if pending := reopened.PendingSessionDeletions(); len(pending) != 0 {
		t.Fatalf("settled session deletions = %+v", pending)
	}
}

func TestStartupReplaysPreparedSessionDeletionBeforeLoadingDrafts(t *testing.T) {
	backend := mock.New()
	workspace := t.TempDir()
	target, err := backend.CreateSession(t.Context(), agent.CreateSession{Title: "Interrupted delete target", Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	stateDirectory := t.TempDir()
	store, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if saveDraftErr := store.SaveDraft(target.ID, agent.Message{Text: "orphaned draft"}); saveDraftErr != nil {
		t.Fatal(saveDraftErr)
	}
	request := agent.DeleteSession{
		CommandID: agent.CommandID("cli_66666666666666666666666666666666"), SessionID: target.ID,
	}
	if stageSessionDeletionErr := store.StageSessionDeletion(request, workbench.ReplayGuard{}); stageSessionDeletionErr != nil {
		t.Fatal(stageSessionDeletionErr)
	}

	host, stop := runUIFromConfig(t, Config{
		Runtime: backend, SessionID: "ses_demo_1", StateDirectory: stateDirectory,
	})
	host.Shows(t, "Ask lyra")
	stop()
	if _, getSessionErr := backend.GetSession(t.Context(), target.ID); !errors.Is(getSessionErr, agent.ErrSessionNotFound) {
		t.Fatalf("recovered deletion read = %v", getSessionErr)
	}
	reopened, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if draft, found, err := reopened.Draft(target.ID); err != nil || found {
		t.Fatalf("recovered deletion draft = %+v, %t, %v", draft, found, err)
	}
	if pending := reopened.PendingSessionDeletions(); len(pending) != 0 {
		t.Fatalf("recovered deletion journals = %+v", pending)
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

func TestRollbackPreviewProvesOnlyTheExactHistoryOutcome(t *testing.T) {
	backend := mock.New()
	before, err := backend.GetSession(t.Context(), "ses_demo_1")
	if err != nil {
		t.Fatal(err)
	}
	request := agent.RollbackSession{SessionID: before.Session.ID, Scope: agent.RestoreHistory}
	preview, err := previewRollback(before, request)
	if err != nil {
		t.Fatal(err)
	}
	if validateAppliedErr := preview.ValidateApplied(before); validateAppliedErr == nil {
		t.Fatal("unchanged session was accepted as a committed rollback")
	}
	if _, rollbackSessionErr := backend.RollbackSession(t.Context(), request); rollbackSessionErr != nil {
		t.Fatal(rollbackSessionErr)
	}
	after, err := backend.GetSession(t.Context(), request.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if validateAppliedErr := preview.ValidateApplied(after); validateAppliedErr != nil {
		t.Fatalf("authoritative rollback outcome: %v", validateAppliedErr)
	}
	wrong := after
	wrong.Runs = slices.Clone(after.Runs)
	wrong.Runs = append(wrong.Runs, before.Runs[len(before.Runs)-1])
	if validateAppliedErr := preview.ValidateApplied(wrong); validateAppliedErr == nil {
		t.Fatal("rollback outcome with a surviving dropped run was accepted")
	}
	files, err := previewRollback(before, agent.RollbackSession{
		SessionID: before.Session.ID, ToRunID: before.Runs[0].ID, Scope: agent.RestoreFiles,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := files.ValidateApplied(after); err == nil {
		t.Fatal("files-only rollback was inferred from a session projection")
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

type committedThenCanceledRollbackRuntime struct {
	*mock.Runtime

	committed chan struct{}
	once      sync.Once
}

func (c *committedThenCanceledRollbackRuntime) RollbackSession(
	ctx context.Context,
	request agent.RollbackSession,
) (agent.RollbackResult, error) {
	if _, err := c.Runtime.RollbackSession(ctx, request); err != nil {
		return agent.RollbackResult{}, err
	}
	c.once.Do(func() { close(c.committed) })
	<-ctx.Done()
	return agent.RollbackResult{}, context.Cause(ctx)
}

type postCommitRollbackRuntime struct {
	*mock.Runtime

	mu      sync.Mutex
	request agent.RollbackSession
}

func (p *postCommitRollbackRuntime) RollbackSession(
	ctx context.Context,
	request agent.RollbackSession,
) (agent.RollbackResult, error) {
	result, err := p.Runtime.RollbackSession(ctx, request)
	if err != nil {
		return result, err
	}
	p.mu.Lock()
	p.request = request
	p.mu.Unlock()
	return agent.RollbackResult{}, errors.New("runtime cleanup failed after durable rollback")
}

func (p *postCommitRollbackRuntime) rollbackRequest() agent.RollbackSession {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.request
}

func TestRollbackConvergesPostCommitFailureAndRestoresOpeningInput(t *testing.T) {
	backend := &postCommitRollbackRuntime{Runtime: mock.New()}
	host, stop := runUIForSession(t, backend, "ses_demo_1")
	host.Shows(t, "Ask lyra")
	host.Type("/rollback all history")
	host.Press(input.Enter)
	host.Shows(t, "Rollback session")
	host.Press(input.Down)
	host.Press(input.Enter)
	host.Shows(t, "1 runs removed")
	host.Shows(t, "Why is the cache expiry test flaky?")
	host.Shows(t, "runtime cleanup warning")
	request := backend.rollbackRequest()
	if request.SessionID != "ses_demo_1" || request.CommandID == "" {
		t.Fatalf("rollback request = %+v", request)
	}
	stop()
}

func TestRollbackPreservesDraftAuthoredWhileTheRuntimeSettles(t *testing.T) {
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
	awaitSignal(t, backend.entered, "rollback runtime call")
	host.Shows(t, "rolling back session")
	host.Type("new thought")
	host.Shows(t, "new thought")
	close(backend.release)
	host.Shows(t, "Why is the cache expiry test flaky?")
	host.Shows(t, "new thought")
	awaitStoredDraft(t, stateDirectory, "ses_demo_1", agent.Message{
		Text: "Why is the cache expiry test flaky?\n\nnew thought",
	})
	stop()
}

func TestRestartRecoversCommittedRollbackAndOpeningInput(t *testing.T) {
	stateDirectory := t.TempDir()
	underlying := mock.New()
	backend := &committedThenCanceledRollbackRuntime{
		Runtime: underlying, committed: make(chan struct{}),
	}
	host, stop := runUIWithState(t, backend, "/tmp/lyra-cli-test", "ses_demo_1", stateDirectory)
	host.Shows(t, "Ask lyra")
	host.Type("/rollback all history")
	host.Press(input.Enter)
	host.Shows(t, "Rollback session")
	host.Press(input.Down)
	host.Press(input.Enter)
	awaitSignal(t, backend.committed, "durable runtime rollback")
	stop()

	restarted, stopRestarted := runUIWithState(
		t, underlying, "/tmp/lyra-cli-test", "ses_demo_1", stateDirectory,
	)
	restarted.Shows(t, "Why is the cache expiry test flaky?")
	restarted.Shows(t, "recovered rollback input · 1 runs removed")
	reopened, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if pending := reopened.PendingSessionRollbacks(); len(pending) != 0 {
		t.Fatalf("settled rollback journals = %+v", pending)
	}
	stopRestarted()
}

func (b *blockedRollbackRuntime) RollbackSession(ctx context.Context, request agent.RollbackSession) (agent.RollbackResult, error) {
	select {
	case b.entered <- struct{}{}:
	default:
	}
	select {
	case <-b.release:
	case <-ctx.Done():
		return agent.RollbackResult{}, ctx.Err()
	}
	return b.Runtime.RollbackSession(ctx, request)
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

func (i importingTransfer) ExportSession(context.Context, sessiontransfer.ExportRequest) (sessiontransfer.Document, error) {
	return sessiontransfer.Document{}, errors.New("unexpected export")
}

func (i importingTransfer) ImportSession(ctx context.Context, request sessiontransfer.ImportRequest) (agent.Session, error) {
	if err := request.Validate(); err != nil {
		return agent.Session{}, err
	}
	return i.runtime.CreateSession(ctx, agent.CreateSession{Title: "Imported session", Workspace: "/tmp/lyra-imported"})
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

func (b *blockedSteeringRuntime) SteerRun(ctx context.Context, request agent.SteerRun) error {
	select {
	case b.entered <- request:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-b.release:
		return b.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *steeringRuntime) SteerRun(_ context.Context, request agent.SteerRun) error {
	s.mu.Lock()
	s.request = request
	s.mu.Unlock()
	return s.err
}

func (s *steeringRuntime) lastSteer() agent.SteerRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	request := s.request
	request.Message = request.Message.Clone()
	return request
}

type uncertainSteeringRuntime struct {
	*mock.Runtime

	mu       sync.Mutex
	requests []agent.SteerRun
}

type committedThenCanceledSteeringRuntime struct {
	*mock.Runtime

	committed chan agent.SteerRun
}

func (c *committedThenCanceledSteeringRuntime) SteerRun(
	ctx context.Context,
	request agent.SteerRun,
) error {
	if err := c.Runtime.SteerRun(ctx, request); err != nil {
		return err
	}
	select {
	case c.committed <- request.Clone():
	default:
	}
	<-ctx.Done()
	return context.Cause(ctx)
}

type cachedSteeringRuntime struct {
	*mock.Runtime

	accepted agent.SteerRun
	attempts []agent.SteerRun
}

func (c *cachedSteeringRuntime) SteerRun(_ context.Context, request agent.SteerRun) error {
	c.attempts = append(c.attempts, request.Clone())
	if !request.Equal(c.accepted) {
		return errors.New("replayed steer does not match the accepted command")
	}
	return nil
}

func (u *uncertainSteeringRuntime) SteerRun(ctx context.Context, request agent.SteerRun) error {
	u.mu.Lock()
	u.requests = append(u.requests, request)
	attempt := len(u.requests)
	u.mu.Unlock()
	if attempt == 1 {
		if err := u.Runtime.SteerRun(ctx, request); err != nil {
			return err
		}
		return fmt.Errorf("lost steer acknowledgement: %w", context.DeadlineExceeded)
	}
	return nil
}

func (u *uncertainSteeringRuntime) steerAttempts() []agent.SteerRun {
	u.mu.Lock()
	defer u.mu.Unlock()
	return slices.Clone(u.requests)
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
	host.Shows(t, "workbench: settle rejected steer command")
	host.Shows(t, "@notes.txt")

	restoreWrites()
	host.Type("x")
	staged.Text = "x"
	awaitStoredDraft(t, stateDirectory, sessionID, staged)
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

func TestRestartSettlesAcceptedSteerWithoutReturningItsAttachments(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Event: agent.BlockStarted{Block: agent.Block{ID: "thinking", Kind: agent.BlockReasoning}}},
			{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	runtime := &committedThenCanceledSteeringRuntime{
		Runtime: base, committed: make(chan agent.SteerRun, 1),
	}
	workspace := t.TempDir()
	stateDirectory := t.TempDir()
	attachmentPath := filepath.Join(workspace, "notes.txt")
	if err := os.WriteFile(attachmentPath, []byte("notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	profile := steerReplayTestProfile(workspace)
	host, stop := runUIFromConfig(t, Config{
		Runtime: runtime, RuntimeProfile: &profile, Workspace: workspace,
		StateDirectory: stateDirectory,
	})
	host.Shows(t, "Ask lyra")
	sessionID := firstRuntimeSession(t, base)
	host.Type("start long work")
	host.Press(input.Enter)
	host.Shows(t, "thinking")
	host.Type("/attach notes.txt")
	host.Press(input.Enter)
	host.Shows(t, "attached notes.txt")
	host.Type("/steer focus on parsing")
	host.Press(input.Enter)
	accepted := awaitSignalValue(t, runtime.committed, "accepted steer before acknowledgement")
	store, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pending, found := store.PendingSteer(sessionID)
	if !found || !pending.Command.Equal(accepted) {
		t.Fatalf("durable steer = %+v, found %t", pending, found)
	}
	stop()

	replay := &cachedSteeringRuntime{Runtime: base, accepted: accepted}
	restarted, stopRestarted := runUIFromConfig(t, Config{
		Runtime: replay, RuntimeProfile: &profile, Workspace: workspace,
		SessionID: sessionID, StateDirectory: stateDirectory,
	})
	restarted.Shows(t, "focus on parsing")
	if len(replay.attempts) != 1 || !replay.attempts[0].Equal(accepted) {
		t.Fatalf("restart steer attempts = %+v", replay.attempts)
	}
	reopened, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if durable, found := reopened.PendingSteer(sessionID); found {
		t.Fatalf("settled steer remains durable: %+v", durable)
	}
	history := reopened.History()
	if len(history) < 2 || !history[len(history)-1].Equal(accepted.Message) {
		t.Fatalf("restart history = %+v", history)
	}
	restarted.Type("/attachments")
	restarted.Press(input.Enter)
	restarted.Shows(t, "the composer has no attachments")
	stopRestarted()
}

func steerReplayTestProfile(workspace string) runtimeprofile.Profile {
	return runtimeprofile.Profile{
		Protocol: runtimeprofile.Protocol{Version: "2.0"},
		Server: runtimeprofile.Server{
			Name: "steer-test", Version: "1.0.0", DefaultWorkspace: workspace, Home: workspace,
		},
		Features: map[runtimeprofile.FeatureName]runtimeprofile.Feature{},
		Limits: runtimeprofile.Limits{
			MaxConcurrentRuns: 1, IdempotencyRetentionSeconds: 600,
			IdempotencyNamespace: "steer-test-runtime",
			RunReplay: runtimeprofile.ReplayLimits{
				Scope: "runtimeInstanceRootSegment", MaxEvents: 128, MaxBytes: 1 << 20,
			},
			MCPAuthorizationRetentionSeconds: 600,
			RuntimeSubscription:              runtimeprofile.SubscriptionLimits{MaxTopics: 1, MaxWatches: 1},
		},
	}
}
