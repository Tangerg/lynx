package workbench

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func TestPendingSteerAtomicallyReturnsAttachmentsIntoANewerDraft(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	sessionID := "ses_steer"
	attachment := steerTestAttachment(t.TempDir())
	source := agent.Message{Text: "/steer inspect the parser", Attachments: []agent.Attachment{attachment}}
	if err := store.SaveDraft(sessionID, source); err != nil {
		t.Fatal(err)
	}
	pending := steerTestPending(sessionID, attachment)
	if err := store.StagePendingSteer(pending, source); err != nil {
		t.Fatal(err)
	}
	if draft, found, err := store.Draft(sessionID); err != nil || found {
		t.Fatalf("draft after staging = %+v, found %t, error %v", draft, found, err)
	}

	reopened, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	staged, found := reopened.PendingSteer(sessionID)
	if !found || !staged.Command.Equal(pending.Command) {
		t.Fatalf("reopened pending steer = %+v, found %t", staged, found)
	}
	newer := agent.Message{Text: "new input while steer settles"}
	if err := reopened.SaveDraft(sessionID, newer); err != nil {
		t.Fatal(err)
	}
	recovered, err := reopened.RejectPendingSteer(sessionID, pending.Command.CommandID, newer)
	if err != nil {
		t.Fatal(err)
	}
	want := agent.Message{Text: newer.Text, Attachments: []agent.Attachment{attachment}}
	if !recovered.Equal(want) {
		t.Fatalf("recovered draft = %+v, want %+v", recovered, want)
	}

	settled, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, found := settled.PendingSteer(sessionID); found {
		t.Fatal("rejected pending steer survived restart")
	}
	if draft, found, err := settled.Draft(sessionID); err != nil || !found || !draft.Equal(want) {
		t.Fatalf("settled draft = %+v, found %t, error %v", draft, found, err)
	}
}

func TestPendingSteerAcknowledgementIsRestartIdempotentAndPreservesDraft(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	sessionID := "ses_steer"
	attachment := steerTestAttachment(t.TempDir())
	source := agent.Message{Text: "/steer inspect the parser", Attachments: []agent.Attachment{attachment}}
	pending := steerTestPending(sessionID, attachment)
	if err := store.SaveDraft(sessionID, source); err != nil {
		t.Fatal(err)
	}
	if err := store.StagePendingSteer(pending, source); err != nil {
		t.Fatal(err)
	}
	newer := agent.Message{Text: "keep this newer thought"}
	if err := store.SaveDraft(sessionID, newer); err != nil {
		t.Fatal(err)
	}
	if err := store.AcknowledgePendingSteer(sessionID, pending.Command.CommandID); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, found := reopened.PendingSteer(sessionID); found {
		t.Fatal("acknowledged pending steer survived restart")
	}
	if draft, found, err := reopened.Draft(sessionID); err != nil || !found || !draft.Equal(newer) {
		t.Fatalf("newer draft = %+v, found %t, error %v", draft, found, err)
	}
	history := reopened.History()
	if len(history) != 1 || !history[0].Equal(pending.Command.Message) {
		t.Fatalf("steer history = %+v", history)
	}
}

func steerTestAttachment(directory string) agent.Attachment {
	return agent.Attachment{
		ID: "att_notes", Kind: agent.AttachmentText, Name: "notes.txt",
		Path: filepath.Join(directory, "notes.txt"), MimeType: "text/plain", Size: 5,
	}
}

func steerTestPending(
	sessionID string,
	attachment agent.Attachment,
) PendingSteer {
	stagedAt := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	return PendingSteer{
		SessionID: sessionID,
		Command: agent.SteerRun{
			CommandID: "cli_11111111111111111111111111111111",
			RunID:     "run_1", SegmentID: "seg_1",
			Message: agent.Message{Text: "inspect the parser", Attachments: []agent.Attachment{attachment}},
		},
		StagedAt: stagedAt, ReplayNamespace: "runtime-test", ReplayUntil: stagedAt.Add(time.Hour),
	}
}
