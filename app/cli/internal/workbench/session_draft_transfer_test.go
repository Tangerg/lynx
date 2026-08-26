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
	if saveDraftErr := store.SaveDraft(transfer.SourceSessionID, transfer.SourceBefore); saveDraftErr != nil {
		t.Fatal(saveDraftErr)
	}
	if saveDraftErr := store.SaveDraft(transfer.DestinationSessionID, transfer.DestinationBefore); saveDraftErr != nil {
		t.Fatal(saveDraftErr)
	}

	sourcePath := store.path(store.sessionStateName(transfer.SourceSessionID))
	backupPath := sourcePath + ".backup"
	if renameErr := os.Rename(sourcePath, backupPath); renameErr != nil {
		t.Fatal(renameErr)
	}
	if mkdirErr := os.Mkdir(sourcePath, 0o700); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}
	if writeFileErr := os.WriteFile(filepath.Join(sourcePath, "blocker"), []byte("block replacement"), 0o600); writeFileErr != nil {
		t.Fatal(writeFileErr)
	}

	if applyDraftTransferErr := store.ApplyDraftTransfer(transfer); applyDraftTransferErr == nil {
		t.Fatal("partially blocked draft transfer unexpectedly succeeded")
	}
	if saveDraftErr := store.SaveDraft(transfer.SourceSessionID, agent.Message{Text: "must not overwrite the journal"}); saveDraftErr == nil ||
		!strings.Contains(saveDraftErr.Error(), "draft transfer") {
		t.Fatalf("source mutation while transfer is pending = %v", saveDraftErr)
	}
	if destination, found, draftErr := store.Draft(transfer.DestinationSessionID); draftErr != nil || !found ||
		!destination.Equal(transfer.DestinationAfter) {
		t.Fatalf("partially committed destination = %+v, found %t, error %v", destination, found, draftErr)
	}

	if removeErr := os.Remove(filepath.Join(sourcePath, "blocker")); removeErr != nil {
		t.Fatal(removeErr)
	}
	if removeErr := os.Remove(sourcePath); removeErr != nil {
		t.Fatal(removeErr)
	}
	if renameErr := os.Rename(backupPath, sourcePath); renameErr != nil {
		t.Fatal(renameErr)
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
	if saveDraftErr := store.SaveDraft(transfer.SourceSessionID, transfer.SourceBefore); saveDraftErr != nil {
		t.Fatal(saveDraftErr)
	}
	if saveErr := store.save(sessionDraftTransferName, transfer); saveErr != nil {
		t.Fatal(saveErr)
	}
	// Simulate a crash after the source retirement but before the destination
	// replacement. Recovery must finish the move instead of losing the draft.
	if saveDraftErr := store.SaveDraft(transfer.SourceSessionID, agent.Message{}); saveDraftErr != nil {
		t.Fatal(saveDraftErr)
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
	if saveDraftErr := store.SaveDraft(transfer.SourceSessionID, transfer.SourceBefore); saveDraftErr != nil {
		t.Fatal(saveDraftErr)
	}
	if saveErr := store.save(sessionDraftTransferName, transfer); saveErr != nil {
		t.Fatal(saveErr)
	}
	newer := agent.Message{Text: "authored after the stale journal"}
	if saveDraftErr := store.SaveDraft(transfer.SourceSessionID, newer); saveDraftErr != nil {
		t.Fatal(saveDraftErr)
	}

	if _, openErr := Open(directory, Config{}); openErr == nil || !strings.Contains(openErr.Error(), "source draft changed") {
		t.Fatalf("open with conflicting draft transfer = %v", openErr)
	}
	if removeErr := os.Remove(store.path(sessionDraftTransferName)); removeErr != nil {
		t.Fatal(removeErr)
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
