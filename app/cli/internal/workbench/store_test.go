package workbench

import (
	"bytes"
	"os"
	"path/filepath"
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
	if err := store.StagePendingRun(PendingRun{State: PendingRunDispatching, Command: pending}); err != nil {
		t.Fatal(err)
	}
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

func TestRejectedDispatchRequeuesWithANewCommandIdentity(t *testing.T) {
	store, err := Open(t.TempDir(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	original := agent.CommandID("cli_11111111111111111111111111111111")
	command := agent.StartRun{
		CommandID: original, SessionID: "ses_1", Message: agent.Message{Text: "wait behind active run"},
	}
	if err := store.StagePendingRun(PendingRun{State: PendingRunDispatching, Command: command}); err != nil {
		t.Fatal(err)
	}
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
	if err := store.StagePendingRun(PendingRun{State: PendingRunDispatching, Command: command}); err != nil {
		t.Fatal(err)
	}
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
	pending := PendingResume{
		Command: agent.ResumeRun{
			CommandID: agent.CommandID("cli_33333333333333333333333333333333"), RunID: approval.RunID,
			Answers: []agent.InterruptAnswer{{
				ItemID: approval.ItemID,
				Answer: agent.ApprovalAnswer{
					Decision: agent.ApprovalDeny, Reason: "keep the generated fixture",
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
	if !ok || answer.Decision != agent.ApprovalDeny || answer.Reason != "keep the generated fixture" {
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
