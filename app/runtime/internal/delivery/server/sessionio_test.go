package server

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspacepath"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/core/model/chat"
)

// TestSessionExportImport_RoundTrip exports a populated session to a json
// artifact, wipes it, and imports it back — verifying metadata, chat history,
// items, and runs all survive the round trip under the original id (restore
// semantics).
func TestSessionExportImport_RoundTrip(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := t.Context()
	cwd := t.TempDir()
	canonicalCwd := workspacepath.Canonical(cwd)

	ses, err := rt.sess.Create(ctx, "My Session", cwd)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := rt.SeedHistory(ctx, ses.ID, []chat.Message{
		chat.NewUserMessage("hello"),
		chat.NewAssistantMessage("hi there"),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	putRun(t, rt, ses.ID, "run1", 1, 2)
	putUserItem(t, rt, ses.ID, "run1", "item1", "hello")

	// Export (json).
	exp, err := s.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: ses.ID})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if exp.Format != protocol.ExportFormatJSON || exp.Artifact == nil {
		t.Fatalf("export = %+v, want a json artifact", exp)
	}
	art := exp.Artifact
	if art.Session.Title != "My Session" || art.Session.Cwd != cwd {
		t.Errorf("artifact session = %+v, want title/cwd preserved", art.Session)
	}
	if len(art.Messages) != 2 || len(art.Items) != 1 || len(art.Runs) != 1 {
		t.Fatalf("artifact = %d msgs / %d items / %d runs, want 2/1/1", len(art.Messages), len(art.Items), len(art.Runs))
	}

	// Wipe the session entirely.
	if err := rt.sess.Delete(ctx, ses.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := rt.Transcript().DeleteSession(ctx, ses.ID); err != nil {
		t.Fatalf("delete history: %v", err)
	}
	_ = rt.TruncateMessages(ctx, ses.ID, 0)
	if _, err := rt.sess.Get(ctx, ses.ID); err == nil {
		t.Fatal("session should be gone before import")
	}

	// Import restores it under the same id.
	imp, err := s.ImportSession(ctx, protocol.ImportSessionRequest{Artifact: *art})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if imp.Session == nil || imp.Session.ID != ses.ID || imp.Session.Title != "My Session" || imp.Session.Cwd != canonicalCwd {
		t.Fatalf("imported session = %+v, want id/title/cwd restored", imp.Session)
	}

	// Chat history restored.
	msgs, _ := rt.ReadHistory(ctx, ses.ID)
	if len(msgs) != 2 {
		t.Errorf("restored messages = %d, want 2", len(msgs))
	}
	// Items + runs restored (items.list).
	items, err := s.ListItems(ctx, protocol.ListItemsRequest{SessionID: ses.ID})
	if err != nil {
		t.Fatalf("items.list: %v", err)
	}
	if len(items.Data) != 1 || len(items.Runs) != 1 {
		t.Errorf("restored items/runs = %d/%d, want 1/1", len(items.Data), len(items.Runs))
	}
}

func TestSessionImportRejectsActiveSession(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := context.Background()

	ses, err := rt.sess.Create(ctx, "Live", "/proj")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !s.claimSession(ses.ID) {
		t.Fatal("claim session")
	}
	t.Cleanup(func() { s.releaseSession(ses.ID) })

	_, err = s.ImportSession(ctx, protocol.ImportSessionRequest{
		Artifact: protocol.SessionArtifact{
			Version: protocol.SessionArtifactVersion,
			Session: protocol.Session{
				ID:    ses.ID,
				Title: "Restored",
				Cwd:   "/restore",
			},
		},
	})
	if !errors.Is(err, protocol.ErrSessionBusy) {
		t.Fatalf("import err = %v, want ErrSessionBusy", err)
	}
	got, err := rt.sess.Get(ctx, ses.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.Title != "Live" || got.Cwd != "/proj" {
		t.Fatalf("session mutated under active run: %+v", got)
	}
}

func TestSessionExportRejectsActiveSession(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := t.Context()
	ses, err := rt.sess.Create(ctx, "Live", t.TempDir())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !s.claimSession(ses.ID) {
		t.Fatal("claim session")
	}
	t.Cleanup(func() { s.releaseSession(ses.ID) })

	_, err = s.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: ses.ID})
	if !errors.Is(err, protocol.ErrSessionBusy) {
		t.Fatalf("export err = %v, want ErrSessionBusy", err)
	}
}

func TestSessionImportRejectsOpenInterrupt(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := context.Background()

	ses, err := rt.sess.Create(ctx, "Parked", "/proj")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := rt.interrupts.Put(ctx, interrupts.Pending{RunID: "run_parked", SessionID: ses.ID}); err != nil {
		t.Fatalf("seed interrupt: %v", err)
	}

	_, err = s.ImportSession(ctx, protocol.ImportSessionRequest{
		Artifact: protocol.SessionArtifact{
			Version: protocol.SessionArtifactVersion,
			Session: protocol.Session{
				ID:    ses.ID,
				Title: "Restored",
				Cwd:   "/restore",
			},
		},
	})
	if !errors.Is(err, protocol.ErrSessionBusy) {
		t.Fatalf("import err = %v, want ErrSessionBusy", err)
	}
}

func TestSessionExportRejectsOpenInterrupt(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := t.Context()
	ses, err := rt.sess.Create(ctx, "Parked", t.TempDir())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := rt.interrupts.Put(ctx, interrupts.Pending{RunID: "run_parked", SessionID: ses.ID}); err != nil {
		t.Fatalf("seed interrupt: %v", err)
	}

	_, err = s.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: ses.ID})
	if !errors.Is(err, protocol.ErrSessionBusy) {
		t.Fatalf("export err = %v, want ErrSessionBusy", err)
	}
}

func TestCancelParkedRunProducesPortableTerminalSnapshot(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := t.Context()
	ses, err := rt.sess.Create(ctx, "Parked", t.TempDir())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	rt.history[ses.ID] = []chat.Message{chat.NewUserMessage("hello"), chat.NewAssistantMessage("waiting")}
	if err := rt.hist.PutRun(ctx, transcript.Run{
		ID: "run_parked", SessionID: ses.ID, State: execution.Interrupted,
		Interrupts:  []transcript.Interrupt{{ItemID: "item_question", Kind: transcript.QuestionInterrupt}},
		MessageMark: -1,
	}); err != nil {
		t.Fatalf("put interrupted run: %v", err)
	}
	if err := rt.hist.AppendItem(ctx, transcript.Item{
		ID: "item_question", RunID: "run_parked", SessionID: ses.ID,
		Kind: transcript.QuestionItem, Status: transcript.ItemRunning,
	}); err != nil {
		t.Fatalf("put interrupt item: %v", err)
	}
	if err := rt.interrupts.Put(ctx, interrupts.Pending{
		RunID: "run_parked", SessionID: ses.ID, TurnID: "turn_parked",
	}); err != nil {
		t.Fatalf("put interrupt: %v", err)
	}

	if err := s.CancelRun(ctx, protocol.CancelRunRequest{RunID: "run_parked", Reason: "user stopped"}); err != nil {
		t.Fatalf("cancel parked run: %v", err)
	}
	exported, err := s.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: ses.ID})
	if err != nil {
		t.Fatalf("export canceled session: %v", err)
	}
	run := exported.Artifact.Runs[0]
	if run.Run.Outcome == nil || run.Run.Outcome.Type != protocol.OutcomeCanceled || run.Run.Outcome.Result == nil {
		t.Fatalf("exported run = %+v, want canceled terminal result", run)
	}
	if run.MessageMark != 2 || run.Run.Outcome.Detail != "user stopped" {
		t.Fatalf("exported mark/detail = %d/%q, want 2/user stopped", run.MessageMark, run.Run.Outcome.Detail)
	}
	if got := exported.Artifact.Items[0].Item.Status; got != protocol.ItemStatusIncomplete {
		t.Fatalf("interrupt item status = %q, want incomplete", got)
	}
}

func TestRestoreSessionClearsOpenInterrupts(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := t.Context()
	restoreCwd := t.TempDir()

	ses, err := rt.sess.Create(ctx, "Old", "/proj")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := rt.interrupts.Put(ctx, interrupts.Pending{RunID: "run_old", SessionID: ses.ID}); err != nil {
		t.Fatalf("seed interrupt: %v", err)
	}

	if err := s.sessions.RestoreSession(ctx, session.Session{
		ID:    ses.ID,
		Title: "Restored",
		Cwd:   restoreCwd,
	}, nil, nil, nil); err != nil {
		t.Fatalf("restore: %v", err)
	}
	pending, err := rt.interrupts.List(ctx, ses.ID)
	if err != nil {
		t.Fatalf("list interrupts: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending interrupts = %+v, want cleared", pending)
	}
}

// TestSessionExport_Markdown renders a human transcript.
func TestSessionExport_Markdown(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := context.Background()
	ses, _ := rt.sess.Create(ctx, "Doc", "/proj")
	putRun(t, rt, ses.ID, "run1", 1, 0)
	putUserItem(t, rt, ses.ID, "run1", "item1", "explain this")

	exp, err := s.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: ses.ID, Format: protocol.ExportFormatMarkdown})
	if err != nil {
		t.Fatalf("export md: %v", err)
	}
	if exp.Format != protocol.ExportFormatMarkdown || exp.Artifact != nil {
		t.Fatalf("export = %+v, want md (no artifact)", exp)
	}
	if !strings.Contains(exp.Markdown, "# Doc") || !strings.Contains(exp.Markdown, "explain this") {
		t.Errorf("markdown = %q, want title + user text", exp.Markdown)
	}
}

func TestSessionExportRejectsUnknownFormat(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := context.Background()
	ses, err := rt.sess.Create(ctx, "Doc", "/proj")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err = s.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: ses.ID, Format: "yaml"})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("export err = %v, want ErrInvalidParams", err)
	}
}

// TestSessionImport_VersionMismatch rejects an unrecognized artifact version.
func TestSessionImport_VersionMismatch(t *testing.T) {
	s, _ := rollbackHarness(t)
	for _, version := range []int{2, 999} {
		_, err := s.ImportSession(context.Background(), protocol.ImportSessionRequest{
			Artifact: protocol.SessionArtifact{Version: version, Session: protocol.Session{ID: "ses_x"}},
		})
		if !errors.Is(err, protocol.ErrInvalidParams) {
			t.Fatalf("version %d mismatch err = %v, want ErrInvalidParams", version, err)
		}
	}
}

func TestSessionImportRejectsUnavailableCwd(t *testing.T) {
	s, _ := rollbackHarness(t)
	missing := t.TempDir() + "/missing"
	_, err := s.ImportSession(t.Context(), protocol.ImportSessionRequest{
		Artifact: protocol.SessionArtifact{
			Version: protocol.SessionArtifactVersion,
			Session: protocol.Session{ID: "ses_missing_cwd", Cwd: missing},
		},
	})
	if !errors.Is(err, protocol.ErrCwdUnavailable) {
		t.Fatalf("import error = %v, want ErrCwdUnavailable", err)
	}
}
