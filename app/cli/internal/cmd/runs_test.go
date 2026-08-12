package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
	"github.com/Tangerg/lynx/app/cli/internal/backend"
	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
)

type recordingRunCatalog struct {
	agent.Runtime
	queries []agent.RunQuery
}

func (runtime *recordingRunCatalog) ListRuns(ctx context.Context, query agent.RunQuery) (agent.RunPage, error) {
	runtime.queries = append(runtime.queries, query)
	return runtime.Runtime.ListRuns(ctx, query)
}

func TestRunsListConsumesFiltersAndStableJSON(t *testing.T) {
	base := instantRuntime()
	runtime := &recordingRunCatalog{Runtime: base}
	out, errOut, err := executeCommand(t, runtime, "", "runs", "ls",
		"--session", "ses_demo_1", "--status", "finished", "--include-descendants", "--limit", "7", "--json",
	)
	if err != nil {
		t.Fatalf("runs ls: %v", err)
	}
	if errOut != "" {
		t.Fatalf("runs ls stderr = %q", errOut)
	}
	if len(runtime.queries) != 1 {
		t.Fatalf("queries = %+v", runtime.queries)
	}
	query := runtime.queries[0]
	if query.SessionID != "ses_demo_1" || len(query.Statuses) != 1 || query.Statuses[0] != agent.RunStatusFinished ||
		!query.IncludeDescendants || query.Limit != 7 {
		t.Fatalf("query = %+v", query)
	}
	var page struct {
		Items []struct {
			ID        string `json:"id"`
			SessionID string `json:"sessionId"`
			Status    string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &page); err != nil {
		t.Fatalf("runs list output: %v\n%s", err, out)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "run_demo_history" || page.Items[0].SessionID != "ses_demo_1" || page.Items[0].Status != "finished" {
		t.Fatalf("page = %+v", page)
	}
	if strings.Contains(out, `"ID"`) {
		t.Fatalf("runs list leaked Go field names:\n%s", out)
	}
}

func TestRunsListKeepsPaginationOutOfMachineOutput(t *testing.T) {
	runtime := instantRuntime()
	runtime.Script = shortCompletedScript
	stream, err := runtime.StartRun(t.Context(), agent.StartRun{
		SessionID: "ses_demo_1", Message: agent.Message{Text: "newer run"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, streamErr := range stream.Events {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
	}

	textOut, textErr, err := executeCommand(t, runtime, "", "runs", "ls", "--session", "ses_demo_1", "--limit", "1")
	if err != nil || len(strings.Split(strings.TrimSpace(textOut), "\n")) != 1 || !strings.Contains(textErr, "more runs: --cursor") {
		t.Fatalf("text page = %q, stderr %q, %v", textOut, textErr, err)
	}
	jsonOut, jsonErr, err := executeCommand(t, runtime, "", "runs", "ls", "--session", "ses_demo_1", "--limit", "1", "--json")
	if err != nil || jsonErr != "" || !strings.Contains(jsonOut, `"nextCursor"`) {
		t.Fatalf("JSON page = %q, stderr %q, %v", jsonOut, jsonErr, err)
	}
}

func TestRunsListRejectsAnInvalidStatusBeforeOpeningTheRuntime(t *testing.T) {
	var opened bool
	provider := runtimeProvider{open: func(context.Context) (backend.Services, error) {
		opened = true
		return backend.AgentOnly(instantRuntime()), nil
	}}
	command := newRunsListCommand(provider)
	command.SetOut(&strings.Builder{})
	command.SetErr(&strings.Builder{})
	command.SetArgs([]string{"--status", "paused"})
	if err := command.ExecuteContext(t.Context()); err == nil || !strings.Contains(err.Error(), "paused") {
		t.Fatalf("invalid status error = %v", err)
	}
	if opened {
		t.Fatal("invalid status opened the runtime")
	}
}

func TestRunsListRejectsDescendantsBeforeCallingAnUnnegotiatedRuntime(t *testing.T) {
	t.Parallel()
	profile := commandRuntimeProfile()
	profile.Features[runtimeprofile.FeatureSubagents] = runtimeprofile.Feature{
		Stability: runtimeprofile.Stable, ClientOptIn: true,
	}
	runtime := &recordingRunCatalog{Runtime: instantRuntime()}
	provider := runtimeProvider{open: func(context.Context) (backend.Services, error) {
		return backend.Services{Agent: runtime, RuntimeProfile: new(profile.Clone())}, nil
	}}
	command := newRunsListCommand(provider)
	command.SetOut(&strings.Builder{})
	command.SetErr(&strings.Builder{})
	command.SetArgs([]string{"--include-descendants"})
	if err := command.ExecuteContext(t.Context()); err == nil || !strings.Contains(err.Error(), "subagents") {
		t.Fatalf("runs list error = %v", err)
	}
	if len(runtime.queries) != 0 {
		t.Fatalf("unnegotiated descendant query reached runtime: %+v", runtime.queries)
	}
}

func TestRunsShowUsesDirectRunRead(t *testing.T) {
	runtime := instantRuntime()
	out, _, err := executeCommand(t, runtime, "", "runs", "show", "run_demo_history", "--json")
	if err != nil {
		t.Fatalf("runs show: %v", err)
	}
	var run struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		Outcome struct {
			Status string `json:"status"`
		} `json:"outcome"`
	}
	if err := json.Unmarshal([]byte(out), &run); err != nil {
		t.Fatalf("runs show output: %v\n%s", err, out)
	}
	if run.ID != "run_demo_history" || run.Status != "finished" || run.Outcome.Status != "completed" {
		t.Fatalf("run = %+v", run)
	}
}

func TestRunsCancelRequiresConfirmationAndReturnsRootSnapshot(t *testing.T) {
	runtime := instantRuntime()
	runtime.Instant = false
	runtime.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{
			Delay: time.Hour,
			Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}},
		}}}
	}
	opened, err := runtime.StartRun(t.Context(), agent.StartRun{
		SessionID: "ses_demo_1", Message: agent.Message{Text: "keep running"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := executeCommand(t, runtime, "", "runs", "cancel", opened.RunID); err == nil {
		t.Fatal("runs cancel did not require --yes")
	}
	stillRunning, err := runtime.GetRun(t.Context(), opened.RunID)
	if err != nil || stillRunning.Status != agent.RunStatusRunning {
		t.Fatalf("unconfirmed cancel changed run = %+v, %v", stillRunning, err)
	}

	out, _, err := executeCommand(t, runtime, "", "runs", "cancel", opened.RunID, "--yes", "--reason", "operator stopped it", "--json")
	if err != nil {
		t.Fatalf("runs cancel: %v", err)
	}
	var result struct {
		Canceled struct {
			ID      string `json:"id"`
			Outcome struct {
				Status string `json:"status"`
				Detail string `json:"detail"`
			} `json:"outcome"`
		} `json:"canceled"`
		Root struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"root"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("runs cancel output: %v\n%s", err, out)
	}
	if result.Canceled.ID != opened.RunID || result.Canceled.Outcome.Status != "canceled" ||
		result.Canceled.Outcome.Detail != "operator stopped it" || result.Root.ID != opened.RunID || result.Root.Status != "finished" {
		t.Fatalf("result = %+v", result)
	}
}

type childCancellationRuntime struct {
	agent.Runtime
	result agent.RunCancellation
}

type uncertainRunCancellationRuntime struct {
	agent.Runtime

	mu       sync.Mutex
	attempts []agent.CancelRun
}

func (runtime *uncertainRunCancellationRuntime) CancelRun(ctx context.Context, request agent.CancelRun) (agent.RunCancellation, error) {
	runtime.mu.Lock()
	runtime.attempts = append(runtime.attempts, request)
	attempt := len(runtime.attempts)
	runtime.mu.Unlock()
	if attempt == 1 {
		return agent.RunCancellation{}, fmt.Errorf("cancellation acknowledgement timed out: %w", context.DeadlineExceeded)
	}
	return runtime.Runtime.CancelRun(ctx, request)
}

func (runtime *uncertainRunCancellationRuntime) cancelAttempts() []agent.CancelRun {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return append([]agent.CancelRun(nil), runtime.attempts...)
}

func (runtime childCancellationRuntime) CancelRun(context.Context, agent.CancelRun) (agent.RunCancellation, error) {
	return runtime.result, nil
}

func TestRunsCancelPreservesSurvivingRootStateForAChild(t *testing.T) {
	child := agent.Run{
		ID: "run_child", SessionID: "ses_1",
		Lineage: agent.RunLineage{SpawnedByBlockID: "item_spawn", ParentRunID: "run_root", RootRunID: "run_root"},
		Status:  agent.RunStatusFinished, Outcome: agent.Outcome{Status: agent.OutcomeCanceled},
	}
	root := agent.Run{ID: "run_root", SessionID: "ses_1", Status: agent.RunStatusWaiting}
	runtime := childCancellationRuntime{
		Runtime: instantRuntime(),
		result:  agent.RunCancellation{Canceled: child, Root: root},
	}
	out, _, err := executeCommand(t, runtime, "", "runs", "cancel", "run_child", "--yes", "--json")
	if err != nil {
		t.Fatalf("runs cancel child: %v", err)
	}
	if !strings.Contains(out, `"id":"run_child"`) || !strings.Contains(out, `"id":"run_root"`) || !strings.Contains(out, `"status":"waiting"`) {
		t.Fatalf("child cancellation output omitted surviving root:\n%s", out)
	}
}

func TestRunsCancelConfirmsTimeoutWithOneMutationIdentity(t *testing.T) {
	base := instantRuntime()
	base.Instant = false
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{
			Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}},
		}}}
	}
	opened, err := base.StartRun(t.Context(), agent.StartRun{
		SessionID: "ses_demo_1", Message: agent.Message{Text: "cancel through subcommand"},
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := &uncertainRunCancellationRuntime{Runtime: base}
	if _, _, err := executeCommand(t, runtime, "", "runs", "cancel", opened.RunID, "--yes"); err != nil {
		t.Fatal(err)
	}
	attempts := runtime.cancelAttempts()
	if len(attempts) != 2 || attempts[0].CommandID == "" || attempts[0].CommandID != attempts[1].CommandID ||
		attempts[0].RunID != opened.RunID || attempts[1].RunID != opened.RunID {
		t.Fatalf("cancellation confirmation attempts = %+v", attempts)
	}
}

func TestRunIDCompletionIncludesDescendants(t *testing.T) {
	runtime := &recordingRunCatalog{Runtime: instantRuntime()}
	out, _, err := executeCommand(t, runtime, "", "__complete", "runs", "show", "run_demo")
	if err != nil || !strings.Contains(out, "run_demo_history") {
		t.Fatalf("completion = %q, %v", out, err)
	}
	if len(runtime.queries) != 1 || !runtime.queries[0].IncludeDescendants || runtime.queries[0].Limit != 100 {
		t.Fatalf("completion query = %+v", runtime.queries)
	}
}

func TestRunIDCompletionFallsBackToRootsWithoutSubagents(t *testing.T) {
	t.Parallel()
	profile := commandRuntimeProfile()
	profile.Features[runtimeprofile.FeatureSubagents] = runtimeprofile.Feature{
		Stability: runtimeprofile.Stable, ClientOptIn: true,
	}
	runtime := &recordingRunCatalog{Runtime: instantRuntime()}
	provider := runtimeProvider{open: func(context.Context) (backend.Services, error) {
		return backend.Services{Agent: runtime, RuntimeProfile: new(profile.Clone())}, nil
	}}
	command := newRunsShowCommand(provider)
	command.SetContext(t.Context())
	items, directive := command.ValidArgsFunction(command, nil, "run_demo")
	if directive != cobra.ShellCompDirectiveNoFileComp || len(items) == 0 {
		t.Fatalf("completion = (%v, %v)", items, directive)
	}
	if len(runtime.queries) != 1 || runtime.queries[0].IncludeDescendants || runtime.queries[0].Limit != 100 {
		t.Fatalf("completion query = %+v", runtime.queries)
	}
}
