package runtimeembedded

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/changefeed"
	"github.com/Tangerg/lynx/app/cli/internal/sessiontransfer"
	workspaceapi "github.com/Tangerg/lynx/app/cli/internal/workspace"
)

func TestEmbeddedRuntimeSessionCatalogAndLifecycle(t *testing.T) {
	configureIntegrationRuntime(t)
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n\nvar answer = 42\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime := openIntegrationRuntime(t, workspace)
	created := requireSessionCatalog(t, runtime, workspace)
	requireWorkspaceInspection(t, runtime, workspace)
	forked := requireSessionMutation(t, runtime, created)
	requireSessionPortability(t, runtime, forked.ID)
	requireRuntimeCatalogs(t, runtime, created.ID)
	requireSessionDeletion(t, runtime, created.ID, forked.ID)
	requireClosedRuntime(t, runtime)
}

func requireSessionPortability(t *testing.T, runtime *Runtime, sessionID string) {
	t.Helper()
	markdown, err := runtime.ExportSession(t.Context(), sessiontransfer.ExportRequest{
		SessionID: sessionID, Format: sessiontransfer.Markdown,
	})
	if err != nil || len(markdown.Bytes()) == 0 || markdown.Importable() {
		t.Fatalf("Markdown ExportSession = (%q, %v)", markdown.Bytes(), err)
	}
	artifact, err := runtime.ExportSession(t.Context(), sessiontransfer.ExportRequest{
		SessionID: sessionID, Format: sessiontransfer.JSON,
	})
	if err != nil || !artifact.Importable() {
		t.Fatalf("JSON ExportSession = (%q, %v)", artifact.Bytes(), err)
	}
	imported, err := runtime.ImportSession(t.Context(), sessiontransfer.ImportRequest{Artifact: artifact})
	if err != nil || imported.ID != sessionID {
		t.Fatalf("ImportSession = (%+v, %v)", imported, err)
	}
	rolledBack, err := runtime.RollbackSession(t.Context(), agent.RollbackSession{
		SessionID: sessionID, Scope: agent.RestoreHistory,
	})
	if err != nil || rolledBack.Session.ID != sessionID || len(rolledBack.Dropped) != 0 {
		t.Fatalf("RollbackSession = (%+v, %v)", rolledBack, err)
	}
}

func requireWorkspaceInspection(t *testing.T, runtime *Runtime, path string) {
	t.Helper()
	if !runtime.Supports(changefeed.FilesChanged) {
		t.Fatal("embedded runtime did not advertise files.changed")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := runtime.Resolve(t.Context(), workspaceapi.ResolveRequest{Path: path})
	if err != nil || resolved.Path != canonical || !resolved.IsAvailable() {
		t.Fatalf("Resolve = (%+v, %v)", resolved, err)
	}
	path = resolved.Path
	known, err := runtime.List(t.Context())
	if err != nil || len(known) == 0 {
		t.Fatalf("List = (%+v, %v)", known, err)
	}
	files, err := runtime.Files(t.Context(), workspaceapi.FilesRequest{Workspace: path, Limit: 20})
	if err != nil || len(files.Entries) != 1 || files.Entries[0].Path != "main.go" {
		t.Fatalf("Files = (%+v, %v)", files, err)
	}
	head, err := runtime.Head(t.Context(), workspaceapi.HeadRequest{Workspace: path, Path: "main.go", Lines: 2})
	if err != nil || len(head.Lines) != 2 || head.Lines[0].Text != "package main" {
		t.Fatalf("Head = (%+v, %v)", head, err)
	}
	found, err := runtime.Search(t.Context(), workspaceapi.SearchRequest{Workspace: path, Query: "answer", Limit: 20})
	if err != nil || found.Total != 1 || len(found.Matches) != 1 {
		t.Fatalf("Search = (%+v, %v)", found, err)
	}
	content, err := runtime.Read(t.Context(), workspaceapi.ReadRequest{Workspace: path, Path: "main.go"})
	if err != nil || content.TotalLines != 4 || content.Content == "" {
		t.Fatalf("Read = (%+v, %v)", content, err)
	}
}

func configureIntegrationRuntime(t *testing.T) {
	t.Helper()
	t.Setenv("LYRA_PROVIDER", "anthropic")
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("LYRA_MCP_SERVERS", "")
	t.Setenv("LYRA_A2A_AGENTS", "")
	t.Setenv("LYRA_A2A_RPC_ORIGINS", "")
}

func openIntegrationRuntime(t *testing.T, workspace string) *Runtime {
	t.Helper()
	runtime, err := Open(t.Context(), Config{
		DataDirectory: t.TempDir(), DefaultWorkspacePath: workspace,
		UserHomePath: t.TempDir(), ConfigDirectories: []string{t.TempDir()}, ClientVersion: "test",
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	return runtime
}

func requireSessionCatalog(t *testing.T, runtime *Runtime, workspace string) agent.Session {
	t.Helper()
	created, err := runtime.CreateSession(t.Context(), agent.CreateSession{Title: "adapter session", Workspace: workspace})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	page, err := runtime.ListSessions(t.Context(), agent.SessionQuery{
		Limit: 10, Search: "ADAPTER", Workspace: created.Workspace,
	})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != created.ID {
		t.Fatalf("filtered sessions = %+v, want %s", page.Items, created.ID)
	}

	snapshot, err := runtime.GetSession(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.Session.ID != created.ID || len(snapshot.Runs) != 0 || len(snapshot.Transcript) != 0 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	return created
}

func requireSessionMutation(t *testing.T, runtime *Runtime, created agent.Session) agent.Session {
	t.Helper()
	title := "renamed adapter session"
	updated, err := runtime.UpdateSession(t.Context(), agent.UpdateSession{
		SessionID: created.ID, Title: &title, ExpectedRevision: created.Revision,
	})
	if err != nil {
		t.Fatalf("UpdateSession: %v", err)
	}
	if updated.Title != "renamed adapter session" || updated.Revision <= created.Revision {
		t.Fatalf("updated = %+v", updated)
	}
	forked, err := runtime.ForkSession(t.Context(), agent.ForkSession{SessionID: created.ID, Title: "forked adapter session"})
	if err != nil {
		t.Fatalf("ForkSession: %v", err)
	}
	if forked.ID == created.ID || forked.Title != "forked adapter session" {
		t.Fatalf("forked = %+v", forked)
	}
	return forked
}

func requireRuntimeCatalogs(t *testing.T, runtime *Runtime, sessionID string) {
	t.Helper()
	models, err := runtime.ListModels(t.Context())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("ListModels returned no provider-qualified models")
	}
	if rules, err := runtime.ListApprovalRules(t.Context(), sessionID); err != nil || len(rules) != 0 {
		t.Fatalf("ListApprovalRules = (%+v, %v)", rules, err)
	}

	applied, err := runtime.SetApprovalMode(t.Context(), agent.ApprovalModeSafe)
	if err != nil || applied != agent.ApprovalModeSafe {
		t.Fatalf("SetApprovalMode = (%q, %v)", applied, err)
	}
	mode, err := runtime.GetApprovalMode(t.Context())
	if err != nil || mode != agent.ApprovalModeSafe {
		t.Fatalf("GetApprovalMode = (%q, %v)", mode, err)
	}
}

func requireSessionDeletion(t *testing.T, runtime *Runtime, sessionIDs ...string) {
	t.Helper()
	for _, sessionID := range sessionIDs {
		if err := runtime.DeleteSession(t.Context(), agent.DeleteSession{SessionID: sessionID}); err != nil {
			t.Fatalf("DeleteSession %s: %v", sessionID, err)
		}
	}
	_, err := runtime.GetSession(t.Context(), sessionIDs[0])
	if !errors.Is(err, agent.ErrSessionNotFound) {
		t.Fatalf("GetSession after delete = %v, want ErrSessionNotFound", err)
	}
	problem, ok := errors.AsType[protocol.ProblemError](err)
	if !ok || problem.Problem().Type != protocol.ErrSessionNotFound.Error() {
		t.Fatalf("structured GetSession error = %T %v", err, err)
	}
}

func requireClosedRuntime(t *testing.T, runtime *Runtime) {
	t.Helper()
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := runtime.ListModels(t.Context()); !errors.Is(err, agent.ErrDisconnected) {
		t.Fatalf("ListModels after Close = %v, want ErrDisconnected", err)
	}
}

func TestOwnerOpensOnceAndRefusesReopenAfterClose(t *testing.T) {
	configureIntegrationRuntime(t)

	owner := NewOwner(Config{
		DataDirectory: t.TempDir(), DefaultWorkspacePath: t.TempDir(),
		UserHomePath: t.TempDir(), ConfigDirectories: []string{t.TempDir()}, ClientVersion: "test",
	})
	first, err := owner.Runtime(t.Context())
	if err != nil {
		t.Fatalf("first Runtime: %v", err)
	}
	second, err := owner.Runtime(t.Context())
	if err != nil {
		t.Fatalf("second Runtime: %v", err)
	}
	if first != second {
		t.Fatal("owner opened more than one runtime")
	}
	if err := owner.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := owner.Runtime(t.Context()); !errors.Is(err, agent.ErrDisconnected) {
		t.Fatalf("Runtime after Close = %v, want ErrDisconnected", err)
	}
}
