package server

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	resultoffload "github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/core/chat"
)

// TestSessionExportImport_RoundTrip exports a populated session to a json
// artifact, wipes it, and imports it back — verifying identity, chat history,
// items, and runs all survive the round trip under the original id (restore
// semantics).
//
// It is the import boundary's evidence for imported_session_keeps_its_identity.
func TestSessionExportImport_RoundTrip(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := t.Context()
	cwd := t.TempDir()
	canonicalCWD := canonicalWorkspacePath(t, cwd)

	ses, err := rt.sess.Create(ctx, "My Session", cwd)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := rt.SeedHistory(ctx, ses.ID, []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("hello")),
		chat.NewAssistantMessage(chat.NewTextPart("hi there")),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	putRun(t, rt, ses.ID, "run1", 1, 2)
	putUserItem(t, rt, ses.ID, "run1", "item1", "hello")
	arguments, err := tool.ArgumentsFromMap(map[string]any{"command": "ls", "description": "List workspace files"})
	if err != nil {
		t.Fatalf("tool arguments: %v", err)
	}
	result, err := tool.NewResult(map[string]any{
		"stdout": "total 0\n", "stderr": "", "exit_code": float64(0),
	})
	if err != nil {
		t.Fatalf("tool result: %v", err)
	}
	if err := rt.hist.AppendItem(ctx, transcript.Item{
		SessionID: ses.ID, RunID: "run1", ID: "item2",
		OccurredAt: time.Unix(2, 0).UTC(),
		FinishedAt: time.Unix(3, 0).UTC(),
		Status:     transcript.ItemCompleted,
		Kind:       transcript.ToolCall,
		Tool: &transcript.ToolInvocation{
			Name:      "shell",
			Arguments: arguments,
			Result:    &result,
		},
	}); err != nil {
		t.Fatalf("seed tool item: %v", err)
	}

	// Export (json).
	exp, err := s.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: ses.ID})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if exp.Format != protocol.ExportFormatJSON || exp.Artifact == nil {
		t.Fatalf("export = %+v, want a json artifact", exp)
	}
	art := exp.Artifact
	if art.Session.Title != "My Session" || art.Session.Workspace.Path != ses.CWD {
		t.Errorf("artifact session = %+v, want title/workspace preserved", art.Session)
	}
	if len(art.Messages) != 2 || len(art.Items) != 2 || len(art.Runs) != 1 {
		t.Fatalf("artifact = %d msgs / %d items / %d runs, want 2/2/1", len(art.Messages), len(art.Items), len(art.Runs))
	}
	wantToolResult := `{"exit_code":0,"stderr":"","stdout":"total 0\n"}`
	assertArtifactToolResult(t, art.Items, "item2", wantToolResult)

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
	if imp.Session == nil || imp.Session.ID != ses.ID || imp.Session.Title != "My Session" || imp.Session.Workspace.Ref.Path != canonicalCWD {
		t.Fatalf("imported session = %+v, want id/title/workspace restored", imp.Session)
	}

	// Chat history restored.
	msgs, _ := rt.ReadHistory(ctx, ses.ID)
	if len(msgs) != 2 {
		t.Errorf("restored messages = %d, want 2", len(msgs))
	}
	// Items + runs restored (items.list).
	items, err := s.ListItems(ctx, protocol.ListItemsRequest{Scope: protocol.ItemListScope{Type: protocol.ItemScopeSession, SessionID: ses.ID}})
	if err != nil {
		t.Fatalf("items.list: %v", err)
	}
	if len(items.Data) != 2 || len(items.Runs) != 1 {
		t.Errorf("restored items/runs = %d/%d, want 2/1", len(items.Data), len(items.Runs))
	}

	// Exporting the restored canonical data preserves the original tool result;
	// archive encoding never runs the client-facing tool presenter.
	reexported, err := s.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: ses.ID})
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	assertArtifactToolResult(t, reexported.Artifact.Items, "item2", wantToolResult)
}

func TestSessionExportImportCarriesOffloadedToolResultsAcrossDatabases(t *testing.T) {
	source, sourceRuntime := rollbackHarness(t)
	destination, destinationRuntime := rollbackHarness(t)
	ctx := t.Context()
	cwd := t.TempDir()

	ses, err := sourceRuntime.sess.Create(ctx, "Portable offload", cwd)
	if err != nil {
		t.Fatalf("create source session: %v", err)
	}
	putRun(t, sourceRuntime, ses.ID, "run_offload", 1, 1)
	body := strings.Repeat("portable-result-", 100)
	id := resultoffload.NewID()
	if err := sourceRuntime.toolResults.Stage(ctx, resultoffload.Stage{
		ID: id, SessionID: ses.ID, ToolName: "vendor_tool", Body: body,
	}); err != nil {
		t.Fatalf("stage source result: %v", err)
	}
	preview := "offloaded preview " + id.String()
	ref := &resultoffload.Ref{ID: id}
	previewValue := tool.StringResult(preview)
	item := transcript.Item{
		SessionID: ses.ID, RunID: "run_offload", ID: "item_offload",
		OccurredAt: time.Unix(2, 0).UTC(), FinishedAt: time.Unix(3, 0).UTC(),
		Status: transcript.ItemCompleted, Kind: transcript.ToolCall,
		Tool: &transcript.ToolInvocation{Name: "vendor_tool", Result: &previewValue, Offload: ref},
	}
	if err := sourceRuntime.hist.AppendItem(ctx, item); err != nil {
		t.Fatalf("append source item: %v", err)
	}
	if err := sourceRuntime.toolResults.Bind(ctx, ses.ID, item.ID, preview, *ref); err != nil {
		t.Fatalf("bind source result: %v", err)
	}
	if err := sourceRuntime.SeedHistory(ctx, ses.ID, []chat.Message{
		chat.NewToolMessage(chat.ToolResult{ID: "call_offload", Name: "vendor_tool", Result: preview}),
	}); err != nil {
		t.Fatalf("seed source history: %v", err)
	}

	exported, err := source.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: ses.ID})
	if err != nil {
		t.Fatalf("export source: %v", err)
	}
	if got := len(exported.Artifact.ToolResults); got != 1 {
		t.Fatalf("artifact tool results = %d, want 1", got)
	}
	if exported.Artifact.ToolResults[0].Body != body || exported.Artifact.ToolResults[0].Preview != preview {
		t.Fatal("artifact did not preserve the offloaded body and preview")
	}
	if len(exported.Artifact.Items) != 1 || exported.Artifact.Items[0].Tool == nil || exported.Artifact.Items[0].Tool.Result != preview {
		t.Fatal("artifact item duplicated the full body instead of carrying its bounded preview")
	}

	if _, err := destination.ImportSession(ctx, protocol.ImportSessionRequest{Artifact: *exported.Artifact}); err != nil {
		t.Fatalf("import destination: %v", err)
	}
	restored, found, err := destinationRuntime.toolResults.Fetch(ctx, ses.ID, id)
	if err != nil || !found || restored != body {
		t.Fatalf("destination read-back = (found %v, bytes %d, err %v), want full body", found, len(restored), err)
	}
	items, err := destinationRuntime.hist.List(ctx, ses.ID)
	if err != nil {
		t.Fatalf("list destination transcript: %v", err)
	}
	if len(items) != 1 || items[0].Tool == nil || items[0].Tool.Result == nil {
		t.Fatalf("destination transcript = %+v, want rehydrated tool result", items)
	}
	if got, ok := items[0].Tool.Result.String(); !ok || got != body {
		t.Fatalf("destination transcript result = %q, want rehydrated tool result", got)
	}
	messages, err := destinationRuntime.ReadHistory(ctx, ses.ID)
	if err != nil {
		t.Fatalf("read destination history: %v", err)
	}
	encodedMessages, err := json.Marshal(messages)
	if err != nil {
		t.Fatalf("marshal destination history: %v", err)
	}
	if !strings.Contains(string(encodedMessages), id.String()) || strings.Contains(string(encodedMessages), body) {
		t.Fatal("destination model history must keep only the retrievable preview")
	}
}

func assertArtifactToolResult(t *testing.T, items []protocol.ArtifactItem, itemID, want string) {
	t.Helper()
	for _, artifactItem := range items {
		if artifactItem.ID != itemID {
			continue
		}
		if artifactItem.Tool == nil {
			t.Fatalf("artifact item %q has no tool", itemID)
		}
		got, err := json.Marshal(artifactItem.Tool.Result)
		if err != nil {
			t.Fatalf("marshal tool result: %v", err)
		}
		if string(got) != want {
			t.Fatalf("tool result = %s, want %s", got, want)
		}
		return
	}
	t.Fatalf("artifact item %q not found", itemID)
}

func TestSessionImportRejectsActiveSession(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := context.Background()

	ses, err := rt.sess.Create(ctx, "Live", "/proj")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	releaseSession, ok := rt.admissions.AcquireSession(ses.ID)
	if !ok {
		t.Fatal("claim session")
	}
	t.Cleanup(releaseSession)

	_, err = s.ImportSession(ctx, protocol.ImportSessionRequest{
		Artifact: protocol.SessionArtifact{
			Version: protocol.SessionArtifactVersion,
			Session: protocol.ArtifactSession{
				ID:        ses.ID,
				Title:     "Restored",
				Workspace: protocol.WorkspaceRef{Path: "/restore"},
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
	if got.Title != "Live" || got.CWD != "/proj" {
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
	releaseSession, ok := rt.admissions.AcquireSession(ses.ID)
	if !ok {
		t.Fatal("claim session")
	}
	t.Cleanup(releaseSession)

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
	if err := rt.interrupts.Open(ctx, serverPending(
		"run_parked",
		ses.ID,
		"",
		"",
		nil,
		time.Now().UTC(),
	)); err != nil {
		t.Fatalf("seed interrupt: %v", err)
	}

	_, err = s.ImportSession(ctx, protocol.ImportSessionRequest{
		Artifact: protocol.SessionArtifact{
			Version: protocol.SessionArtifactVersion,
			Session: protocol.ArtifactSession{
				ID:        ses.ID,
				Title:     "Restored",
				Workspace: protocol.WorkspaceRef{Path: "/restore"},
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
	if err := rt.interrupts.Open(ctx, serverPending(
		"run_parked",
		ses.ID,
		"",
		"",
		nil,
		time.Now().UTC(),
	)); err != nil {
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
	rt.history[ses.ID] = []chat.Message{chat.NewUserMessage(chat.NewTextPart("hello")), chat.NewAssistantMessage(chat.NewTextPart("waiting"))}
	parkedAt := time.Unix(1, 0).UTC()
	capabilities := run.Capabilities{
		InterruptKinds: []interrupt.Kind{interrupt.Question},
	}
	if err := rt.runs.Admit(ctx, run.Draft{SegmentID: "seg_open",
		RunID: "run_parked", SessionID: ses.ID, Capabilities: capabilities, CreatedAt: parkedAt,
	}); err != nil {
		t.Fatalf("admit parked run: %v", err)
	}
	if err := rt.runs.Suspend(ctx, transcript.Run{
		SessionID: ses.ID, ID: "run_parked", State: run.Waiting,
		Capabilities: capabilities,
		Interrupts: []transcript.Interrupt{{
			ItemID: "item_question", ItemOccurredAt: parkedAt,
			RunID: "run_parked", Kind: interrupt.Question,
			Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}},
		}},
		CreatedAt: parkedAt, MessageMark: transcript.UnknownMessageMark,
	}); err != nil {
		t.Fatalf("suspend parked run: %v", err)
	}
	if err := rt.hist.AppendItem(ctx, transcript.Item{
		ID: "item_question", RunID: "run_parked", SessionID: ses.ID,
		Kind: transcript.QuestionItem, Status: transcript.ItemRunning,
		OccurredAt: parkedAt,
		Question:   &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}},
	}); err != nil {
		t.Fatalf("open interrupt item: %v", err)
	}
	if err := rt.interrupts.Open(ctx, serverPending(
		"run_parked",
		ses.ID,
		"exec_parked",
		"member_parked",
		[]transcript.Interrupt{{
			ItemID:   "item_question",
			Kind:     interrupt.Question,
			Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}},
		}},
		parkedAt,
	)); err != nil {
		t.Fatalf("open interrupt: %v", err)
	}

	cancelResult, err := s.CancelRun(ctx, protocol.CancelRunRequest{RunID: "run_parked", Reason: "user stopped"})
	if err != nil {
		t.Fatalf("cancel parked run: %v", err)
	}
	if cancelResult.Type != protocol.CancelRunRoot ||
		cancelResult.Run.ID != "run_parked" ||
		cancelResult.Run.Status != protocol.RunStatusFinished {
		t.Fatalf("cancel result = %+v, want finished root run_parked", cancelResult)
	}
	exported, err := s.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: ses.ID})
	if err != nil {
		t.Fatalf("export canceled session: %v", err)
	}
	run := exported.Artifact.Runs[0]
	if run.Outcome.Type != "canceled" || run.Outcome.Error != nil {
		t.Fatalf("exported run = %+v, want a canceled terminal with no failure", run)
	}
	if run.MessageMark != 2 || run.Outcome.Detail != "user stopped" {
		t.Fatalf("exported mark/detail = %d/%q, want 2/user stopped", run.MessageMark, run.Outcome.Detail)
	}
	if got := exported.Artifact.Items[0].Status; got != "incomplete" {
		t.Fatalf("interrupt item status = %q, want incomplete", got)
	}
}

func TestRestoreSessionApplicationBoundaryRejectsOpenInterrupts(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := t.Context()
	restoreCWD := t.TempDir()

	ses, err := rt.sess.Create(ctx, "Old", "/proj")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := rt.interrupts.Open(ctx, serverPending(
		"run_old",
		ses.ID,
		"",
		"",
		nil,
		time.Now().UTC(),
	)); err != nil {
		t.Fatalf("seed interrupt: %v", err)
	}

	now := time.Now().UTC()
	_, err = s.sessions.RestorePortableSession(ctx, sessions.PortableSnapshot{Session: sessions.PortableSession{
		ID: ses.ID, Title: "Restored", CWD: restoreCWD, CreatedAt: now, UpdatedAt: now,
	}})
	if !errors.Is(err, sessions.ErrSessionBusy) {
		t.Fatalf("restore = %v, want ErrSessionBusy", err)
	}
	pending, err := rt.interrupts.List(ctx, ses.ID)
	if err != nil {
		t.Fatalf("list interrupts: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending interrupts = %+v, want untouched", pending)
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

// TestSessionImport_VersionMismatch rejects every non-current artifact version.
func TestSessionImport_VersionMismatch(t *testing.T) {
	s, _ := rollbackHarness(t)
	for _, version := range []int{0, protocol.SessionArtifactVersion - 1, protocol.SessionArtifactVersion + 1} {
		_, err := s.ImportSession(context.Background(), protocol.ImportSessionRequest{
			Artifact: protocol.SessionArtifact{Version: version, Session: protocol.ArtifactSession{ID: "ses_x"}},
		})
		if !errors.Is(err, protocol.ErrInvalidParams) {
			t.Fatalf("version %d mismatch err = %v, want ErrInvalidParams", version, err)
		}
	}
}

func TestSessionImportRejectsUnavailableCWD(t *testing.T) {
	s, _ := rollbackHarness(t)
	missing := t.TempDir() + "/missing"
	_, err := s.ImportSession(t.Context(), protocol.ImportSessionRequest{
		Artifact: protocol.SessionArtifact{
			Version: protocol.SessionArtifactVersion,
			Session: protocol.ArtifactSession{
				ID: "ses_missing_cwd", Workspace: protocol.WorkspaceRef{Path: missing},
			},
		},
	})
	if !errors.Is(err, protocol.ErrWorkspaceUnavailable) {
		t.Fatalf("import error = %v, want ErrWorkspaceUnavailable", err)
	}
}

// TestSessionImportRejectsAFailedRunWithoutItsFailure is the integration evidence
// that the import boundary maintains terminal_run_explains_how_it_ended.
//
// The snapshot validator refuses this shape, but a pure-function test on the
// validator only proves the rule exists — not that the write set is behind it. What
// the invariant protects against is an artifact restoring a run row that claims to
// have failed while carrying nothing that says how, which no later write repairs.
// So the rejection is asked of the use case, and the session is checked to be absent
// afterwards: a partial import is the same defect arriving one step later.
func TestSessionImportRejectsAFailedRunWithoutItsFailure(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := t.Context()
	created := time.Unix(1, 0).UTC()

	_, err := s.ImportSession(ctx, protocol.ImportSessionRequest{
		Artifact: protocol.SessionArtifact{
			Version: protocol.SessionArtifactVersion,
			Session: protocol.ArtifactSession{
				ID: "ses_unexplained", Title: "Restored", CreatedAt: created, UpdatedAt: created,
			},
			Runs: []protocol.ArtifactRun{{
				ID: "run_1", SessionID: "ses_unexplained",
				// Failed by its own account, with nothing that says how.
				Outcome:   protocol.ArtifactOutcome{Type: protocol.ArtifactOutcomeFailed},
				CreatedAt: created, FinishedAt: created, UpdatedAt: created,
			}},
		},
	})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("import err = %v, want ErrInvalidParams", err)
	}
	if _, err := rt.sess.Get(ctx, "ses_unexplained"); err == nil {
		t.Fatal("the refused import left a session behind")
	}
}

// The plan is part of what a person would notice losing, so it travels with
// the archive — and it comes back with a revision GREATER than the target
// session's, because a restore is a new commit of that projection. Restoring it at
// a lower number would leave a client that had already folded revision N ignoring
// the imported value as stale.
func TestSessionExportImportCarriesThePlanForward(t *testing.T) {
	s, rt := rollbackHarness(t)
	s.features.plan = true // this composition owns the key, so it may restore it
	ctx := t.Context()
	ses, err := rt.sess.Create(ctx, "planned", t.TempDir())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	putRun(t, rt, ses.ID, "run1", 1, 0)
	if err := rt.plan.Replace(ctx, ses.ID, []plan.Step{
		{Description: "split the outcome", Status: plan.StatusCompleted},
		{Description: "carry the list", Status: plan.StatusInProgress},
	}); err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	exported, err := s.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: ses.ID})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	states := exported.Artifact.States
	if len(states) != 1 || states[0].Type != protocol.ArtifactStatePlan || len(states[0].Plan) != 2 {
		t.Fatalf("artifact states = %+v, want the two-step plan", states)
	}
	if states[0].Plan[1].Description != "carry the list" || states[0].Plan[1].Status != protocol.PlanStatusInProgress {
		t.Fatalf("archived plan = %+v, want the in-progress entry verbatim", states[0].Plan[1])
	}

	// The live projection moves on, so the import has something to be newer than.
	if err := rt.plan.Replace(ctx, ses.ID, []plan.Step{{Description: "something else", Status: plan.StatusPending}}); err != nil {
		t.Fatalf("advance plan: %v", err)
	}
	before, err := rt.plan.State(ctx, ses.ID)
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}

	if _, err := s.ImportSession(ctx, protocol.ImportSessionRequest{Artifact: *exported.Artifact}); err != nil {
		t.Fatalf("import: %v", err)
	}
	after, err := rt.plan.State(ctx, ses.ID)
	if err != nil {
		t.Fatalf("read plan after import: %v", err)
	}
	if len(after.Steps) != 2 || after.Steps[0].Description != "split the outcome" {
		t.Fatalf("restored plan = %+v, want the archived plan", after.Steps)
	}
	if after.Revision <= before.Revision {
		t.Fatalf("restored revision = %d, want greater than the %d it replaced", after.Revision, before.Revision)
	}
}

// A build that does not own a state key cannot restore it, and importing the
// conversation while dropping the key would restore a session the archive does not
// describe. The refusal names the KEY, so the caller learns which one.
func TestSessionImportRefusesAnUnadvertisedStateKey(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := t.Context()
	s.features.plan = false
	ses, err := rt.sess.Create(ctx, "planned", t.TempDir())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	artifact := protocol.SessionArtifact{
		Version: protocol.SessionArtifactVersion,
		Session: protocol.ArtifactSession{
			ID: ses.ID, Title: "planned", Workspace: protocol.WorkspaceRef{Path: ses.CWD},
		},
		States: []protocol.ArtifactState{{
			Type: protocol.ArtifactStatePlan,
			Plan: []protocol.PlanSnapshot{{ID: "0", Description: "plan", Status: protocol.PlanStatusPending}},
		}},
	}
	_, err = s.ImportSession(ctx, protocol.ImportSessionRequest{Artifact: artifact})
	if !errors.Is(err, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("import err = %v, want capability_not_negotiated", err)
	}
	gap, ok := errors.AsType[*protocol.CapabilityGapError](err)
	if !ok || len(gap.Requirements) != 1 {
		t.Fatalf("gap = %+v, want one requirement", gap)
	}
	if gap.Requirements[0] != (protocol.CapabilityRequirement{
		Type: protocol.RequirementStateSnapshot, Name: string(protocol.ArtifactStatePlan),
	}) {
		t.Fatalf("requirement = %+v, want the plan state key", gap.Requirements[0])
	}
}
