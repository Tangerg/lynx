package workbench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func TestStoreRecoversSessionDraftTransferAfterPartialCommit(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	transfer := DraftTransfer{
		SourceSessionID:      "source",
		DestinationSessionID: "destination",
		SourceBefore:         agent.Message{Text: "latest source edit"},
		SourceAfter:          agent.Message{Text: "source baseline"},
		DestinationBefore:    agent.Message{Text: "destination draft"},
		DestinationAfter:     agent.Message{Text: "latest source edit"},
	}
	if err := store.SaveDraft(transfer.SourceSessionID, transfer.SourceBefore); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDraft(transfer.DestinationSessionID, transfer.DestinationBefore); err != nil {
		t.Fatal(err)
	}

	sourcePath := store.path(store.sessionStateName(transfer.SourceSessionID))
	backupPath := sourcePath + ".backup"
	if err := os.Rename(sourcePath, backupPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(sourcePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "blocker"), []byte("block replacement"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := store.ApplyDraftTransfer(transfer); err == nil {
		t.Fatal("partially blocked draft transfer unexpectedly succeeded")
	}
	if err := store.SaveDraft(transfer.SourceSessionID, agent.Message{Text: "must not overwrite the journal"}); err == nil ||
		!strings.Contains(err.Error(), "draft transfer") {
		t.Fatalf("source mutation while transfer is pending = %v", err)
	}
	if destination, found, err := store.Draft(transfer.DestinationSessionID); err != nil || !found ||
		!destination.Equal(transfer.DestinationAfter) {
		t.Fatalf("partially committed destination = %+v, found %t, error %v", destination, found, err)
	}

	if err := os.Remove(filepath.Join(sourcePath, "blocker")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(sourcePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backupPath, sourcePath); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	assertDraft(t, reopened, transfer.SourceSessionID, transfer.SourceAfter)
	assertDraft(t, reopened, transfer.DestinationSessionID, transfer.DestinationAfter)
	if _, err := os.Stat(reopened.path(sessionDraftTransferName)); !os.IsNotExist(err) {
		t.Fatalf("draft transfer journal survived recovery: %v", err)
	}
}

func TestStoreRecoversRetiredSourceDraftWithoutDuplicatingOwnership(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	transfer := DraftTransfer{
		SourceSessionID:      "deleted",
		DestinationSessionID: "replacement",
		SourceBefore:         agent.Message{Text: "move me"},
		DestinationAfter:     agent.Message{Text: "move me"},
	}
	if err := store.SaveDraft(transfer.SourceSessionID, transfer.SourceBefore); err != nil {
		t.Fatal(err)
	}
	if err := store.save(sessionDraftTransferName, transfer); err != nil {
		t.Fatal(err)
	}
	// Simulate a crash after the source retirement but before the destination
	// replacement. Recovery must finish the move instead of losing the draft.
	if err := store.SaveDraft(transfer.SourceSessionID, agent.Message{}); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	assertDraft(t, reopened, transfer.SourceSessionID, agent.Message{})
	assertDraft(t, reopened, transfer.DestinationSessionID, transfer.DestinationAfter)
}

func TestStoreRefusesToReplayDraftTransferOverNewerAuthoringState(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	transfer := DraftTransfer{
		SourceSessionID:      "source",
		DestinationSessionID: "destination",
		SourceBefore:         agent.Message{Text: "before"},
		SourceAfter:          agent.Message{Text: "baseline"},
		DestinationAfter:     agent.Message{Text: "before"},
	}
	if err := store.SaveDraft(transfer.SourceSessionID, transfer.SourceBefore); err != nil {
		t.Fatal(err)
	}
	if err := store.save(sessionDraftTransferName, transfer); err != nil {
		t.Fatal(err)
	}
	newer := agent.Message{Text: "authored after the stale journal"}
	if err := store.SaveDraft(transfer.SourceSessionID, newer); err != nil {
		t.Fatal(err)
	}

	if _, err := Open(directory, Config{}); err == nil || !strings.Contains(err.Error(), "source draft changed") {
		t.Fatalf("open with conflicting draft transfer = %v", err)
	}
	if err := os.Remove(store.path(sessionDraftTransferName)); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	assertDraft(t, reopened, transfer.SourceSessionID, newer)
}

func assertDraft(t *testing.T, store *Store, sessionID string, want agent.Message) {
	t.Helper()
	got, found, err := store.Draft(sessionID)
	if err != nil || found != !messageEmpty(want) || !got.Equal(want) {
		t.Fatalf("draft %s = %+v, found %t, error %v; want %+v", sessionID, got, found, err, want)
	}
}
