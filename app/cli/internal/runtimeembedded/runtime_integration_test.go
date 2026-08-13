package runtimeembedded

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agentmemory"
	"github.com/Tangerg/lynx/app/cli/internal/changefeed"
	"github.com/Tangerg/lynx/app/cli/internal/feedback"
	"github.com/Tangerg/lynx/app/cli/internal/goal"
	"github.com/Tangerg/lynx/app/cli/internal/knowledge"
	"github.com/Tangerg/lynx/app/cli/internal/mcp"
	"github.com/Tangerg/lynx/app/cli/internal/modelconfig"
	"github.com/Tangerg/lynx/app/cli/internal/schedule"
	"github.com/Tangerg/lynx/app/cli/internal/sessiontransfer"
	workspaceapi "github.com/Tangerg/lynx/app/cli/internal/workspace"
)

func TestEmbeddedRuntimeSessionCatalogAndLifecycle(t *testing.T) {
	configureIntegrationRuntime(t)
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n\nvar answer = 42\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(workspace, "empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	runtime := openIntegrationRuntime(t, workspace)
	created := requireSessionCatalog(t, runtime, workspace)
	requireWorkspaceInspection(t, runtime, workspace)
	forked := requireSessionMutation(t, runtime, created, t.TempDir())
	requireSessionPortability(t, runtime, forked.ID)
	requireRuntimeCatalogs(t, runtime, created.ID, created.Workspace.Path)
	requireProviderMutationLifecycle(t, runtime)
	requireGoalMutationLifecycle(t, runtime, created.ID)
	requireContextManagement(t, runtime, created.Workspace.Path)
	requireAuxiliaryCapabilities(t, runtime, created.ID, created.Workspace.Path)
	requireExternalAuthoredInvalidations(t, runtime, created.Workspace.Path)
	requireSessionDeletion(t, runtime, created.ID, forked.ID)
	requireClosedRuntime(t, runtime)
}

func requireGoalMutationLifecycle(t *testing.T, runtime *Runtime, sessionID string) {
	t.Helper()
	start := goal.Start{
		SessionID: sessionID, Objective: "verify embedded goal lifecycle",
		Provider: "missing", Model: "missing", Budget: goal.Budget{MaxRuns: 3},
	}
	started, err := runtime.StartGoal(t.Context(), start)
	if err != nil {
		t.Fatalf("StartGoal: %v", err)
	}
	if err := start.ValidateResult(started); err != nil {
		t.Fatalf("started goal: %v", err)
	}
	stopped, err := runtime.StopGoal(t.Context(), sessionID)
	if err != nil {
		t.Fatalf("StopGoal: %v", err)
	}
	if stopped.Status == goal.Active {
		t.Fatalf("stopped goal remained active: %+v", stopped)
	}
	resumed, err := runtime.ResumeGoal(t.Context(), sessionID)
	if err != nil {
		t.Fatalf("ResumeGoal: %v", err)
	}
	if resumed.Status != goal.Active {
		t.Fatalf("resumed goal = %+v", resumed)
	}
	if _, err := runtime.StopGoal(t.Context(), sessionID); err != nil {
		t.Fatalf("final StopGoal: %v", err)
	}
}

func requireExternalAuthoredInvalidations(t *testing.T, runtime *Runtime, workspace string) {
	t.Helper()
	for _, topic := range []changefeed.Topic{changefeed.KnowledgeChanged, changefeed.HooksChanged} {
		if !runtime.Supports(topic) {
			t.Fatalf("embedded runtime did not advertise %s", topic)
		}
	}

	streamContext, cancelStream := context.WithCancel(t.Context())
	stream, err := runtime.Subscribe(streamContext, changefeed.Subscription{
		Topics: []changefeed.Topic{
			changefeed.FilesChanged,
			changefeed.KnowledgeChanged,
			changefeed.HooksChanged,
		},
		Watches: []changefeed.Watch{{ID: "authored-resources", Workspace: workspace}},
	})
	if err != nil {
		t.Fatalf("subscribe to authored resources: %v", err)
	}
	events := make(chan changefeed.Event, 8)
	streamErrors := make(chan error, 1)
	streamStopped := make(chan struct{})
	go func() {
		defer close(streamStopped)
		for event, streamErr := range stream {
			if streamErr != nil {
				streamErrors <- streamErr
				return
			}
			select {
			case events <- event:
			case <-streamContext.Done():
				return
			}
		}
	}()
	defer func() {
		cancelStream()
		select {
		case <-streamStopped:
		case <-time.After(3 * time.Second):
			t.Error("authored-resource subscription did not stop")
		}
	}()

	knowledgePath := filepath.Join(workspace, "LYRA.md")
	if err := os.WriteFile(knowledgePath, []byte("# External knowledge\n"), 0o600); err != nil {
		t.Fatalf("write external knowledge: %v", err)
	}
	awaitRuntimeInvalidation(t, events, streamErrors, changefeed.KnowledgeChanged)
	target, err := knowledge.NewTarget(knowledge.WorkingDirectory, workspace)
	if err != nil {
		t.Fatal(err)
	}
	document, err := runtime.services().Knowledge.Document(t.Context(), target)
	if err != nil || document.Content != "# External knowledge\n" {
		t.Fatalf("knowledge after external invalidation = (%+v, %v)", document, err)
	}

	hooksDirectory := filepath.Join(workspace, ".lyra")
	if err := os.MkdirAll(hooksDirectory, 0o700); err != nil {
		t.Fatalf("create external hooks directory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(hooksDirectory, "hooks.json"),
		[]byte(`{"hooks":[{"event":"SessionStart","inject":"external context"}]}`),
		0o600,
	); err != nil {
		t.Fatalf("write external hooks: %v", err)
	}
	awaitRuntimeInvalidation(t, events, streamErrors, changefeed.HooksChanged)
	catalog, err := runtime.services().Hooks.Catalog(t.Context(), workspace)
	if err != nil || len(catalog.Hooks) != 1 || catalog.Hooks[0].Inject != "external context" {
		t.Fatalf("hooks after external invalidation = (%+v, %v)", catalog, err)
	}
}

func awaitRuntimeInvalidation(
	t *testing.T,
	events <-chan changefeed.Event,
	streamErrors <-chan error,
	topic changefeed.Topic,
) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-events:
			if event.Type == changefeed.EventType(topic) {
				return
			}
		case err := <-streamErrors:
			t.Fatalf("wait for %s: %v", topic, err)
		case <-timer.C:
			t.Fatalf("no %s invalidation after external edit", topic)
		}
	}
}

func requireAuxiliaryCapabilities(t *testing.T, runtime *Runtime, sessionID, workspace string) {
	t.Helper()
	services := runtime.services()
	if services.DiagnosticTools == nil || services.AuthoringContext == nil || services.Hooks == nil || services.Feedback == nil {
		t.Fatalf("stable auxiliary services were not composed: %+v", services)
	}
	tools, err := services.DiagnosticTools.Tools(t.Context())
	if err != nil || len(tools) == 0 {
		t.Fatalf("DiagnosticTools = (%+v, %v)", tools, err)
	}
	if documents, err := services.AuthoringContext.Documents(t.Context(), workspace); err != nil {
		t.Fatalf("Agent documents = (%+v, %v)", documents, err)
	}
	if recipes, err := services.AuthoringContext.Recipes(t.Context(), workspace); err != nil {
		t.Fatalf("Recipes = (%+v, %v)", recipes, err)
	}
	if hooks, err := services.Hooks.Catalog(t.Context(), workspace); err != nil {
		t.Fatalf("Hooks = (%+v, %v)", hooks, err)
	}
	if err := services.Feedback.Record(t.Context(), feedback.Signal{
		SessionID: sessionID, Rating: feedback.Positive, Text: "embedded integration",
	}); err != nil {
		t.Fatalf("Create feedback: %v", err)
	}
	if services.Codebase != nil {
		if status, err := services.Codebase.Status(t.Context(), workspace); err != nil {
			t.Fatalf("Codebase status = (%+v, %v)", status, err)
		}
	}
}

func requireContextManagement(t *testing.T, runtime *Runtime, workspace string) {
	t.Helper()
	services := runtime.services()
	if services.AgentMemory == nil || services.Knowledge == nil {
		t.Fatalf("context services were not advertised: %+v", services)
	}
	userTarget, err := agentmemory.NewTarget(agentmemory.User, "")
	if err != nil {
		t.Fatal(err)
	}
	added, err := services.AgentMemory.Add(t.Context(), userTarget, "integration preference")
	if err != nil {
		t.Fatalf("Add agent memory: %v", err)
	}
	items, err := services.AgentMemory.Items(t.Context(), userTarget)
	if err != nil || len(items) != 1 || items[0].ID != added.ID {
		t.Fatalf("Items agent memory = (%+v, %v)", items, err)
	}
	pinned := true
	updated, err := services.AgentMemory.Update(t.Context(), agentmemory.Patch{ID: added.ID, Pinned: &pinned})
	if err != nil || !updated.Pinned {
		t.Fatalf("Update agent memory = (%+v, %v)", updated, err)
	}
	if err := services.AgentMemory.Delete(t.Context(), added.ID); err != nil {
		t.Fatalf("Delete agent memory: %v", err)
	}
	entries, err := services.Knowledge.Entries(t.Context(), workspace)
	if err != nil {
		t.Fatalf("Entries knowledge = (%+v, %v)", entries, err)
	}
	target, err := knowledge.NewTarget(knowledge.WorkingDirectory, workspace)
	if err != nil {
		t.Fatal(err)
	}
	before, err := services.Knowledge.Document(t.Context(), target)
	if err != nil {
		t.Fatalf("read knowledge before save: %v", err)
	}
	update, err := before.Revise(target, "# Integration knowledge\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := services.Knowledge.Save(t.Context(), update); err != nil {
		t.Fatalf("Save knowledge: %v", err)
	}
	document, err := services.Knowledge.Document(t.Context(), target)
	if err != nil || document.Content != "# Integration knowledge\n" {
		t.Fatalf("Document knowledge = (%+v, %v)", document, err)
	}
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
	files, err := runtime.Files(t.Context(), workspaceapi.FilesRequest{Workspace: path})
	if err != nil || len(files.Entries) != 2 || files.Entries[0].Path != "empty" ||
		files.Entries[0].Type != workspaceapi.FileEntryDirectory || files.Entries[1].Path != "main.go" {
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
	t.Setenv("DEEPSEEK_API_KEY", "integration-env-key")
	t.Setenv("LYRA_MCP_SERVERS", "")
	t.Setenv("LYRA_A2A_AGENTS", "")
	t.Setenv("LYRA_A2A_RPC_ORIGINS", "")
}

func requireProviderMutationLifecycle(t *testing.T, runtime *Runtime) {
	t.Helper()
	setBaseURL := modelconfig.ValueChange{Kind: modelconfig.SetValue, Value: "https://provider.integration.test"}
	setAPIKey := modelconfig.ValueChange{Kind: modelconfig.SetValue, Value: "integration-stored-key"}
	configured, err := runtime.UpdateProvider(t.Context(), modelconfig.UpdateProvider{
		Provider: "deepseek", BaseURL: &setBaseURL, APIKey: &setAPIKey,
	})
	if err != nil {
		t.Fatalf("configure provider: %v", err)
	}
	if configured.BaseURL != setBaseURL.Value || configured.APIKeyMasked == "" || configured.KeySource != modelconfig.KeyStored {
		t.Fatalf("configured provider = %+v", configured)
	}

	clear := modelconfig.ValueChange{Kind: modelconfig.ClearValue}
	fallback, err := runtime.UpdateProvider(t.Context(), modelconfig.UpdateProvider{
		Provider: "deepseek", BaseURL: &clear, APIKey: &clear,
	})
	if err != nil {
		t.Fatalf("clear provider: %v", err)
	}
	if fallback.BaseURL != "" || fallback.APIKeyMasked == "" || fallback.KeySource != modelconfig.KeyEnv {
		t.Fatalf("provider environment fallback = %+v", fallback)
	}
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
		Limit: 10, Search: "ADAPTER", Workspace: created.Workspace.Path,
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
	runs, err := runtime.ListRuns(t.Context(), agent.RunQuery{SessionID: created.ID, IncludeDescendants: true})
	if err != nil || len(runs.Items) != 0 {
		t.Fatalf("ListRuns = (%+v, %v)", runs, err)
	}
	if _, err := runtime.GetRun(t.Context(), "run_missing"); !errors.Is(err, agent.ErrRunNotFound) {
		t.Fatalf("GetRun missing = %v, want ErrRunNotFound", err)
	}
	return created
}

func requireSessionMutation(t *testing.T, runtime *Runtime, created agent.Session, workspace string) agent.Session {
	t.Helper()
	title, model, favorite := "renamed adapter session", "integration-model", true
	updated, err := runtime.UpdateSession(t.Context(), agent.UpdateSession{
		SessionID: created.ID, Title: &title, Workspace: &workspace, Model: &model,
		Favorite: &favorite, ExpectedRevision: created.Revision,
	})
	if err != nil {
		t.Fatalf("UpdateSession: %v", err)
	}
	canonicalWorkspace, canonicalErr := filepath.EvalSymlinks(workspace)
	if canonicalErr != nil {
		t.Fatal(canonicalErr)
	}
	if updated.Title != title || updated.Workspace.Path != canonicalWorkspace || updated.Model != model ||
		!updated.Favorite || updated.Revision <= created.Revision {
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

func requireRuntimeCatalogs(t *testing.T, runtime *Runtime, sessionID, workspace string) {
	t.Helper()
	models, err := runtime.ListModels(t.Context())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("ListModels returned no provider-qualified models")
	}
	providers, err := runtime.Providers(t.Context())
	if err != nil || len(providers) == 0 {
		t.Fatalf("Providers = (%+v, %v)", providers, err)
	}
	roles, err := runtime.Roles(t.Context())
	if err != nil {
		t.Fatalf("Roles = (%+v, %v)", roles, err)
	}
	sessionUsage, err := runtime.SessionUsage(t.Context(), sessionID)
	if err != nil || sessionUsage.SessionID != sessionID {
		t.Fatalf("SessionUsage = (%+v, %v)", sessionUsage, err)
	}
	usageSummary, err := runtime.Summary(t.Context(), 30)
	if err != nil || usageSummary.SinceDays != 30 {
		t.Fatalf("Summary = (%+v, %v)", usageSummary, err)
	}
	if current, exists, err := runtime.GetGoal(t.Context(), sessionID); err != nil || exists {
		t.Fatalf("GetGoal without a goal = (%+v, %t, %v)", current, exists, err)
	}
	if discovered, err := runtime.Discover(t.Context(), workspace); err != nil {
		t.Fatalf("Discover skills = (%+v, %v)", discovered, err)
	}
	if managed, err := runtime.Managed(t.Context()); err != nil {
		t.Fatalf("Managed skills = (%+v, %v)", managed, err)
	}
	if proposals, err := runtime.Proposals(t.Context(), workspace); err != nil {
		t.Fatalf("Skill proposals = (%+v, %v)", proposals, err)
	}
	if servers, err := runtime.Servers(t.Context()); err != nil {
		t.Fatalf("MCP servers = (%+v, %v)", servers, err)
	}
	if tools, err := runtime.Tools(t.Context(), ""); err != nil {
		t.Fatalf("MCP tools = (%+v, %v)", tools, err)
	}
	requireMCPMutationLifecycle(t, runtime)
	requireScheduleLifecycle(t, runtime, workspace)
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

func requireMCPMutationLifecycle(t *testing.T, runtime *Runtime) {
	t.Helper()
	authorization := mcp.AuthorizationChange{Kind: mcp.Set, Value: "Bearer integration-secret"}
	headers := mcp.HeadersChange{Kind: mcp.Set, Value: map[string]string{"X-Key": "integration-secret"}}
	candidate := mcp.Candidate{
		Name: "integration-docs", Enabled: false, Description: "Integration MCP",
		Connection: mcp.ConnectionInput{
			Transport: mcp.StreamableHTTP, URL: "https://mcp.example/tools",
			Authorization: &authorization, Headers: &headers,
		},
		TimeoutSeconds: 5, DisabledTools: []string{"write"}, AutoApproveTools: []string{"search"},
	}
	created, err := runtime.CreateServer(t.Context(), candidate)
	if err != nil {
		t.Fatalf("Create MCP server: %v", err)
	}
	if err := candidate.ValidateResult(created); err != nil {
		t.Fatalf("created MCP server: %v", err)
	}
	clearAuthorization := mcp.AuthorizationChange{Kind: mcp.Clear}
	clearHeaders := mcp.HeadersChange{Kind: mcp.Clear}
	description, timeout := "Updated integration MCP", 10
	update := mcp.ServerUpdate{
		Server: candidate.Name, Description: &description, TimeoutSeconds: &timeout,
		Connection: &mcp.ConnectionInput{
			Transport: mcp.StreamableHTTP, URL: candidate.Connection.URL,
			Authorization: &clearAuthorization, Headers: &clearHeaders,
		},
	}
	updated, err := runtime.UpdateServer(t.Context(), update)
	if err != nil {
		t.Fatalf("Update MCP server: %v", err)
	}
	if err := update.ValidateResult(updated); err != nil {
		t.Fatalf("updated MCP server: %v", err)
	}
	if err := runtime.DeleteServer(t.Context(), candidate.Name); err != nil {
		t.Fatalf("Delete MCP server: %v", err)
	}
}

func requireScheduleLifecycle(t *testing.T, runtime *Runtime, workspace string) {
	t.Helper()
	created, err := runtime.Create(t.Context(), schedule.Candidate{
		Title: "Adapter schedule", Instructions: "review the workspace", Workspace: workspace, Cron: "0 9 * * 1-5",
	})
	if err != nil {
		t.Fatalf("Create schedule: %v", err)
	}
	listed, err := runtime.Schedules(t.Context())
	if err != nil || len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("Schedules = (%+v, %v)", listed, err)
	}
	enabled := false
	updated, err := runtime.Update(t.Context(), schedule.Patch{
		ID: created.ID, ExpectedRevision: created.Revision, Enabled: &enabled,
	})
	if err != nil || updated.Enabled || updated.Revision <= created.Revision {
		t.Fatalf("Update schedule = (%+v, %v)", updated, err)
	}
	if err := runtime.Delete(t.Context(), created.ID); err != nil {
		t.Fatalf("Delete schedule: %v", err)
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
	if first.Agent != second.Agent {
		t.Fatal("owner opened more than one runtime")
	}
	if first.RuntimeProfile == nil || second.RuntimeProfile == nil || first.RuntimeProfile == second.RuntimeProfile {
		t.Fatal("owner did not return independently owned runtime profiles")
	}
	if first.Goals == nil || first.Skills == nil || first.MCP == nil || first.Schedules == nil ||
		first.AgentMemory == nil || first.Knowledge == nil || first.DiagnosticTools == nil ||
		first.AuthoringContext == nil || first.Hooks == nil || first.Feedback == nil {
		t.Fatalf("advertised optional services were not composed: %+v", first)
	}
	if err := owner.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := owner.Runtime(t.Context()); !errors.Is(err, agent.ErrDisconnected) {
		t.Fatalf("Runtime after Close = %v, want ErrDisconnected", err)
	}
}
