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
	draftPath := store.path(store.draftName(sessionID))
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
