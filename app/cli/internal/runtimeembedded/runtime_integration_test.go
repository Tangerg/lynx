package runtimeembedded

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func TestEmbeddedRuntimeSessionCatalogAndLifecycle(t *testing.T) {
	t.Setenv("LYRA_PROVIDER", "anthropic")
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("LYRA_MCP_SERVERS", "")
	t.Setenv("LYRA_A2A_AGENTS", "")
	t.Setenv("LYRA_A2A_RPC_ORIGINS", "")

	workspace := t.TempDir()
	runtime, err := Open(t.Context(), Config{
		DataDirectory: t.TempDir(), DefaultWorkspacePath: workspace,
		UserHomePath: t.TempDir(), ConfigDirectories: []string{t.TempDir()}, ClientVersion: "test",
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

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

	updated, err := runtime.UpdateSession(t.Context(), agent.UpdateSession{
		SessionID: created.ID, Title: "renamed adapter session", ExpectedRevision: created.Revision,
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

	models, err := runtime.ListModels(t.Context())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("ListModels returned no provider-qualified models")
	}
	if rules, err := runtime.ListApprovalRules(t.Context(), created.ID); err != nil || len(rules) != 0 {
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

	if err := runtime.DeleteSession(t.Context(), agent.DeleteSession{SessionID: created.ID}); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if err := runtime.DeleteSession(t.Context(), agent.DeleteSession{SessionID: forked.ID}); err != nil {
		t.Fatalf("DeleteSession fork: %v", err)
	}
	if _, err := runtime.GetSession(t.Context(), created.ID); !errors.Is(err, agent.ErrSessionNotFound) {
		t.Fatalf("GetSession after delete = %v, want ErrSessionNotFound", err)
	} else {
		var problem protocol.ProblemError
		if !errors.As(err, &problem) || problem.Problem().Type != protocol.ErrSessionNotFound.Error() {
			t.Fatalf("structured GetSession error = %T %v", err, err)
		}
	}

	if err := runtime.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := runtime.ListModels(t.Context()); !errors.Is(err, agent.ErrDisconnected) {
		t.Fatalf("ListModels after Close = %v, want ErrDisconnected", err)
	}
}

func TestOwnerOpensOnceAndRefusesReopenAfterClose(t *testing.T) {
	t.Setenv("LYRA_PROVIDER", "anthropic")
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("LYRA_MCP_SERVERS", "")
	t.Setenv("LYRA_A2A_AGENTS", "")
	t.Setenv("LYRA_A2A_RPC_ORIGINS", "")

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
