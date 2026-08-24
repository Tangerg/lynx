package runtimehost_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/httptransport"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/runtimehost"
	"github.com/Tangerg/sse"
)

func TestPublicRunLifecycleNormalQuestionApprovalAndDelegation(t *testing.T) {
	harness := newAcceptanceHarness(t)
	model, runtime, namespace, meta := harness.model, harness.runtime, harness.namespace, harness.meta

	t.Run("normal", func(t *testing.T) {
		session := createRunSession(t, runtime.baseURL, namespace, "normal")
		ack, events := rpcRunStream[*protocol.StartRunResponse](
			t, runtime.baseURL, "runs.start", protocol.StartRunRequest{
				SessionID: session.ID,
				Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "SCENARIO_NORMAL"}},
				Provider:  "openai-compatible", Model: "test-model",
			}, meta, "normal-run", namespace,
		)
		assertCompletedRunStream(t, ack.RunID, events)
		assertFinishedRun(t, runtime.baseURL, ack.RunID, meta)
		items := rpcCallWithMeta[*protocol.ListItemsResponse](
			t, runtime.baseURL, "items.list", protocol.ListItemsRequest{
				Scope:     protocol.ItemListScope{Type: protocol.ItemScopeSession, SessionID: session.ID},
				PageQuery: protocol.PageQuery{Limit: 100},
			}, meta,
		)
		if !containsFinalText(items.Data, "normal complete") {
			t.Fatalf("normal transcript omitted final answer: %+v", items.Data)
		}
	})

	t.Run("question", func(t *testing.T) {
		session := createRunSession(t, runtime.baseURL, namespace, "question")
		ack, events := rpcRunStream[*protocol.StartRunResponse](
			t, runtime.baseURL, "runs.start", protocol.StartRunRequest{
				SessionID: session.ID,
				Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "SCENARIO_QUESTION"}},
				Provider:  "openai-compatible", Model: "test-model",
			}, meta, "question-run", namespace,
		)
		assertInterruptedRunStream(t, ack.RunID, events)
		pending := rpcCallWithMeta[*protocol.Page[protocol.PendingInterruptSet]](
			t, runtime.baseURL, "interrupts.list",
			protocol.ListInterruptsRequest{SessionID: session.ID, RootRunID: ack.RunID}, meta,
		)
		if len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 ||
			pending.Data[0].Interrupts[0].Type != protocol.InterruptQuestion {
			t.Fatalf("pending question = %+v", pending.Data)
		}
		interrupt := pending.Data[0].Interrupts[0]
		resumed, resumedEvents := rpcRunStream[*protocol.ResumeRunResponse](
			t, runtime.baseURL, "runs.resume", protocol.ResumeRunRequest{
				RunID: ack.RunID,
				Responses: []protocol.InterruptResponse{{
					ItemID: interrupt.ItemID,
					Response: protocol.InterruptResponseValue{
						Type: protocol.InterruptResponseAnswer, Answers: [][]string{{"Blue"}},
					},
				}},
			}, meta, "question-resume", namespace,
		)
		if resumed.RunID != ack.RunID || resumed.SegmentID == ack.SegmentID {
			t.Fatalf("question resume ack = %+v, start = %+v", resumed, ack)
		}
		assertCompletedRunStream(t, ack.RunID, resumedEvents)
		assertFinishedRun(t, runtime.baseURL, ack.RunID, meta)
	})

	t.Run("approval", func(t *testing.T) {
		session := createRunSession(t, runtime.baseURL, namespace, "approval")
		ack, events := rpcRunStream[*protocol.StartRunResponse](
			t, runtime.baseURL, "runs.start", protocol.StartRunRequest{
				SessionID: session.ID,
				Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "SCENARIO_APPROVAL"}},
				Provider:  "openai-compatible", Model: "test-model",
			}, meta, "approval-run", namespace,
		)
		assertInterruptedRunStream(t, ack.RunID, events)
		pending := rpcCallWithMeta[*protocol.Page[protocol.PendingInterruptSet]](
			t, runtime.baseURL, "interrupts.list",
			protocol.ListInterruptsRequest{SessionID: session.ID, RootRunID: ack.RunID}, meta,
		)
		if len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 ||
			pending.Data[0].Interrupts[0].Type != protocol.InterruptApproval {
			t.Fatalf("pending approval = %+v", pending.Data)
		}
		interrupt := pending.Data[0].Interrupts[0]
		_, resumedEvents := rpcRunStream[*protocol.ResumeRunResponse](
			t, runtime.baseURL, "runs.resume", protocol.ResumeRunRequest{
				RunID: ack.RunID,
				Responses: []protocol.InterruptResponse{{
					ItemID: interrupt.ItemID,
					Response: protocol.InterruptResponseValue{
						Type: protocol.InterruptResponseApproval, Decision: protocol.ApprovalApprove,
					},
				}},
			}, meta, "approval-resume", namespace,
		)
		assertCompletedRunStream(t, ack.RunID, resumedEvents)
		assertFinishedRun(t, runtime.baseURL, ack.RunID, meta)
	})

	t.Run("delegation", func(t *testing.T) {
		session := createRunSession(t, runtime.baseURL, namespace, "delegation")
		ack, events := rpcRunStream[*protocol.StartRunResponse](
			t, runtime.baseURL, "runs.start", protocol.StartRunRequest{
				SessionID: session.ID,
				Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "SCENARIO_DELEGATE"}},
				Provider:  "openai-compatible", Model: "test-model",
			}, meta, "delegation-run", namespace,
		)
		assertCompletedRunStream(t, ack.RunID, events)
		runs := rpcCallWithMeta[*protocol.Page[protocol.RunRef]](
			t, runtime.baseURL, "runs.list", protocol.ListRunsRequest{
				SessionID: session.ID, IncludeDescendants: true, PageQuery: protocol.PageQuery{Limit: 100},
			}, meta,
		)
		if len(runs.Data) != 2 {
			t.Fatalf("delegated Run tree = %+v", runs.Data)
		}
		childFound := false
		for _, run := range runs.Data {
			if run.ID != ack.RunID && run.ParentRunID == ack.RunID && run.RootRunID == ack.RunID {
				childFound = true
			}
		}
		if !childFound {
			t.Fatalf("delegated Run tree lacks child lineage: %+v", runs.Data)
		}
	})

	t.Run("Plan", func(t *testing.T) {
		session := createRunSession(t, runtime.baseURL, namespace, "plan")
		ack, events := rpcRunStream[*protocol.StartRunResponse](
			t, runtime.baseURL, "runs.start", protocol.StartRunRequest{
				SessionID: session.ID,
				Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "SCENARIO_PLAN"}},
				Provider:  "openai-compatible", Model: "test-model",
			}, meta, "plan-run", namespace,
		)
		assertCompletedRunStream(t, ack.RunID, events)
		if !containsPlanUpdate(events, session.ID) {
			t.Fatalf("Plan Run omitted plan.updated: %+v", events)
		}
		plan := rpcCallWithMeta[*protocol.Plan](
			t, runtime.baseURL, "plan.get", protocol.GetPlanRequest{SessionID: session.ID}, meta,
		)
		if plan.Revision == 0 || len(plan.Steps) != 2 || plan.Steps[0].Status != protocol.PlanStatusInProgress {
			t.Fatalf("durable Plan = %+v", plan)
		}
	})

	t.Run("provider failure", func(t *testing.T) {
		session := createRunSession(t, runtime.baseURL, namespace, "provider-failure")
		ack, events := rpcRunStream[*protocol.StartRunResponse](
			t, runtime.baseURL, "runs.start", protocol.StartRunRequest{
				SessionID: session.ID,
				Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "SCENARIO_PROVIDER_FAILURE"}},
				Provider:  "openai-compatible", Model: "test-model",
			}, meta, "provider-failure-run", namespace,
		)
		if !streamFailedWith(events, ack.RunID, protocol.ProblemProviderUnavailable) {
			t.Fatalf("provider failure stream = %+v", events)
		}
		run := rpcCallWithMeta[*protocol.RunRef](
			t, runtime.baseURL, "runs.get", protocol.GetRunRequest{RunID: ack.RunID}, meta,
		)
		if run.Status != protocol.RunStatusFinished || run.Outcome == nil ||
			run.Outcome.Type != protocol.OutcomeFailed || run.Outcome.Error == nil ||
			run.Outcome.Error.Type != protocol.ProblemProviderUnavailable {
			t.Fatalf("provider failure Run status = %q, outcome = %+v, error = %+v", run.Status, run.Outcome, run.Outcome.Error)
		}
		exported := rpcCall[*protocol.ExportSessionResponse](
			t, runtime.baseURL, "sessions.export", protocol.ExportSessionRequest{
				SessionID: session.ID, Format: protocol.ExportFormatJSON,
			}, "", "",
		)
		if exported.Artifact == nil || len(exported.Artifact.Runs) != 1 ||
			exported.Artifact.Runs[0].Outcome.Error == nil ||
			exported.Artifact.Runs[0].Outcome.Error.Type != protocol.ArtifactProblemProviderUnavailable {
			t.Fatalf("provider failure artifact = %+v", exported.Artifact)
		}
		rpcCall[struct{}](
			t, runtime.baseURL, "sessions.delete", protocol.DeleteSessionRequest{SessionID: session.ID},
			"provider-failure-delete", namespace,
		)
		rpcCall[*protocol.ImportSessionResponse](
			t, runtime.baseURL, "sessions.import", protocol.ImportSessionRequest{Artifact: *exported.Artifact},
			"provider-failure-import", namespace,
		)
		restored := rpcCallWithMeta[*protocol.RunRef](
			t, runtime.baseURL, "runs.get", protocol.GetRunRequest{RunID: ack.RunID}, meta,
		)
		if restored.Outcome == nil || restored.Outcome.Error == nil ||
			restored.Outcome.Error.Type != protocol.ProblemProviderUnavailable {
			t.Fatalf("restored provider failure Run = %+v", restored.Outcome)
		}
	})

	t.Run("MCP failure", func(t *testing.T) {
		result := rpcCallWithMeta[*protocol.MCPTestResult](
			t, runtime.baseURL, "mcp.servers.test", protocol.MCPServerCandidate{
				Name: "unreachable", Enabled: true, TimeoutSeconds: 1,
				Connection: protocol.MCPConnectionInput{
					Type: protocol.MCPTransportStreamableHTTP, URL: "http://127.0.0.1:1",
				},
			}, meta,
		)
		if result.OK || result.Error == nil || result.Error.Type != protocol.ProblemMCPDialFailed {
			t.Fatalf("MCP failure result = %+v", result)
		}
	})

	if model.calls.Load() == 0 {
		t.Fatal("model server received no calls")
	}
}

func TestPublicSessionLifecycleRoundTrips(t *testing.T) {
	harness := newAcceptanceHarness(t)
	runtime, namespace, meta := harness.runtime, harness.namespace, harness.meta
	session := createRunSession(t, runtime.baseURL, namespace, "lifecycle")

	first, firstEvents := rpcRunStream[*protocol.StartRunResponse](
		t, runtime.baseURL, "runs.start", protocol.StartRunRequest{
			SessionID: session.ID,
			Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "first message"}},
			Provider:  "openai-compatible", Model: "test-model",
		}, meta, "lifecycle-run-1", namespace,
	)
	assertCompletedRunStream(t, first.RunID, firstEvents)
	second, secondEvents := rpcRunStream[*protocol.StartRunResponse](
		t, runtime.baseURL, "runs.start", protocol.StartRunRequest{
			SessionID: session.ID,
			Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "second message"}},
			Provider:  "openai-compatible", Model: "test-model",
		}, meta, "lifecycle-run-2", namespace,
	)
	assertCompletedRunStream(t, second.RunID, secondEvents)

	title, favorite := "renamed lifecycle", true
	updated := rpcCall[*protocol.Session](
		t, runtime.baseURL, "sessions.update", protocol.UpdateSessionRequest{
			SessionID: session.ID, ExpectedRevision: session.Revision,
			Title: &title, Favorite: &favorite,
		}, "lifecycle-update", namespace,
	)
	if updated.Title != title || !updated.Favorite || updated.Revision <= session.Revision {
		t.Fatalf("updated session = %+v", updated)
	}
	got := rpcCall[*protocol.Session](
		t, runtime.baseURL, "sessions.get", protocol.GetSessionRequest{SessionID: session.ID}, "", "",
	)
	if got.ID != session.ID || got.Revision != updated.Revision {
		t.Fatalf("sessions.get = %+v, want revision %d", got, updated.Revision)
	}
	page := rpcCall[*protocol.Page[protocol.Session]](
		t, runtime.baseURL, "sessions.list", protocol.PageQuery{Limit: 100}, "", "",
	)
	if len(page.Data) != 1 || page.Data[0].ID != session.ID {
		t.Fatalf("sessions.list = %+v", page.Data)
	}
	snapshot := rpcCallWithMeta[*protocol.SessionSnapshot](
		t, runtime.baseURL, "sessions.snapshot", protocol.GetSessionSnapshotRequest{
			SessionID: session.ID, IncludeDescendants: true,
		}, meta,
	)
	if len(snapshot.Runs) != 2 || len(snapshot.Items) < 4 {
		t.Fatalf("sessions.snapshot = %d Runs, %d Items", len(snapshot.Runs), len(snapshot.Items))
	}

	forked := rpcCall[*protocol.Session](
		t, runtime.baseURL, "sessions.fork", protocol.ForkSessionRequest{
			SessionID: session.ID, FromRunID: first.RunID, Title: "forked lifecycle",
		}, "lifecycle-fork", namespace,
	)
	forkRuns := rpcCallWithMeta[*protocol.Page[protocol.RunRef]](
		t, runtime.baseURL, "runs.list", protocol.ListRunsRequest{
			SessionID: forked.ID, IncludeDescendants: true, PageQuery: protocol.PageQuery{Limit: 100},
		}, meta,
	)
	if len(forkRuns.Data) != 1 {
		t.Fatalf("forked Runs = %+v", forkRuns.Data)
	}

	rolledBack := rpcCall[*protocol.RollbackSessionResponse](
		t, runtime.baseURL, "sessions.rollback", protocol.RollbackSessionRequest{
			SessionID: session.ID, ToRunID: first.RunID, RestoreType: protocol.RestoreHistory,
		}, "lifecycle-rollback", namespace,
	)
	if len(rolledBack.DroppedRuns) != 1 || rolledBack.DroppedRuns[0].Run.ID != second.RunID {
		t.Fatalf("rollback dropped Runs = %+v", rolledBack.DroppedRuns)
	}

	exported := rpcCall[*protocol.ExportSessionResponse](
		t, runtime.baseURL, "sessions.export", protocol.ExportSessionRequest{
			SessionID: session.ID, Format: protocol.ExportFormatJSON,
		}, "", "",
	)
	if exported.Artifact == nil || exported.Artifact.Version != protocol.SessionArtifactVersion || len(exported.Artifact.Runs) != 1 {
		t.Fatalf("JSON export = %+v", exported)
	}
	markdown := rpcCall[*protocol.ExportSessionResponse](
		t, runtime.baseURL, "sessions.export", protocol.ExportSessionRequest{
			SessionID: session.ID, Format: protocol.ExportFormatMarkdown,
		}, "", "",
	)
	if markdown.Markdown == "" || markdown.Artifact != nil {
		t.Fatalf("Markdown export = %+v", markdown)
	}

	rpcCall[struct{}](
		t, runtime.baseURL, "sessions.delete", protocol.DeleteSessionRequest{SessionID: session.ID},
		"lifecycle-delete", namespace,
	)
	imported := rpcCall[*protocol.ImportSessionResponse](
		t, runtime.baseURL, "sessions.import", protocol.ImportSessionRequest{Artifact: *exported.Artifact},
		"lifecycle-import", namespace,
	)
	if imported.Session.ID != session.ID {
		t.Fatalf("imported Session = %+v, want %q", imported.Session, session.ID)
	}
	importedRuns := rpcCallWithMeta[*protocol.Page[protocol.RunRef]](
		t, runtime.baseURL, "runs.list", protocol.ListRunsRequest{
			SessionID: session.ID, IncludeDescendants: true, PageQuery: protocol.PageQuery{Limit: 100},
		}, meta,
	)
	if len(importedRuns.Data) != 1 || importedRuns.Data[0].ID != first.RunID {
		t.Fatalf("imported Runs = %+v", importedRuns.Data)
	}
}

func TestPublicRuntimeSubscriptionResyncsAcrossTransportReconnect(t *testing.T) {
	harness := newAcceptanceHarness(t)
	var created *protocol.Session
	events := rpcRuntimeEvents(
		t, harness.runtime.baseURL,
		protocol.RuntimeSubscribeRequest{Topics: []protocol.RuntimeTopic{protocol.TopicSessionsChanged}},
		harness.meta, 2,
		func() {
			created = createRunSession(t, harness.runtime.baseURL, harness.namespace, "subscription")
		},
	)
	if events[0].Type != protocol.RuntimeResync || events[0].Sequence != 1 ||
		len(events[0].Topics) != 1 || events[0].Topics[0] != protocol.TopicSessionsChanged {
		t.Fatalf("cold subscription event = %+v", events[0])
	}
	if events[1].Type != protocol.RuntimeSessionsChanged || events[1].Sequence != 2 ||
		len(events[1].SessionIDs) != 1 || events[1].SessionIDs[0] != created.ID {
		t.Fatalf("session invalidation = %+v, session = %q", events[1], created.ID)
	}

	reconnected := rpcRuntimeEvents(
		t, harness.runtime.baseURL,
		protocol.RuntimeSubscribeRequest{Topics: []protocol.RuntimeTopic{protocol.TopicSessionsChanged}},
		harness.meta, 1, nil,
	)
	if reconnected[0].Type != protocol.RuntimeResync || reconnected[0].Sequence != 1 {
		t.Fatalf("reconnected cold event = %+v", reconnected[0])
	}
	got := rpcCall[*protocol.Session](
		t, harness.runtime.baseURL, "sessions.get", protocol.GetSessionRequest{SessionID: created.ID}, "", "",
	)
	if got.ID != created.ID {
		t.Fatalf("session after transport reconnect = %+v", got)
	}
}

func TestPublicWorkspaceGitAndDirectToolSurfaces(t *testing.T) {
	harness := newAcceptanceHarness(t)
	workspace := harness.workspace
	runFixtureCommand(t, workspace, "git", "init", "--initial-branch=main")
	runFixtureCommand(t, workspace, "git", "config", "user.email", "runtime-acceptance@example.test")
	runFixtureCommand(t, workspace, "git", "config", "user.name", "Runtime Acceptance")
	writeFixtureFile(t, filepath.Join(workspace, "tracked.txt"), "baseline\n")
	runFixtureCommand(t, workspace, "git", "add", "tracked.txt")
	runFixtureCommand(t, workspace, "git", "commit", "-m", "baseline")
	var changed strings.Builder
	for line := 1; line <= 80; line++ {
		fmt.Fprintf(&changed, "line %03d needle\n", line)
	}
	writeFixtureFile(t, filepath.Join(workspace, "tracked.txt"), changed.String())
	writeFixtureFile(t, filepath.Join(workspace, "untracked.txt"), "untracked needle\n")

	resolved := rpcCall[*protocol.WorkspaceInfo](
		t, harness.runtime.baseURL, "workspaces.resolve", protocol.ResolveWorkspaceRequest{}, "", "",
	)
	if resolved.Ref.Path != workspace || resolved.Availability != protocol.WorkspaceAvailable || resolved.ProjectRoot != workspace {
		t.Fatalf("resolved workspace = %+v", resolved)
	}
	missingPath := filepath.Join(filepath.Dir(workspace), "missing-workspace")
	missing := rpcCall[*protocol.WorkspaceInfo](
		t, harness.runtime.baseURL, "workspaces.resolve", protocol.ResolveWorkspaceRequest{
			Ref: &protocol.WorkspaceRef{Path: missingPath},
		}, "", "",
	)
	if missing.Ref.Path != missingPath || missing.Availability != protocol.WorkspaceMissing {
		t.Fatalf("missing workspace = %+v", missing)
	}
	assertRPCProblem(
		t, harness.runtime.baseURL, "workspace.files.read", protocol.ReadFileRequest{
			Workspace: protocol.WorkspaceRef{Path: missingPath}, Path: "tracked.txt",
		}, "", "", protocol.ErrWorkspaceUnavailable.Error(),
	)

	firstPage := rpcCall[*protocol.Page[protocol.FileEntry]](
		t, harness.runtime.baseURL, "workspace.files.list", protocol.ListFilesRequest{
			Workspace: protocol.WorkspaceRef{Path: workspace}, Recursive: true,
			PageQuery: protocol.PageQuery{Limit: 1},
		}, "", "",
	)
	if len(firstPage.Data) != 1 || firstPage.NextCursor == "" {
		t.Fatalf("first file page = %+v", firstPage)
	}
	secondPage := rpcCall[*protocol.Page[protocol.FileEntry]](
		t, harness.runtime.baseURL, "workspace.files.list", protocol.ListFilesRequest{
			Workspace: protocol.WorkspaceRef{Path: workspace}, Recursive: true,
			PageQuery: protocol.PageQuery{Limit: 10, Cursor: firstPage.NextCursor},
		}, "", "",
	)
	if len(secondPage.Data) == 0 {
		t.Fatalf("second file page = %+v", secondPage)
	}
	content := rpcCall[*protocol.FileContent](
		t, harness.runtime.baseURL, "workspace.files.read", protocol.ReadFileRequest{
			Workspace: protocol.WorkspaceRef{Path: workspace}, Path: "tracked.txt",
			StartLine: 10, EndLine: 12,
		}, "", "",
	)
	if content.StartLine != 10 || content.EndLine != 12 || content.TotalLines != 80 ||
		!strings.Contains(content.Content, "line 010 needle") {
		t.Fatalf("windowed file content = %+v", content)
	}
	truncated := rpcCall[*protocol.FileContent](
		t, harness.runtime.baseURL, "workspace.files.read", protocol.ReadFileRequest{
			Workspace: protocol.WorkspaceRef{Path: workspace}, Path: "tracked.txt", MaxBytes: 32,
		}, "", "",
	)
	if !truncated.Truncated || len(truncated.Content) > 32 {
		t.Fatalf("bounded file content = %+v", truncated)
	}
	head := rpcCall[*protocol.FileHead](
		t, harness.runtime.baseURL, "workspace.files.head", protocol.GetFileHeadRequest{
			Workspace: protocol.WorkspaceRef{Path: workspace}, Path: "tracked.txt", Lines: 3,
		}, "", "",
	)
	if len(head.Lines) != 3 || head.Lines[0].LineNumber != 1 {
		t.Fatalf("file head = %+v", head)
	}
	grep := rpcCall[*protocol.GrepResult](
		t, harness.runtime.baseURL, "workspace.files.search", protocol.GrepRequest{
			Workspace: protocol.WorkspaceRef{Path: workspace}, Query: "needle", Limit: 5,
		}, "", "",
	)
	if len(grep.Matches) != 5 || grep.Total < 81 {
		t.Fatalf("workspace grep = %+v", grep)
	}
	assertRPCProblem(
		t, harness.runtime.baseURL, "workspace.files.read", protocol.ReadFileRequest{
			Workspace: protocol.WorkspaceRef{Path: workspace}, Path: "../outside.txt",
		}, "", "", protocol.ErrPathOutsideRoot.Error(),
	)

	changes := rpcCallWithMeta[*protocol.Page[protocol.WorkspaceFileChange]](
		t, harness.runtime.baseURL, "workspace.changes.list",
		protocol.WorkspaceQuery{Workspace: protocol.WorkspaceRef{Path: workspace}}, harness.meta,
	)
	if len(changes.Data) != 2 {
		t.Fatalf("workspace changes = %+v", changes.Data)
	}
	rows := rpcCallWithMeta[*protocol.Diff](
		t, harness.runtime.baseURL, "workspace.diff.get", protocol.GetDiffRequest{
			Workspace: protocol.WorkspaceRef{Path: workspace}, Format: protocol.DiffFormatRows, Limit: 5,
		}, harness.meta,
	)
	if !rows.Truncated {
		t.Fatalf("bounded row diff = %+v", rows)
	}
	raw := rpcCallWithMeta[*protocol.Diff](
		t, harness.runtime.baseURL, "workspace.diff.get", protocol.GetDiffRequest{
			Workspace: protocol.WorkspaceRef{Path: workspace}, Format: protocol.DiffFormatRaw,
		}, harness.meta,
	)
	if raw.Patch == "" || raw.Files != nil || !strings.Contains(raw.Patch, "untracked.txt") {
		t.Fatalf("raw workspace diff = %+v", raw)
	}

	tools := rpcCall[*protocol.Page[protocol.ToolSpec]](
		t, harness.runtime.baseURL, "tools.list", struct{}{}, "", "",
	)
	if len(tools.Data) != 3 || tools.Data[0].Name == "" {
		t.Fatalf("direct tool catalog = %+v", tools.Data)
	}
	readResult := rpcCall[map[string]any](
		t, harness.runtime.baseURL, "tools.invoke", protocol.InvokeToolRequest{
			Name: "read", Workspace: &protocol.WorkspaceRef{Path: workspace},
			Arguments: map[string]any{"path": "tracked.txt", "start_line": 1, "max_lines": 2},
		}, "direct-read", harness.namespace,
	)
	totalLines, totalOK := readResult["total_lines"].(float64)
	contentText, contentOK := readResult["content"].(string)
	if !contentOK || contentText == "" || !totalOK || totalLines < 80 {
		t.Fatalf("direct read result = %+v", readResult)
	}
}

func TestPublicSettingsHooksSchedulesAndUsageSurfaces(t *testing.T) {
	harness := newAcceptanceHarness(t)
	baseURL, namespace, meta := harness.runtime.baseURL, harness.namespace, harness.meta
	session := createRunSession(t, baseURL, namespace, "settings-surfaces")

	providers := rpcCallWithMeta[*protocol.Page[protocol.Provider]](
		t, baseURL, "providers.list", struct{}{}, meta,
	)
	configuredProvider := false
	for _, provider := range providers.Data {
		if provider.ID == "openai-compatible" && provider.BaseURL == harness.model.URL &&
			provider.APIKeyMasked != "" && provider.APIKeyMasked != "test-key" {
			configuredProvider = true
		}
	}
	if !configuredProvider {
		t.Fatalf("providers.list omitted masked configured provider: %+v", providers.Data)
	}
	models := rpcCallWithMeta[*protocol.Page[protocol.Model]](
		t, baseURL, "models.list", protocol.ListModelsRequest{Provider: "openai-compatible"}, meta,
	)
	if len(models.Data) != 1 || models.Data[0].ID != "test-model" ||
		models.Data[0].Provider != "openai-compatible" {
		t.Fatalf("models.list = %+v", models.Data)
	}
	role := rpcCall[*protocol.UtilityRole](
		t, baseURL, "models.setUtilityRole",
		protocol.UtilityRole{Provider: "openai-compatible", Model: "test-model"},
		"set-utility-role", namespace,
	)
	if role.Provider != "openai-compatible" || role.Model != "test-model" {
		t.Fatalf("models.setUtilityRole = %+v", role)
	}
	storedRole := rpcCallWithMeta[*protocol.UtilityRole](
		t, baseURL, "models.getUtilityRole", struct{}{}, meta,
	)
	if *storedRole != *role {
		t.Fatalf("models.getUtilityRole = %+v, want %+v", storedRole, role)
	}

	mode := rpcCallWithMeta[*protocol.ApprovalModeResult](
		t, baseURL, "approval.getMode", struct{}{}, meta,
	)
	if mode.Mode != protocol.ApprovalModeBalanced {
		t.Fatalf("default approval mode = %q", mode.Mode)
	}
	mode = rpcCall[*protocol.ApprovalModeResult](
		t, baseURL, "approval.setMode", protocol.SetApprovalModeRequest{Mode: protocol.ApprovalModeSafe},
		"set-approval-mode", namespace,
	)
	if mode.Mode != protocol.ApprovalModeSafe {
		t.Fatalf("approval.setMode = %+v", mode)
	}
	rules := rpcCallWithMeta[*protocol.ListApprovalRulesResult](
		t, baseURL, "approval.listRules", protocol.ListApprovalRulesRequest{SessionID: session.ID}, meta,
	)
	if rules.Rules == nil || len(rules.Rules) != 0 {
		t.Fatalf("approval.listRules = %+v", rules.Rules)
	}

	hooksDirectory := filepath.Join(harness.workspace, ".lyra")
	if err := os.MkdirAll(hooksDirectory, 0o700); err != nil {
		t.Fatalf("create hooks directory error = %v", err)
	}
	hooksPath := filepath.Join(hooksDirectory, "hooks.json")
	writeFixtureFile(t, hooksPath, `{"hooks":[{"event":"SessionStart","inject":"acceptance context"}]}`)
	hooks := rpcCallWithMeta[*protocol.HooksListResult](
		t, baseURL, "hooks.list", protocol.ListHooksRequest{
			Workspace: protocol.WorkspaceRef{Path: harness.workspace},
		}, meta,
	)
	if hooks.ProjectRoot != harness.workspace || hooks.ProjectTrusted || len(hooks.Hooks) != 1 ||
		hooks.Hooks[0].Scope != protocol.HookScopeProject || hooks.Hooks[0].Source != hooksPath ||
		hooks.Hooks[0].Active {
		t.Fatalf("untrusted hooks.list = %+v", hooks)
	}
	rpcCall[struct{}](
		t, baseURL, "hooks.setTrust", protocol.SetHookTrustRequest{
			ProjectRoot: hooks.ProjectRoot, Trusted: true,
		}, "trust-project-hooks", namespace,
	)
	hooks = rpcCallWithMeta[*protocol.HooksListResult](
		t, baseURL, "hooks.list", protocol.ListHooksRequest{
			Workspace: protocol.WorkspaceRef{Path: harness.workspace},
		}, meta,
	)
	if !hooks.ProjectTrusted || len(hooks.Hooks) != 1 || !hooks.Hooks[0].Active {
		t.Fatalf("trusted hooks.list = %+v", hooks)
	}

	schedule := rpcCall[*protocol.Schedule](
		t, baseURL, "schedules.create", protocol.CreateScheduleRequest{
			Title: "Acceptance schedule", Instructions: "SCENARIO_NORMAL",
			Workspace: &protocol.WorkspaceRef{Path: harness.workspace},
			Provider:  "openai-compatible", Model: "test-model", Cron: "0 0 * * *",
		}, "create-schedule", namespace,
	)
	if schedule.ID == "" || !schedule.Enabled || schedule.Revision == 0 || schedule.NextRunAt == nil {
		t.Fatalf("schedules.create = %+v", schedule)
	}
	updatedTitle := "Updated acceptance schedule"
	updated := rpcCall[*protocol.Schedule](
		t, baseURL, "schedules.update", protocol.UpdateScheduleRequest{
			ID: schedule.ID, ExpectedRevision: schedule.Revision, Title: &updatedTitle,
		}, "update-schedule", namespace,
	)
	if updated.Title != updatedTitle || updated.Revision <= schedule.Revision {
		t.Fatalf("schedules.update = %+v", updated)
	}
	schedules := rpcCallWithMeta[*protocol.Page[protocol.Schedule]](
		t, baseURL, "schedules.list", protocol.PageQuery{Limit: 100}, meta,
	)
	if len(schedules.Data) != 1 || schedules.Data[0].ID != schedule.ID ||
		schedules.Data[0].Revision != updated.Revision {
		t.Fatalf("schedules.list = %+v", schedules.Data)
	}
	started := rpcCall[*protocol.RunScheduleNowResponse](
		t, baseURL, "schedules.runNow", protocol.RunScheduleNowRequest{ID: schedule.ID},
		"run-schedule-now", namespace,
	)
	waitForFinishedRun(t, baseURL, started.RunID, meta)
	usage := rpcCallWithMeta[*protocol.Usage](
		t, baseURL, "usage.session", protocol.SessionUsageRequest{SessionID: started.SessionID}, meta,
	)
	if usage.InputTokens == 0 || usage.OutputTokens == 0 || len(usage.ByModel) != 1 {
		t.Fatalf("usage.session = %+v", usage)
	}
	summary := rpcCallWithMeta[*protocol.UsageSummary](
		t, baseURL, "usage.summary", protocol.UsageSummaryRequest{SinceDays: 1}, meta,
	)
	if summary.Runs == 0 || summary.Sessions == 0 || summary.Total.InputTokens == 0 ||
		len(summary.ByProvider) == 0 || len(summary.ByModel) == 0 || len(summary.ByDay) == 0 {
		t.Fatalf("usage.summary = %+v", summary)
	}
	rpcCall[struct{}](
		t, baseURL, "schedules.delete", protocol.DeleteScheduleRequest{ID: schedule.ID},
		"delete-schedule", namespace,
	)
	schedules = rpcCallWithMeta[*protocol.Page[protocol.Schedule]](
		t, baseURL, "schedules.list", protocol.PageQuery{Limit: 100}, meta,
	)
	if len(schedules.Data) != 0 {
		t.Fatalf("schedules.list after delete = %+v", schedules.Data)
	}
}

func TestPublicLongHistoryIsBoundedAndCursorComplete(t *testing.T) {
	harness := newAcceptanceHarness(t)
	session := createRunSession(t, harness.runtime.baseURL, harness.namespace, "long-history")
	const runCount = 101
	for index := 0; index < runCount; index++ {
		ack, events := rpcRunStream[*protocol.StartRunResponse](
			t, harness.runtime.baseURL, "runs.start", protocol.StartRunRequest{
				SessionID: session.ID,
				Input: []protocol.ContentBlock{{
					Type: protocol.ContentBlockText, Text: fmt.Sprintf("history message %03d", index),
				}},
				Provider: "openai-compatible", Model: "test-model",
			}, harness.meta, fmt.Sprintf("long-history-run-%03d", index), harness.namespace,
		)
		assertCompletedRunStream(t, ack.RunID, events)
	}
	snapshot := rpcCallWithMeta[*protocol.SessionSnapshot](
		t, harness.runtime.baseURL, "sessions.snapshot", protocol.GetSessionSnapshotRequest{
			SessionID: session.ID, IncludeDescendants: true,
		}, harness.meta,
	)
	if len(snapshot.Items) != 200 {
		t.Fatalf("bounded snapshot Items = %d, want 200", len(snapshot.Items))
	}
	exported := rpcCall[*protocol.ExportSessionResponse](
		t, harness.runtime.baseURL, "sessions.export", protocol.ExportSessionRequest{
			SessionID: session.ID, Format: protocol.ExportFormatJSON,
		}, "", "",
	)
	if exported.Artifact == nil {
		t.Fatal("long-history export omitted its artifact")
	}
	if len(exported.Artifact.Items) < runCount*2 {
		t.Fatalf("long-history artifact has %d Items", len(exported.Artifact.Items))
	}
	seen := make(map[string]bool, runCount*2)
	cursor := ""
	pageCount := 0
	for {
		page := rpcCallWithMeta[*protocol.ListItemsResponse](
			t, harness.runtime.baseURL, "items.list", protocol.ListItemsRequest{
				Scope:     protocol.ItemListScope{Type: protocol.ItemScopeSession, SessionID: session.ID},
				Order:     protocol.ItemOrderDesc,
				PageQuery: protocol.PageQuery{Limit: 100, Cursor: cursor},
			}, harness.meta,
		)
		pageCount++
		if len(page.Data) == 0 {
			t.Fatalf("history page %d was empty before cursor exhaustion", pageCount)
		}
		for _, item := range page.Data {
			if seen[item.ID] {
				t.Fatalf("history cursor repeated Item %q", item.ID)
			}
			seen[item.ID] = true
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	wantItems := len(exported.Artifact.Items)
	wantPages := (wantItems + 99) / 100
	if pageCount != wantPages || len(seen) != wantItems {
		t.Fatalf("history pagination = %d pages / %d Items, want %d / %d", pageCount, len(seen), wantPages, wantItems)
	}
}

func TestPublicGoalDriverRunsAndSettlesBudget(t *testing.T) {
	harness := newAcceptanceHarness(t)
	session := createRunSession(t, harness.runtime.baseURL, harness.namespace, "goal-driver")
	started := rpcCall[*protocol.Goal](
		t, harness.runtime.baseURL, "goals.start", protocol.StartGoalRequest{
			SessionID: session.ID, Objective: "Complete one bounded autonomous iteration",
			Provider: "openai-compatible", Model: "test-model",
			Budget: protocol.GoalBudget{MaxRuns: 1},
		}, "goal-start", harness.namespace,
	)
	if started.Status != protocol.GoalActive || started.Objective == "" {
		t.Fatalf("started Goal = %+v", started)
	}
	var settled *protocol.Goal
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		settled = rpcCall[*protocol.Goal](
			t, harness.runtime.baseURL, "goals.get", protocol.GoalRequest{SessionID: session.ID}, "", "",
		)
		if settled != nil && settled.Status == protocol.GoalBlocked {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if settled == nil || settled.Status != protocol.GoalBlocked || settled.Reason == nil ||
		settled.Reason.Code != protocol.GoalReasonRunBudgetReached || settled.Used.Runs != 1 {
		t.Fatalf("settled Goal = %+v", settled)
	}
	runs := rpcCallWithMeta[*protocol.Page[protocol.RunRef]](
		t, harness.runtime.baseURL, "runs.list", protocol.ListRunsRequest{
			SessionID: session.ID, IncludeDescendants: true, PageQuery: protocol.PageQuery{Limit: 100},
		}, harness.meta,
	)
	if len(runs.Data) != 1 || runs.Data[0].Status != protocol.RunStatusFinished ||
		runs.Data[0].Outcome == nil || runs.Data[0].Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("Goal-owned Runs = %+v", runs.Data)
	}
	rpcCall[struct{}](
		t, harness.runtime.baseURL, "goals.clear", protocol.GoalRequest{SessionID: session.ID},
		"goal-clear", harness.namespace,
	)
	if current := rpcCall[*protocol.Goal](
		t, harness.runtime.baseURL, "goals.get", protocol.GoalRequest{SessionID: session.ID}, "", "",
	); current != nil {
		t.Fatalf("cleared Goal = %+v", current)
	}
}

type acceptanceHarness struct {
	runtime   *runningRuntime
	model     *scenarioModel
	workspace string
	home      string
	namespace string
	meta      protocol.RequestMeta
}

func newAcceptanceHarness(t *testing.T) acceptanceHarness {
	t.Helper()
	model := newScenarioModel(t)
	t.Cleanup(model.Close)
	workspace := privateDirectory(t, "workspace")
	workspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatalf("resolve acceptance workspace error = %v", err)
	}
	home := privateDirectory(t, "home")
	runtime := startRuntime(t, runtimehost.Config{
		Listen: "127.0.0.1:0", DatabasePath: filepath.Join(privateDirectory(t, "data"), "runtime.sqlite"),
		DefaultWorkspace: workspace, UserHome: home,
		ServerName: "lyra-runtime", ServerVersion: "acceptance",
	})
	t.Cleanup(func() { runtime.stop(t) })
	discovered := rpcCall[protocol.DiscoverResponse](
		t, runtime.baseURL, "runtime.discover", struct{}{}, "", "",
	)
	namespace := discovered.Capabilities.Limits.Idempotency.Namespace
	set := func(value string) *protocol.ProviderConfigChange {
		return &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: &value}
	}
	configured := rpcCall[*protocol.Provider](
		t, runtime.baseURL, "providers.update", protocol.UpdateProviderRequest{
			Provider: "openai-compatible", BaseURL: set(model.URL), APIKey: set("test-key"),
		}, "configure-provider", namespace,
	)
	if configured.APIKeyMasked == "" || configured.BaseURL != model.URL {
		t.Fatalf("configured provider = %+v", configured)
	}
	probe := rpcCall[*protocol.ProviderTestResult](
		t, runtime.baseURL, "providers.test",
		protocol.TestProviderRequest{Provider: "openai-compatible"}, "", "",
	)
	if !probe.OK || probe.Error != nil {
		t.Fatalf("provider probe = %+v", probe)
	}
	return acceptanceHarness{
		runtime: runtime, model: model, workspace: workspace, home: home, namespace: namespace,
		meta: protocol.RequestMeta{
			ProtocolVersion: protocol.ProtocolVersion,
			ClientInfo:      &protocol.ClientInfo{Name: "runtime-acceptance", Version: "1"},
			ClientCapabilities: &protocol.ClientCapabilities{
				Features: map[string]protocol.FeaturePreference{
					protocol.FeatureGoals:       {Enabled: true},
					protocol.FeaturePlan:        {Enabled: true},
					protocol.FeatureSubagents:   {Enabled: true},
					protocol.FeatureMCP:         {Enabled: true},
					protocol.FeatureGit:         {Enabled: true},
					protocol.FeatureSchedules:   {Enabled: true},
					protocol.FeatureSkills:      {Enabled: true},
					protocol.FeatureKnowledge:   {Enabled: true},
					protocol.FeatureAgentMemory: {Enabled: true},
					protocol.FeatureCodebase:    {Enabled: true},
				},
				InterruptTypes: []protocol.InterruptType{
					protocol.InterruptApproval, protocol.InterruptQuestion,
				},
			},
		},
	}
}

func writeFixtureFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write fixture %q error = %v", path, err)
	}
}

func runFixtureCommand(t *testing.T, directory string, name string, arguments ...string) {
	t.Helper()
	command := exec.Command(name, arguments...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("run %s %v error = %v\n%s", name, arguments, err, output)
	}
}

func createRunSession(t *testing.T, baseURL string, namespace string, title string) *protocol.Session {
	t.Helper()
	return rpcCall[*protocol.Session](
		t, baseURL, "sessions.create", protocol.CreateSessionRequest{
			Title: title, Provider: "openai-compatible", Model: "test-model",
		}, "session-"+title, namespace,
	)
}

func rpcCallWithMeta[Result any](
	t *testing.T,
	baseURL string,
	method string,
	params any,
	meta protocol.RequestMeta,
) Result {
	t.Helper()
	response := rpcRequestWithMeta(t, baseURL, method, params, &meta, "", "")
	if response.Error != nil {
		t.Fatalf("%s problem = %+v", method, response.Error.Data)
	}
	var value Result
	if err := json.Unmarshal(response.Result, &value); err != nil {
		t.Fatalf("decode %s result error = %v", method, err)
	}
	return value
}

func rpcRunStream[Ack any](
	t *testing.T,
	baseURL string,
	method string,
	params any,
	meta protocol.RequestMeta,
	idempotencyKey string,
	idempotencyNamespace string,
) (Ack, []protocol.RunEvent) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": "stream-acceptance", "method": method,
		"params": rpcParameters(t, params, &meta),
	})
	if err != nil {
		t.Fatalf("encode %s request error = %v", method, err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, baseURL+httptransport.PathRPC, bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("build %s request error = %v", method, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", idempotencyKey)
	request.Header.Set("Idempotency-Namespace", idempotencyNamespace)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("call %s stream error = %v", method, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("call %s stream status = %d", method, response.StatusCode)
	}
	reader, err := sse.NewHTTPReader(response)
	if err != nil {
		t.Fatalf("open %s SSE error = %v", method, err)
	}
	var (
		ack      Ack
		events   []protocol.RunEvent
		messageN int
	)
	for message, messageErr := range reader.Messages() {
		if messageErr != nil {
			t.Fatalf("read %s SSE error = %v", method, messageErr)
		}
		messageN++
		if messageN == 1 {
			var envelope rpcResponse
			if err := json.Unmarshal(message.Data, &envelope); err != nil {
				t.Fatalf("decode %s ack envelope error = %v", method, err)
			}
			if envelope.Error != nil {
				t.Fatalf("%s ack problem = %+v", method, envelope.Error.Data)
			}
			if err := json.Unmarshal(envelope.Result, &ack); err != nil {
				t.Fatalf("decode %s ack error = %v", method, err)
			}
			continue
		}
		var notification struct {
			Method string            `json:"method"`
			Params protocol.RunEvent `json:"params"`
		}
		if err := json.Unmarshal(message.Data, &notification); err != nil {
			t.Fatalf("decode %s notification error = %v", method, err)
		}
		if notification.Method != "notifications.run.event" {
			t.Fatalf("%s notification method = %q", method, notification.Method)
		}
		events = append(events, notification.Params)
	}
	if messageN == 0 {
		t.Fatalf("%s stream returned no acknowledgement", method)
	}
	return ack, events
}

func rpcRuntimeEvents(
	t *testing.T,
	baseURL string,
	params protocol.RuntimeSubscribeRequest,
	meta protocol.RequestMeta,
	count int,
	afterFirst func(),
) []protocol.RuntimeEvent {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": "runtime-subscription-acceptance", "method": "runtime.subscribe",
		"params": rpcParameters(t, params, &meta),
	})
	if err != nil {
		t.Fatalf("encode runtime.subscribe request error = %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, baseURL+httptransport.PathRPC, bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("build runtime.subscribe request error = %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("call runtime.subscribe error = %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("runtime.subscribe status = %d", response.StatusCode)
	}
	reader, err := sse.NewHTTPReader(response)
	if err != nil {
		t.Fatalf("open runtime.subscribe SSE error = %v", err)
	}
	events := make([]protocol.RuntimeEvent, 0, count)
	messageN := 0
	for message, messageErr := range reader.Messages() {
		if messageErr != nil {
			t.Fatalf("read runtime.subscribe SSE error = %v", messageErr)
		}
		messageN++
		if messageN == 1 {
			var envelope rpcResponse
			if err := json.Unmarshal(message.Data, &envelope); err != nil {
				t.Fatalf("decode runtime.subscribe acknowledgement error = %v", err)
			}
			if envelope.Error != nil {
				t.Fatalf("runtime.subscribe problem = %+v", envelope.Error.Data)
			}
			continue
		}
		var notification struct {
			Method string                            `json:"method"`
			Params protocol.RuntimeEventNotification `json:"params"`
		}
		if err := json.Unmarshal(message.Data, &notification); err != nil {
			t.Fatalf("decode runtime.subscribe notification error = %v", err)
		}
		if notification.Method != "notifications.runtime.event" {
			t.Fatalf("runtime.subscribe notification method = %q", notification.Method)
		}
		events = append(events, notification.Params.Event)
		if len(events) == 1 && afterFirst != nil {
			afterFirst()
		}
		if len(events) == count {
			return events
		}
	}
	t.Fatalf("runtime.subscribe closed after %d/%d events", len(events), count)
	return nil
}

func assertCompletedRunStream(t *testing.T, runID string, events []protocol.RunEvent) {
	t.Helper()
	started, finished := false, false
	for _, event := range events {
		if event.RunID != runID {
			continue
		}
		switch event.Event.Type {
		case protocol.StreamSegmentStarted:
			started = true
		case protocol.StreamSegmentFinished:
			finished = event.Event.Outcome != nil && event.Event.Outcome.Type == protocol.SegmentCompleted
		}
	}
	if !started || !finished {
		t.Fatalf("completed stream flags = started:%v finished:%v events:%+v", started, finished, events)
	}
}

func assertInterruptedRunStream(t *testing.T, runID string, events []protocol.RunEvent) {
	t.Helper()
	if streamSettledWith(events, runID, protocol.SegmentInterrupt) {
		return
	}
	t.Fatalf("Run %q stream omitted interrupt settlement: %+v", runID, events)
}

func streamSettledWith(events []protocol.RunEvent, runID string, outcome protocol.SegmentOutcomeType) bool {
	for _, event := range events {
		if event.RunID == runID && event.Event.Type == protocol.StreamSegmentFinished &&
			event.Event.Outcome != nil && event.Event.Outcome.Type == outcome {
			return true
		}
	}
	return false
}

func streamFailedWith(events []protocol.RunEvent, runID string, problemType string) bool {
	for _, event := range events {
		if event.RunID == runID && event.Event.Type == protocol.StreamSegmentFinished &&
			event.Event.Outcome != nil && event.Event.Outcome.Type == protocol.SegmentFailed &&
			event.Event.Outcome.Error != nil && event.Event.Outcome.Error.Type == problemType {
			return true
		}
	}
	return false
}

func containsPlanUpdate(events []protocol.RunEvent, sessionID string) bool {
	for _, event := range events {
		if event.Event.Type == protocol.StreamPlanUpdated && event.Event.Plan != nil &&
			event.Event.Plan.SessionID == sessionID && event.Event.Plan.Revision > 0 {
			return true
		}
	}
	return false
}

func assertFinishedRun(t *testing.T, baseURL string, runID string, meta protocol.RequestMeta) {
	t.Helper()
	run := rpcCallWithMeta[*protocol.RunRef](
		t, baseURL, "runs.get", protocol.GetRunRequest{RunID: runID}, meta,
	)
	if run.Status != protocol.RunStatusFinished || run.Outcome == nil || run.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("Run %q state = %+v", runID, run)
	}
}

func waitForFinishedRun(t *testing.T, baseURL string, runID string, meta protocol.RequestMeta) *protocol.RunRef {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		run := rpcCallWithMeta[*protocol.RunRef](
			t, baseURL, "runs.get", protocol.GetRunRequest{RunID: runID}, meta,
		)
		if run.Status == protocol.RunStatusFinished {
			if run.Outcome == nil || run.Outcome.Type != protocol.OutcomeCompleted {
				t.Fatalf("Run %q settled as %+v", runID, run.Outcome)
			}
			return run
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("Run %q did not finish before timeout", runID)
	return nil
}

func containsFinalText(items []protocol.Item, wanted string) bool {
	for _, item := range items {
		if item.Type != protocol.ItemTypeAgentMessage || item.Phase != protocol.MessagePhaseFinalAnswer {
			continue
		}
		for _, block := range item.Content {
			if block.Type == protocol.ContentBlockText && strings.Contains(block.Text, wanted) {
				return true
			}
		}
	}
	return false
}

type scenarioModel struct {
	*httptest.Server
	calls atomic.Int64
}

func newScenarioModel(t *testing.T) *scenarioModel {
	t.Helper()
	model := &scenarioModel{}
	model.Server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("model authorization = %q", request.Header.Get("Authorization"))
		}
		switch request.URL.Path {
		case "/models":
			response.Header().Set("Content-Type", "application/json")
			fmt.Fprint(response, `{"object":"list","data":[{"id":"test-model","object":"model"}]}`)
		case "/embeddings":
			var body struct {
				Input []string `json:"input"`
				Model string   `json:"model"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil || len(body.Input) == 0 {
				t.Errorf("decode embedding request error = %v, body = %+v", err, body)
				http.Error(response, "bad request", http.StatusBadRequest)
				return
			}
			data := make([]any, len(body.Input))
			for index := range body.Input {
				data[index] = map[string]any{
					"object": "embedding", "index": index,
					"embedding": []float64{1, 0.5, 0.25},
				}
			}
			response.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(response).Encode(map[string]any{
				"object": "list", "data": data, "model": body.Model,
				"usage": map[string]any{"prompt_tokens": len(body.Input), "total_tokens": len(body.Input)},
			})
		case "/chat/completions":
			model.calls.Add(1)
			var body struct {
				Stream   bool `json:"stream"`
				Messages []struct {
					Role    string          `json:"role"`
					Content json.RawMessage `json:"content"`
				} `json:"messages"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Errorf("decode model request error = %v", err)
				http.Error(response, "bad request", http.StatusBadRequest)
				return
			}
			encoded, _ := json.Marshal(body.Messages)
			material := string(encoded)
			if !body.Stream {
				writeScenarioCompletion(response, "background complete")
				return
			}
			hasToolResult := false
			for _, message := range body.Messages {
				hasToolResult = hasToolResult || message.Role == "tool"
			}
			switch {
			case strings.Contains(material, "SCENARIO_CHILD"):
				writeScenarioTextStream(response, "child complete")
			case strings.Contains(material, "SCENARIO_QUESTION") && !hasToolResult:
				writeScenarioToolStream(response, "call-question", "ask_user", `{"fields":[{"prompt":"Pick a color","header":"Color","type":"choice","options":[{"label":"Blue"},{"label":"Green"}]}]}`)
			case strings.Contains(material, "SCENARIO_QUESTION"):
				writeScenarioTextStream(response, "question complete")
			case strings.Contains(material, "SCENARIO_APPROVAL") && !hasToolResult:
				writeScenarioToolStream(response, "call-shell", "shell", `{"command":"printf r11-approved","description":"Verify approval path"}`)
			case strings.Contains(material, "SCENARIO_APPROVAL"):
				writeScenarioTextStream(response, "approval complete")
			case strings.Contains(material, "SCENARIO_DELEGATE") && !hasToolResult:
				writeScenarioToolStream(response, "call-delegate", "delegate_task", `{"summary":"Independent child","instructions":"SCENARIO_CHILD"}`)
			case strings.Contains(material, "SCENARIO_DELEGATE"):
				writeScenarioTextStream(response, "delegation complete")
			case strings.Contains(material, "SCENARIO_PLAN") && !hasToolResult:
				writeScenarioToolStream(response, "call-plan", "set_plan", `{"steps":[{"description":"Inspect architecture","status":"in_progress"},{"description":"Implement cleanly","status":"pending"}]}`)
			case strings.Contains(material, "SCENARIO_PLAN"):
				writeScenarioTextStream(response, "plan complete")
			case strings.Contains(material, "SCENARIO_PROPOSE_SKILL") && !hasToolResult:
				writeScenarioToolStream(response, "call-propose-skill", "propose_skill", `{"name":"proposed-skill","description":"A proposed acceptance skill","instructions":"Apply the accepted workflow carefully.","scope":"project"}`)
			case strings.Contains(material, "SCENARIO_REJECT_SKILL") && !hasToolResult:
				writeScenarioToolStream(response, "call-reject-skill", "propose_skill", `{"name":"rejected-skill","description":"A rejected acceptance skill","instructions":"This proposal should remain inactive.","scope":"project"}`)
			case strings.Contains(material, "SCENARIO_PROPOSE_SKILL"), strings.Contains(material, "SCENARIO_REJECT_SKILL"):
				writeScenarioTextStream(response, "skill proposal complete")
			case strings.Contains(material, "SCENARIO_PROVIDER_FAILURE"):
				http.Error(response, "provider temporarily unavailable", http.StatusServiceUnavailable)
			default:
				writeScenarioTextStream(response, "normal complete")
			}
		default:
			http.NotFound(response, request)
		}
	}))
	return model
}

func writeScenarioTextStream(response http.ResponseWriter, content string) {
	writeScenarioStream(response, map[string]any{
		"role": "assistant", "content": content,
	}, "stop")
}

func writeScenarioToolStream(response http.ResponseWriter, id string, name string, arguments string) {
	writeScenarioStream(response, map[string]any{
		"role": "assistant",
		"tool_calls": []any{map[string]any{
			"index": 0, "id": id, "type": "function",
			"function": map[string]any{"name": name, "arguments": arguments},
		}},
	}, "tool_calls")
}

func writeScenarioStream(response http.ResponseWriter, delta map[string]any, finishReason string) {
	response.Header().Set("Content-Type", "text/event-stream")
	chunk := map[string]any{
		"id": "chatcmpl-acceptance", "object": "chat.completion.chunk",
		"created": 1787529600, "model": "test-model",
		"choices": []any{map[string]any{
			"index": 0, "delta": delta, "finish_reason": finishReason,
		}},
	}
	encoded, _ := json.Marshal(chunk)
	fmt.Fprintf(response, "data: %s\n\n", encoded)
	usage, _ := json.Marshal(map[string]any{
		"id": "chatcmpl-acceptance", "object": "chat.completion.chunk",
		"created": 1787529600, "model": "test-model", "choices": []any{},
		"usage": map[string]any{"prompt_tokens": 8, "completion_tokens": 4, "total_tokens": 12},
	})
	fmt.Fprintf(response, "data: %s\n\ndata: [DONE]\n\n", usage)
}

func writeScenarioCompletion(response http.ResponseWriter, content string) {
	response.Header().Set("Content-Type", "application/json")
	encoded, _ := json.Marshal(map[string]any{
		"id": "chatcmpl-acceptance", "object": "chat.completion", "created": 1787529600,
		"model": "test-model",
		"choices": []any{map[string]any{
			"index": 0, "finish_reason": "stop",
			"message": map[string]any{"role": "assistant", "content": content},
		}},
		"usage": map[string]any{"prompt_tokens": 8, "completion_tokens": 4, "total_tokens": 12},
	})
	_, _ = response.Write(encoded)
}
