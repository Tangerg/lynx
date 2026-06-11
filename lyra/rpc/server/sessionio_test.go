package server

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// TestSessionExportImport_RoundTrip exports a populated session to a json
// artifact, wipes it, and imports it back — verifying metadata, chat history,
// items, and runs all survive the round trip under the original id (restore
// semantics).
func TestSessionExportImport_RoundTrip(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := context.Background()

	ses, err := rt.sess.Create(ctx, "My Session", "/proj")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := rt.SeedHistory(ctx, ses.ID, []chat.Message{
		chat.NewUserMessage("hello"),
		chat.NewAssistantMessage("hi there"),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	putRun(t, rt, ses.ID, "run1", "", 1, 2)
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
	if art.Session.Title != "My Session" || art.Session.Cwd != "/proj" {
		t.Errorf("artifact session = %+v, want title/cwd preserved", art.Session)
	}
	if len(art.Messages) != 2 || len(art.Items) != 1 || len(art.Runs) != 1 {
		t.Fatalf("artifact = %d msgs / %d items / %d runs, want 2/1/1", len(art.Messages), len(art.Items), len(art.Runs))
	}

	// Wipe the session entirely.
	if err := rt.sess.Delete(ctx, ses.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := rt.History().DeleteSession(ctx, ses.ID); err != nil {
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
	if imp.Session == nil || imp.Session.ID != ses.ID || imp.Session.Title != "My Session" || imp.Session.Cwd != "/proj" {
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

// TestSessionExport_Markdown renders a human transcript.
func TestSessionExport_Markdown(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := context.Background()
	ses, _ := rt.sess.Create(ctx, "Doc", "/proj")
	putRun(t, rt, ses.ID, "run1", "", 1, 0)
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

// TestSessionImport_VersionMismatch rejects an unrecognized artifact version.
func TestSessionImport_VersionMismatch(t *testing.T) {
	s, _ := rollbackHarness(t)
	_, err := s.ImportSession(context.Background(), protocol.ImportSessionRequest{
		Artifact: protocol.SessionArtifact{Version: 999, Session: protocol.Session{ID: "ses_x"}},
	})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("version mismatch err = %v, want ErrInvalidParams", err)
	}
}
