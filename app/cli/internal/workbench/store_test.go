package workbench

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func TestStorePersistsBoundedHistoryDraftsStashesAndWorkspaces(t *testing.T) {
	directory := t.TempDir()
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	store, err := Open(directory, Config{
		HistoryLimit: 2, StashLimit: 2, WorkspaceLimit: 2,
		Now: func() time.Time { return now }, Random: bytes.NewReader([]byte("12345678abcdefgh")),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"one", "two", "three"} {
		if err := store.Remember(agent.Message{Text: text}); err != nil {
			t.Fatal(err)
		}
	}
	draft := agent.Message{Text: "unfinished", Attachments: []agent.Attachment{{ID: "attachment", Path: "/tmp/a.go", Name: "a.go", Kind: agent.AttachmentText}}}
	if err := store.SaveDraft("../../session", draft); err != nil {
		t.Fatal(err)
	}
	if _, err := store.StashPrompt(agent.Message{Text: "saved prompt"}); err != nil {
		t.Fatal(err)
	}
	for _, workspace := range []string{"one", "two", "three"} {
		if err := store.RememberWorkspace(filepath.Join(directory, workspace)); err != nil {
			t.Fatal(err)
		}
	}

	reopened, err := Open(directory, Config{HistoryLimit: 2, StashLimit: 2, WorkspaceLimit: 2})
	if err != nil {
		t.Fatal(err)
	}
	history := reopened.History()
	if len(history) != 2 || history[0].Text != "two" || history[1].Text != "three" {
		t.Fatalf("history = %+v", history)
	}
	restored, ok, err := reopened.Draft("../../session")
	if err != nil || !ok || restored.Text != draft.Text || restored.Attachments[0].ID != "attachment" {
		t.Fatalf("draft = %+v, %v, %v", restored, ok, err)
	}
	if stashes := reopened.Stashes(); len(stashes) != 1 || stashes[0].Message.Text != "saved prompt" {
		t.Fatalf("stashes = %+v", stashes)
	}
	workspaces := reopened.Workspaces()
	if len(workspaces) != 2 || workspaces[0].Path != filepath.Join(directory, "three") || workspaces[1].Path != filepath.Join(directory, "two") {
		t.Fatalf("workspaces = %+v", workspaces)
	}
	if _, err := os.Stat(filepath.Join(directory, "session")); !os.IsNotExist(err) {
		t.Fatalf("untrusted session id escaped state root: %v", err)
	}
}

func TestStoreDoesNotMutateMemoryWhenPersistenceFails(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(directory, 0o700) })
	if err := store.Remember(agent.Message{Text: "should fail"}); err == nil {
		t.Skip("filesystem permits writes despite directory mode")
	}
	if history := store.History(); len(history) != 0 {
		t.Fatalf("failed persistence mutated history: %+v", history)
	}
}

func TestStorePreservesCachedDraftWhenDurableDeletionFails(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	const sessionID = "session"
	want := agent.Message{Text: "keep this draft"}
	if err := store.SaveDraft(sessionID, want); err != nil {
		t.Fatal(err)
	}
	draftPath := store.path(store.sessionStateName(sessionID))
	if err := os.Remove(draftPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(draftPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(draftPath, "blocker"), []byte("block deletion"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := store.DiscardDraft(sessionID); err == nil {
		t.Fatal("durable draft deletion unexpectedly succeeded")
	}
	got, ok, err := store.Draft(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got.Text != want.Text {
		t.Fatalf("cached draft after failed deletion = %+v, %v; want %+v, true", got, ok, want)
	}
}

func TestStoreRollsBackAStashWhenDraftRetirementFails(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory, Config{
		StashLimit: 1, Random: bytes.NewReader([]byte("12345678abcdefgh")),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.StashPrompt(agent.Message{Text: "older stash"}); err != nil {
		t.Fatal(err)
	}
	const sessionID = "session"
	draft := agent.Message{Text: "draft must keep one owner"}
	if err := store.SaveDraft(sessionID, draft); err != nil {
		t.Fatal(err)
	}
	draftPath := store.path(store.sessionStateName(sessionID))
	if err := os.Remove(draftPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(draftPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(draftPath, "blocker"), []byte("block deletion"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := store.StashDraft(sessionID, draft); err == nil {
		t.Fatal("stash transaction unexpectedly retired the blocked draft")
	}
	if stashes := store.Stashes(); len(stashes) != 1 || stashes[0].Message.Text != "older stash" {
		t.Fatalf("stashes after rollback = %+v, want the pre-transaction collection", stashes)
	}
	if got, found, err := store.Draft(sessionID); err != nil || !found || !got.Equal(draft) {
		t.Fatalf("draft after failed stash = %+v, %t, %v", got, found, err)
	}
}

func TestStoreStashesDraftWithoutRetiringSessionOutboxes(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	const sessionID = "session"
	pending := PendingRun{
		State: PendingRunQueued,
		Command: agent.StartRun{
			CommandID: agent.CommandID("cli_11111111111111111111111111111111"),
			SessionID: sessionID, Message: agent.Message{Text: "pending run"},
		},
	}
	if err := store.StagePendingRun(pending); err != nil {
		t.Fatal(err)
	}
	approval := agent.Approval{
		RunID: "run_waiting", ItemID: "approval", Title: "Approve",
		Tool: &agent.ToolCall{Kind: agent.ToolRead, Name: "read", Path: "README.md", Status: agent.ToolRunning},
	}
	resume := PendingResume{
		Command: agent.ResumeRun{
			CommandID: agent.CommandID("cli_22222222222222222222222222222222"), RunID: approval.RunID,
			Answers: []agent.InterruptAnswer{{ItemID: approval.ItemID, Answer: agent.ApprovalAnswer{Decision: agent.ApprovalDeny}}},
		},
		Interactions: []agent.Interaction{approval},
	}
	if err := store.StagePendingResume(sessionID, resume); err != nil {
		t.Fatal(err)
	}
	draft := agent.Message{Text: "stash only this draft"}
	if err := store.SaveDraft(sessionID, draft); err != nil {
		t.Fatal(err)
	}
	if _, err := store.StashDraft(sessionID, draft); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := reopened.Draft(sessionID); err != nil || found {
		t.Fatalf("draft after stash = found %t, error %v", found, err)
	}
	if stashes := reopened.Stashes(); len(stashes) != 1 || !stashes[0].Message.Equal(draft) {
		t.Fatalf("stashes = %+v", stashes)
	}
	if runs := reopened.PendingRuns(sessionID); len(runs) != 1 || runs[0].Command.CommandID != pending.Command.CommandID {
		t.Fatalf("pending runs after stash = %+v", runs)
	}
	if got, found := reopened.PendingResume(sessionID); !found || got.Command.CommandID != resume.Command.CommandID {
		t.Fatalf("pending resume after stash = %+v, %t", got, found)
	}
}

func TestStoreCompletesInterruptedStashTransfersOnOpen(t *testing.T) {
	for _, phase := range []string{"intent saved", "stash saved", "source retired"} {
		t.Run(phase, func(t *testing.T) {
			directory := t.TempDir()
			store, err := Open(directory, Config{StashLimit: 2})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.StashPrompt(agent.Message{Text: "older stash"}); err != nil {
				t.Fatal(err)
			}
			const sessionID = "session"
			draft := agent.Message{Text: "restart-safe draft transfer"}
			if err := store.SaveDraft(sessionID, draft); err != nil {
				t.Fatal(err)
			}
			transfer := stashTransfer{
				SessionID: sessionID, Draft: draft,
				Stash: Stash{
					ID: "0123456789abcdef", CreatedAt: time.Date(2026, 8, 13, 20, 0, 0, 0, time.UTC),
					Message: draft,
				},
			}
			if err := store.save(stashTransferName, transfer); err != nil {
				t.Fatal(err)
			}
			if phase == "stash saved" || phase == "source retired" {
				next := tailStashes(append(slices.Clone(store.stashes), transfer.Stash), store.stashLimit)
				if err := store.save("stashes.json", next); err != nil {
					t.Fatal(err)
				}
			}
			if phase == "source retired" {
				if err := store.saveSessionState(sessionID, agent.Message{}, nil); err != nil {
					t.Fatal(err)
				}
			}

			reopened, err := Open(directory, Config{StashLimit: 2})
			if err != nil {
				t.Fatal(err)
			}
			if _, found, err := reopened.Draft(sessionID); err != nil || found {
				t.Fatalf("draft after recovery = found %t, error %v", found, err)
			}
			stashes := reopened.Stashes()
			if len(stashes) != 2 || stashes[0].ID != transfer.Stash.ID || !stashes[0].Message.Equal(draft) {
				t.Fatalf("stashes after recovery = %+v", stashes)
			}
			settled, err := Open(directory, Config{StashLimit: 2})
			if err != nil {
				t.Fatal(err)
			}
			if stashes := settled.Stashes(); len(stashes) != 2 || stashes[0].ID != transfer.Stash.ID {
				t.Fatalf("stashes after idempotent reopen = %+v", stashes)
			}
		})
	}
}

func TestStoreDoesNotReplayAStashTransferOverANewerDraft(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	const sessionID = "session"
	old := agent.Message{Text: "old transfer source"}
	if err := store.SaveDraft(sessionID, old); err != nil {
		t.Fatal(err)
	}
	transfer := stashTransfer{
		SessionID: sessionID, Draft: old,
		Stash: Stash{ID: "0123456789abcdef", CreatedAt: time.Now().UTC(), Message: old},
	}
	if err := store.save(stashTransferName, transfer); err != nil {
		t.Fatal(err)
	}
	newer := agent.Message{Text: "new owner"}
	if err := store.SaveDraft(sessionID, newer); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if draft, found, err := reopened.Draft(sessionID); err != nil || !found || !draft.Equal(newer) {
		t.Fatalf("newer draft after recovery = %+v, %t, %v", draft, found, err)
	}
	if stashes := reopened.Stashes(); len(stashes) != 0 {
		t.Fatalf("superseded transfer created stashes = %+v", stashes)
	}
}

func TestStoreDoesNotRewriteAnAlreadyEmptyDraft(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	const sessionID = "session"
	draftPath := store.path(store.sessionStateName(sessionID))
	if err := os.MkdirAll(draftPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(draftPath, "blocker"), []byte("block writes"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := store.SaveDraft(sessionID, agent.Message{}); err != nil {
		t.Fatalf("saving the already-empty draft rewrote session state: %v", err)
	}
}

func TestStoreRetiresCompleteSessionStateBehindADurableTombstone(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	const sessionID = "session"
	command := PendingRun{
		State: PendingRunQueued,
		Command: agent.StartRun{
			CommandID: agent.CommandID("cli_11111111111111111111111111111111"),
			SessionID: sessionID, Message: agent.Message{Text: "pending run"},
		},
	}
	if err := store.StagePendingRun(command); err != nil {
		t.Fatal(err)
	}
	approval := agent.Approval{
		RunID: "run_waiting", ItemID: "approval", Title: "Approve",
		Tool: &agent.ToolCall{Kind: agent.ToolRead, Name: "read", Path: "README.md", Status: agent.ToolRunning},
	}
	resume := PendingResume{
		Command: agent.ResumeRun{
			CommandID: agent.CommandID("cli_22222222222222222222222222222222"), RunID: approval.RunID,
			Answers: []agent.InterruptAnswer{{ItemID: approval.ItemID, Answer: agent.ApprovalAnswer{Decision: agent.ApprovalDeny}}},
		},
		Interactions: []agent.Interaction{approval},
	}
	if err := store.StagePendingResume(sessionID, resume); err != nil {
		t.Fatal(err)
	}
	draft := agent.Message{Text: "unsent draft"}
	if err := store.SaveDraft(sessionID, draft); err != nil {
		t.Fatal(err)
	}

	statePath := store.path(store.sessionStateName(sessionID))
	backupPath := statePath + ".backup"
	if err := os.Rename(statePath, backupPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(statePath, 0o700); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(statePath, "blocker")
	if err := os.WriteFile(blocker, []byte("block deletion"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := store.RetireSessionState(sessionID); err != nil {
		t.Fatal(err)
	}
	if got, found, err := store.Draft(sessionID); err != nil || found {
		t.Fatalf("retired draft = %+v, %v, %v", got, found, err)
	}
	if got := store.PendingRuns(sessionID); len(got) != 0 {
		t.Fatalf("retired runs remain = %+v", got)
	}
	if got, found := store.PendingResume(sessionID); found {
		t.Fatalf("retired resume remains = %+v", got)
	}
	if deletions := store.PendingSessionDeletions(); len(deletions) != 1 ||
		deletions[0].SessionID != sessionID || deletions[0].Phase != SessionDeletionConfirmed {
		t.Fatalf("session deletion tombstones = %+v", deletions)
	}
	reopened, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if got, found, err := reopened.Draft(sessionID); err != nil || found || len(reopened.PendingRuns(sessionID)) != 0 {
		t.Fatalf("reopened retired state has draft=%+v found=%v runs=%+v error=%v", got, found, reopened.PendingRuns(sessionID), err)
	}
	if pending, found := reopened.PendingResume(sessionID); found {
		t.Fatalf("reopened retired resume = %+v", pending)
	}
	if deletions := reopened.PendingSessionDeletions(); len(deletions) != 1 || deletions[0].Phase != SessionDeletionConfirmed {
		t.Fatalf("reopened deletion tombstones = %+v", deletions)
	}

	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(statePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backupPath, statePath); err != nil {
		t.Fatal(err)
	}
	reopened, err = Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := reopened.Draft(sessionID); err != nil || found || len(reopened.PendingRuns(sessionID)) != 0 {
		t.Fatalf("reopened retired state has draft=%v runs=%+v error=%v", found, reopened.PendingRuns(sessionID), err)
	}
	if pending, found := reopened.PendingResume(sessionID); found {
		t.Fatalf("reopened retired resume = %+v", pending)
	}
	if deletions := reopened.PendingSessionDeletions(); len(deletions) != 0 {
		t.Fatalf("cleaned deletion tombstones = %+v", deletions)
	}
}

func TestStoreRecoversPreparedSessionDeletionWithStableIdentity(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	const sessionID = "session"
	request := agent.DeleteSession{
		CommandID: agent.CommandID("cli_33333333333333333333333333333333"), SessionID: sessionID,
	}
	if err := store.SaveDraft(sessionID, agent.Message{Text: "owned until runtime confirmation"}); err != nil {
		t.Fatal(err)
	}
	if err := store.StageSessionDeletion(request); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	deletions := reopened.PendingSessionDeletions()
	if len(deletions) != 1 || deletions[0].Phase != SessionDeletionPrepared || deletions[0].Request() != request {
		t.Fatalf("prepared deletion = %+v", deletions)
	}
	if draft, found, err := reopened.Draft(sessionID); err != nil || !found || draft.Text != "owned until runtime confirmation" {
		t.Fatalf("prepared deletion draft = %+v, %t, %v", draft, found, err)
	}
	if err := reopened.ConfirmSessionDeletion(sessionID, request.CommandID); err != nil {
		t.Fatal(err)
	}
	if draft, found, err := reopened.Draft(sessionID); err != nil || found {
		t.Fatalf("confirmed deletion draft = %+v, %t, %v", draft, found, err)
	}
	if deletions := reopened.PendingSessionDeletions(); len(deletions) != 0 {
		t.Fatalf("settled deletions = %+v", deletions)
	}
}

func TestStoreRejectsOnlyTheExactPreparedSessionDeletion(t *testing.T) {
	store, err := Open(t.TempDir(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	request := agent.DeleteSession{
		CommandID: agent.CommandID("cli_44444444444444444444444444444444"), SessionID: "session",
	}
	if err := store.StageSessionDeletion(request); err != nil {
		t.Fatal(err)
	}
	if err := store.RejectSessionDeletion(request.SessionID, agent.CommandID("cli_55555555555555555555555555555555")); err == nil {
		t.Fatal("stale rejection removed another deletion intent")
	}
	if err := store.RejectSessionDeletion(request.SessionID, request.CommandID); err != nil {
		t.Fatal(err)
	}
	if deletions := store.PendingSessionDeletions(); len(deletions) != 0 {
		t.Fatalf("rejected deletions = %+v", deletions)
	}
}

func TestStoreDoesNotDeduplicateChangedAttachmentMetadata(t *testing.T) {
	store, err := Open("", Config{})
	if err != nil {
		t.Fatal(err)
	}
	first := agent.Message{Text: "inspect", Attachments: []agent.Attachment{{ID: "file", Path: "/tmp/file", Name: "old.go", Kind: agent.AttachmentText}}}
	second := first.Clone()
	second.Attachments[0].Name = "new.go"
	if err := store.Remember(first); err != nil {
		t.Fatal(err)
	}
	if err := store.Remember(second); err != nil {
		t.Fatal(err)
	}
	history := store.History()
	if len(history) != 2 || history[1].Attachments[0].Name != "new.go" {
		t.Fatalf("history = %+v, want both attachment metadata revisions", history)
	}
}

func TestStoreRejectsUnknownOnDiskFormat(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "history.json"), []byte(`{"version":99,"value":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(directory, Config{}); err == nil {
		t.Fatal("unknown format was accepted")
	}
}

func TestStorePersistsAndAcknowledgesPendingRunsByCommandIdentity(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	commandID := agent.CommandID("cli_0123456789abcdef0123456789abcdef")
	pending := agent.StartRun{
		CommandID: commandID, SessionID: "ses_1", Message: agent.Message{Text: "recover this start"},
		Options: agent.RunOptions{Provider: "deepseek", Model: "deepseek-v4-flash", Generation: agent.GenerationParams{Stop: []string{"done"}}},
	}
	if err := store.SaveDraft("ses_1", pending.Message); err != nil {
		t.Fatal(err)
	}
	stageDispatchingPendingRun(t, store, pending)
	pending.Message.Text = "mutated"
	pending.Options.Generation.Stop[0] = "mutated"

	reopened, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	restored := reopened.PendingRuns("ses_1")
	if len(restored) != 1 || restored[0].State != PendingRunDispatching || restored[0].Command.Message.Text != "recover this start" || restored[0].Command.Options.Generation.Stop[0] != "done" {
		t.Fatalf("restored pending runs = %+v", restored)
	}
	if err := reopened.AcknowledgePendingRun("ses_1", agent.CommandID("cli_ffffffffffffffffffffffffffffffff")); err == nil {
		t.Fatal("mismatched acknowledgement removed pending run")
	}
	if len(reopened.PendingRuns("ses_1")) != 1 {
		t.Fatal("mismatched acknowledgement removed pending run")
	}
	if err := reopened.AcknowledgePendingRun("ses_1", commandID); err != nil {
		t.Fatal(err)
	}
	if len(reopened.PendingRuns("ses_1")) != 0 {
		t.Fatal("acknowledged pending run remains")
	}
}

func TestPendingRunAcknowledgementIsIdempotentAfterSessionStatePersistenceFailure(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	command := agent.StartRun{
		CommandID: agent.CommandID("cli_99999999999999999999999999999999"),
		SessionID: "ses_1", Message: agent.Message{Text: "commit this prompt exactly once"},
	}
	stageDispatchingPendingRun(t, store, command)

	statePath := store.path(store.sessionStateName(command.SessionID))
	backupPath := statePath + ".backup"
	if err := os.Rename(statePath, backupPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(statePath, 0o700); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(statePath, "blocker")
	if err := os.WriteFile(blocker, []byte("block outbox retirement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.AcknowledgePendingRun(command.SessionID, command.CommandID); err == nil {
		t.Fatal("pending run acknowledgement survived blocked outbox retirement")
	}
	if history := store.History(); len(history) != 1 || !history[0].Equal(command.Message) {
		t.Fatalf("first settlement half did not publish history: %+v", history)
	}

	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(statePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backupPath, statePath); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.AcknowledgePendingRun(command.SessionID, command.CommandID); err != nil {
		t.Fatal(err)
	}
	if history := reopened.History(); len(history) != 1 || !history[0].Equal(command.Message) {
		t.Fatalf("retried settlement duplicated history: %+v", history)
	}
	if pending := reopened.PendingRuns(command.SessionID); len(pending) != 0 {
		t.Fatalf("retried settlement retained outbox: %+v", pending)
	}

	settled, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if history := settled.History(); len(history) != 1 || !history[0].Equal(command.Message) {
		t.Fatalf("durable history after restart = %+v", history)
	}
}

func TestBoundedHistoryRetainsAnUnsettledCommandIdentity(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory, Config{HistoryLimit: 1})
	if err != nil {
		t.Fatal(err)
	}
	command := agent.StartRun{
		CommandID: agent.CommandID("cli_abababababababababababababababab"),
		SessionID: "ses_1", Message: agent.Message{Text: "unsettled accepted prompt"},
	}
	stageDispatchingPendingRun(t, store, command)

	statePath := store.path(store.sessionStateName(command.SessionID))
	backupPath := statePath + ".backup"
	if err := os.Rename(statePath, backupPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(statePath, 0o700); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(statePath, "blocker")
	if err := os.WriteFile(blocker, []byte("block outbox retirement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.AcknowledgePendingRun(command.SessionID, command.CommandID); err == nil {
		t.Fatal("acknowledgement unexpectedly retired the blocked outbox")
	}
	if err := store.Remember(agent.Message{Text: "newer plain history"}); err != nil {
		t.Fatal(err)
	}
	if history := store.History(); len(history) != 2 || !history[0].Equal(command.Message) || history[1].Text != "newer plain history" {
		t.Fatalf("history limit evicted unsettled command identity: %+v", history)
	}

	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(statePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backupPath, statePath); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(directory, Config{HistoryLimit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.AcknowledgePendingRun(command.SessionID, command.CommandID); err != nil {
		t.Fatal(err)
	}
	if history := reopened.History(); len(history) != 1 || history[0].Text != "newer plain history" {
		t.Fatalf("bounded settlement did not restore ordinary history policy: %+v", history)
	}
}

func TestStoreRejectsDuplicateHistoryCommandIdentity(t *testing.T) {
	directory := t.TempDir()
	commandID := agent.CommandID("cli_cdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd")
	encoded := fmt.Sprintf(`{"version":1,"value":[{"Text":"one","commandId":%q},{"Text":"two","commandId":%q}]}`, commandID, commandID)
	if err := os.WriteFile(filepath.Join(directory, "history.json"), []byte(encoded), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(directory, Config{}); err == nil {
		t.Fatal("duplicate history command identity was accepted")
	}
}

func TestStagingTheSameCommandRejectsADifferentPayload(t *testing.T) {
	store, err := Open(t.TempDir(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	pending := PendingRun{State: PendingRunQueued, Command: agent.StartRun{
		CommandID: agent.CommandID("cli_33333333333333333333333333333333"),
		SessionID: "ses_1", Message: agent.Message{Text: "original"},
		Options: agent.RunOptions{Provider: "deepseek", Model: "v4"},
	}}
	if err := store.StagePendingRun(pending); err != nil {
		t.Fatal(err)
	}
	if err := store.StagePendingRun(pending); err != nil {
		t.Fatalf("identical idempotent staging returned %v", err)
	}
	conflict := pending
	conflict.Command.Message.Text = "different"
	if err := store.StagePendingRun(conflict); err == nil {
		t.Fatal("same command identity accepted a different payload")
	}
	conflict = pending
	conflict.Command.Options.Model = "v5"
	if err := store.StagePendingRun(conflict); err == nil {
		t.Fatal("same command identity accepted different run options")
	}
	if got := store.PendingRuns("ses_1"); len(got) != 1 || got[0].Command.Message.Text != "original" {
		t.Fatalf("conflicting staging mutated outbox: %+v", got)
	}
}

func TestRejectedDispatchRequeuesWithANewCommandIdentity(t *testing.T) {
	store, err := Open(t.TempDir(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	original := agent.CommandID("cli_11111111111111111111111111111111")
	command := agent.StartRun{
		CommandID: original, SessionID: "ses_1", Message: agent.Message{Text: "wait behind active run"},
	}
	stageDispatchingPendingRun(t, store, command)
	replacement, err := store.RequeuePendingRun(command.SessionID, original)
	if err != nil {
		t.Fatal(err)
	}
	if replacement == original || replacement.Validate() != nil {
		t.Fatalf("replacement command id = %q", replacement)
	}
	pending := store.PendingRuns(command.SessionID)
	if len(pending) != 1 || pending[0].State != PendingRunQueued ||
		pending[0].Command.CommandID != replacement || !pending[0].Command.Message.Equal(command.Message) {
		t.Fatalf("requeued command = %+v", pending)
	}
}

func TestPendingRunStateMachineRejectsUndeliveredSettlement(t *testing.T) {
	store, err := Open(t.TempDir(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	command := agent.StartRun{
		CommandID: agent.CommandID("cli_44444444444444444444444444444444"),
		SessionID: "ses_1", Message: agent.Message{Text: "must be delivered before settlement"},
	}
	if err := store.StagePendingRun(PendingRun{State: PendingRunQueued, Command: command}); err != nil {
		t.Fatal(err)
	}
	if err := store.AcknowledgePendingRun(command.SessionID, command.CommandID); err == nil {
		t.Fatal("queued run was acknowledged before delivery")
	}
	if _, err := store.RequeuePendingRun(command.SessionID, command.CommandID); err == nil {
		t.Fatal("queued run was reidentified before delivery")
	}
	if _, err := store.MarkPendingRunCanceling(command.SessionID, command.CommandID); err == nil {
		t.Fatal("queued run entered cancellation before delivery")
	}
	pending := store.PendingRuns(command.SessionID)
	if len(pending) != 1 || pending[0].State != PendingRunQueued || pending[0].Command.CommandID != command.CommandID {
		t.Fatalf("invalid transitions mutated outbox: %+v", pending)
	}
	if history := store.History(); len(history) != 0 {
		t.Fatalf("invalid acknowledgement committed history: %+v", history)
	}

	if err := store.MarkPendingRunDispatching(command.SessionID, command.CommandID); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkPendingRunDispatching(command.SessionID, command.CommandID); err != nil {
		t.Fatalf("idempotent dispatch returned %v", err)
	}
	if err := store.AcknowledgePendingRun(command.SessionID, command.CommandID); err != nil {
		t.Fatal(err)
	}
	if pending := store.PendingRuns(command.SessionID); len(pending) != 0 {
		t.Fatalf("acknowledged dispatch remains: %+v", pending)
	}
	if history := store.History(); len(history) != 1 || !history[0].Equal(command.Message) {
		t.Fatalf("acknowledged history = %+v", history)
	}
}

func TestCancelingPendingRunCannotReturnToQueuedDelivery(t *testing.T) {
	store, err := Open(t.TempDir(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	command := agent.StartRun{
		CommandID: agent.CommandID("cli_55555555555555555555555555555555"),
		SessionID: "ses_1", Message: agent.Message{Text: "canceling delivery"},
	}
	stageDispatchingPendingRun(t, store, command)
	cancelID, err := store.MarkPendingRunCanceling(command.SessionID, command.CommandID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RequeuePendingRun(command.SessionID, command.CommandID); err == nil {
		t.Fatal("canceling run returned to queued delivery")
	}
	pending := store.PendingRuns(command.SessionID)
	if len(pending) != 1 || pending[0].State != PendingRunCanceling || pending[0].CancelCommandID != cancelID {
		t.Fatalf("invalid requeue mutated canceling run: %+v", pending)
	}
	if err := store.AcknowledgePendingRun(command.SessionID, command.CommandID); err != nil {
		t.Fatal(err)
	}
}

func TestCancelingDispatchPersistsBothMutationIdentities(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	command := agent.StartRun{
		CommandID: agent.CommandID("cli_88888888888888888888888888888888"),
		SessionID: "ses_1", Message: agent.Message{Text: "cancel this uncertain start"},
	}
	stageDispatchingPendingRun(t, store, command)
	cancelID, err := store.MarkPendingRunCanceling(command.SessionID, command.CommandID)
	if err != nil {
		t.Fatal(err)
	}
	if replayed, err := store.MarkPendingRunCanceling(command.SessionID, command.CommandID); err != nil || replayed != cancelID {
		t.Fatalf("idempotent cancel transition = %q, %v; want %q", replayed, err, cancelID)
	}
	reopened, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	pending := reopened.PendingRuns(command.SessionID)
	if len(pending) != 1 || pending[0].State != PendingRunCanceling ||
		pending[0].Command.CommandID != command.CommandID || pending[0].CancelCommandID != cancelID {
		t.Fatalf("restored canceling dispatch = %+v", pending)
	}
}

func TestPendingRunSequenceKeepsTheOnlyDeliveryStateAtTheFIFOBoundary(t *testing.T) {
	store, err := Open(t.TempDir(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	commands := []PendingRun{
		{State: PendingRunQueued, Command: agent.StartRun{
			CommandID: agent.CommandID("cli_11111111111111111111111111111111"),
			SessionID: "ses_1", Message: agent.Message{Text: "first"},
		}},
		{State: PendingRunQueued, Command: agent.StartRun{
			CommandID: agent.CommandID("cli_22222222222222222222222222222222"),
			SessionID: "ses_1", Message: agent.Message{Text: "second"},
		}},
	}
	if err := store.SavePendingRuns("ses_1", commands); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkPendingRunDispatching("ses_1", commands[1].Command.CommandID); err == nil {
		t.Fatal("non-front command entered dispatching state")
	}
	if got := store.PendingRuns("ses_1"); len(got) != 2 || got[0].State != PendingRunQueued || got[1].State != PendingRunQueued {
		t.Fatalf("rejected transition mutated pending runs: %+v", got)
	}
	invalid := clonePendingRunSlice(commands)
	invalid[1].State = PendingRunDispatching
	if err := store.SavePendingRuns("ses_1", invalid); err == nil {
		t.Fatal("durable replacement accepted delivery state behind the FIFO boundary")
	}
}

func TestPendingRunCannotBeStagedWithoutAQueuedCommandIdentity(t *testing.T) {
	store, err := Open(t.TempDir(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	command := agent.StartRun{SessionID: "ses_1", Message: agent.Message{Text: "missing identity"}}
	if err := store.StagePendingRun(PendingRun{State: PendingRunQueued, Command: command}); err == nil {
		t.Fatal("pending run without a command identity was staged")
	}
	command.CommandID = agent.CommandID("cli_99999999999999999999999999999999")
	if err := store.StagePendingRun(PendingRun{State: PendingRunDispatching, Command: command}); err == nil {
		t.Fatal("pending run bypassed the queued initial state")
	}
	if pending := store.PendingRuns(command.SessionID); len(pending) != 0 {
		t.Fatalf("invalid staging mutated outbox: %+v", pending)
	}
}

func TestInvalidPendingRunTransitionsDoNotAllocateMutationIdentity(t *testing.T) {
	pending := PendingRun{State: PendingRunQueued, Command: agent.StartRun{
		CommandID: agent.CommandID("cli_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		SessionID: "ses_1", Message: agent.Message{Text: "still queued"},
	}}
	allocations := 0
	allocate := func() (agent.CommandID, error) {
		allocations++
		return agent.CommandID("cli_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), nil
	}
	if _, err := pending.beginCancellation(allocate); err == nil {
		t.Fatal("queued run began cancellation")
	}
	if _, err := pending.requeue(allocate); err == nil {
		t.Fatal("queued run was reidentified")
	}
	if allocations != 0 {
		t.Fatalf("invalid transitions allocated %d mutation identities", allocations)
	}
	if pending.State != PendingRunQueued || pending.Command.CommandID != agent.CommandID("cli_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa") {
		t.Fatalf("invalid transitions mutated pending run: %+v", pending)
	}
}

func stageDispatchingPendingRun(t *testing.T, store *Store, command agent.StartRun) {
	t.Helper()
	if err := store.StagePendingRun(PendingRun{State: PendingRunQueued, Command: command}); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkPendingRunDispatching(command.SessionID, command.CommandID); err != nil {
		t.Fatal(err)
	}
}

func TestStorePersistsPendingInteractionResumeUntilExactSettlement(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	approval := agent.Approval{
		RunID: "run_1", ItemID: "item_approval_1", Title: "Delete generated files",
		Tool:         &agent.ToolCall{Kind: agent.ToolEdit, Name: "delete", Status: agent.ToolRunning},
		Rememberable: true,
	}
	override, err := agent.ParseToolArgumentOverride([]byte(`{"path":"generated/fixture.go"}`))
	if err != nil {
		t.Fatal(err)
	}
	pending := PendingResume{
		Command: agent.ResumeRun{
			CommandID: agent.CommandID("cli_33333333333333333333333333333333"), RunID: approval.RunID,
			Answers: []agent.InterruptAnswer{{
				ItemID: approval.ItemID,
				Answer: agent.ApprovalAnswer{
					Decision: agent.ApprovalApprove, ArgumentOverride: override,
				},
			}},
		},
		Interactions: []agent.Interaction{approval},
	}
	if err := store.StagePendingResume("ses_1", pending); err != nil {
		t.Fatal(err)
	}
	pending.Command.Answers[0].Answer = agent.ApprovalAnswer{Decision: agent.ApprovalApprove}
	pending.Interactions[0] = agent.Approval{RunID: "mutated"}

	reopened, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	restored, ok := reopened.PendingResume("ses_1")
	if !ok || restored.Command.CommandID != agent.CommandID("cli_33333333333333333333333333333333") ||
		restored.Command.RunID != approval.RunID || len(restored.Interactions) != 1 {
		t.Fatalf("restored pending resume = %+v, present = %v", restored, ok)
	}
	answer, ok := restored.Command.Answers[0].Answer.(agent.ApprovalAnswer)
	if !ok || answer.Decision != agent.ApprovalApprove || answer.ArgumentOverride == nil ||
		string(answer.ArgumentOverride.JSON()) != `{"path":"generated/fixture.go"}` {
		t.Fatalf("restored pending answer = %#v", restored.Command.Answers[0].Answer)
	}
	if err := reopened.AcknowledgePendingResume("ses_1", agent.CommandID("cli_44444444444444444444444444444444")); err == nil {
		t.Fatal("mismatched acknowledgement retired pending resume")
	}
	if _, ok := reopened.PendingResume("ses_1"); !ok {
		t.Fatal("mismatched acknowledgement removed pending resume")
	}
	if err := reopened.AcknowledgePendingResume("ses_1", restored.Command.CommandID); err != nil {
		t.Fatal(err)
	}
	if _, ok := reopened.PendingResume("ses_1"); ok {
		t.Fatal("acknowledged pending resume remains")
	}
}

func TestStagingTheSameResumeCommandRejectsDifferentDecisions(t *testing.T) {
	store, err := Open(t.TempDir(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	approval := agent.Approval{
		RunID: "run_1", ItemID: "item_1", Title: "Run checks", Rememberable: true,
		Tool: &agent.ToolCall{Kind: agent.ToolShell, Name: "shell", Status: agent.ToolRunning},
	}
	pending := PendingResume{
		Command: agent.ResumeRun{
			CommandID: agent.CommandID("cli_66666666666666666666666666666666"), RunID: approval.RunID,
			Answers: []agent.InterruptAnswer{{ItemID: approval.ItemID, Answer: agent.ApprovalAnswer{Decision: agent.ApprovalApprove}}},
		},
		Interactions: []agent.Interaction{approval},
	}
	if err := store.StagePendingResume("ses_1", pending); err != nil {
		t.Fatal(err)
	}
	if err := store.StagePendingResume("ses_1", pending); err != nil {
		t.Fatalf("identical idempotent resume staging returned %v", err)
	}

	changedAnswer := clonePendingResume(pending)
	changedAnswer.Command.Answers[0].Answer = agent.ApprovalAnswer{
		Decision: agent.ApprovalDeny, Reason: "not this command",
	}
	if err := store.StagePendingResume("ses_1", changedAnswer); err == nil {
		t.Fatal("same resume identity accepted a different answer")
	}
	changedInteraction := clonePendingResume(pending)
	changedInteraction.Interactions[0] = agent.Approval{
		RunID: approval.RunID, ItemID: approval.ItemID, Title: "Different request", Rememberable: true,
		Tool: &agent.ToolCall{Kind: agent.ToolShell, Name: "shell", Status: agent.ToolRunning},
	}
	if err := store.StagePendingResume("ses_1", changedInteraction); err == nil {
		t.Fatal("same resume identity accepted a different interaction")
	}
	message := agent.Message{Text: "additional guidance"}
	changedMessage := clonePendingResume(pending)
	changedMessage.Command.Message = &message
	if err := store.StagePendingResume("ses_1", changedMessage); err == nil {
		t.Fatal("same resume identity accepted a different message")
	}

	stored, ok := store.PendingResume("ses_1")
	if !ok || !pendingResumeEqual(stored, pending) {
		t.Fatalf("conflicting resume staging mutated outbox: %+v, present %t", stored, ok)
	}
}

func TestStoreRejectsPendingResumeWithoutCommandIdentity(t *testing.T) {
	store, err := Open(t.TempDir(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	approval := agent.Approval{
		RunID: "run_1", ItemID: "item_1", Title: "Run checks",
		Tool: &agent.ToolCall{Kind: agent.ToolShell, Name: "shell", Status: agent.ToolRunning},
	}
	pending := PendingResume{
		Command: agent.ResumeRun{
			RunID: approval.RunID,
			Answers: []agent.InterruptAnswer{{
				ItemID: approval.ItemID,
				Answer: agent.ApprovalAnswer{Decision: agent.ApprovalApprove},
			}},
		},
		Interactions: []agent.Interaction{approval},
	}

	if err := store.StagePendingResume("ses_1", pending); err == nil {
		t.Fatal("pending resume without command identity was accepted")
	}
	if restored, found := store.PendingResume("ses_1"); found {
		t.Fatalf("invalid pending resume mutated the outbox: %+v", restored)
	}
}

func TestStorePersistsTheCompleteMixedInteractionReview(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	approval := agent.Approval{
		RunID: "run_1", ItemID: "item_approval", Title: "Run checks", Rememberable: true,
		Tool: &agent.ToolCall{Kind: agent.ToolShell, Name: "shell", Status: agent.ToolRunning},
	}
	question := agent.Question{
		RunID: "run_1", ItemID: "item_question", Title: "Choose targets",
		Fields: []agent.QuestionField{
			{Prompt: "Reason", Kind: agent.QuestionText},
			{Prompt: "Platforms", Kind: agent.QuestionMulti, AllowCustom: true, Options: []agent.QuestionOption{{Label: "linux"}, {Label: "darwin"}}},
		},
	}
	pending := PendingResume{
		Command: agent.ResumeRun{
			CommandID: agent.CommandID("cli_77777777777777777777777777777777"), RunID: "run_1",
			Answers: []agent.InterruptAnswer{
				{ItemID: approval.ItemID, Answer: agent.ApprovalAnswer{
					Decision: agent.ApprovalDeny, Remember: agent.RememberProject, Reason: "protect generated output",
				}},
				{ItemID: question.ItemID, Answer: agent.QuestionAnswer{Values: [][]string{{"portable"}, {"linux", "freebsd"}}}},
			},
		},
		Interactions: []agent.Interaction{approval, question},
	}
	if err := store.StagePendingResume("ses_1", pending); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	restored, ok := reopened.PendingResume("ses_1")
	if !ok || len(restored.Command.Answers) != 2 || len(restored.Interactions) != 2 {
		t.Fatalf("restored mixed resume = %+v, present = %t", restored, ok)
	}
	approvalAnswer, ok := restored.Command.Answers[0].Answer.(agent.ApprovalAnswer)
	if !ok || approvalAnswer.Decision != agent.ApprovalDeny || approvalAnswer.Remember != agent.RememberProject ||
		approvalAnswer.Reason != "protect generated output" {
		t.Fatalf("restored approval answer = %#v", restored.Command.Answers[0].Answer)
	}
	questionAnswer, ok := restored.Command.Answers[1].Answer.(agent.QuestionAnswer)
	if !ok || !reflect.DeepEqual(questionAnswer.Values, [][]string{{"portable"}, {"linux", "freebsd"}}) {
		t.Fatalf("restored question answer = %#v", restored.Command.Answers[1].Answer)
	}
	questionAnswer.Values[1][0] = "mutated"
	again, _ := reopened.PendingResume("ses_1")
	againQuestion := again.Command.Answers[1].Answer.(agent.QuestionAnswer)
	if againQuestion.Values[1][0] != "linux" {
		t.Fatal("pending resume exposed shared nested question storage")
	}
}
