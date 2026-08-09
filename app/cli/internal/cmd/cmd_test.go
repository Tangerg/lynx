package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/client/mock"
)

// exec runs the CLI in memory and returns stdout, stderr and the command error.
// Nothing here touches the process's own streams, which is what lets the tests
// run in parallel and assert on exact output.
func exec(t *testing.T, rt client.Runtime, stdin string, args ...string) (string, string, error) {
	t.Helper()
	var out, errb bytes.Buffer
	root := NewRoot(rt)
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	return out.String(), errb.String(), err
}

func instant() *mock.Runtime {
	rt := mock.New()
	rt.Instant = true
	return rt
}

func firstSession(t *testing.T, rt client.Runtime) string {
	t.Helper()
	sessions, err := rt.ListSessions(context.Background(), client.SessionQuery{})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	return sessions.Items[0].ID
}

func TestRunDeclinesApprovalWhenUnattended(t *testing.T) {
	rt := instant()
	out, _, err := exec(t, rt, "", "run", "-s", firstSession(t, rt), "why?")
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "? edit internal/store/cache_test.go") {
		t.Fatalf("output does not show the approval request:\n%s", out)
	}
	if !strings.Contains(out, "Edit declined") {
		t.Fatalf("output does not show the run continuing after the refusal:\n%s", out)
	}
	if strings.Contains(out, "● edit") {
		t.Fatalf("the edit ran despite being declined:\n%s", out)
	}
	if !strings.Contains(out, "↑ ") {
		t.Fatalf("output has no usage footer, so the run did not finish:\n%s", out)
	}
}

func TestRunApproveAllLetsTheRunThrough(t *testing.T) {
	rt := instant()
	out, _, err := exec(t, rt, "", "run", "--approve-all", "-s", firstSession(t, rt), "why?")
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "● edit · internal/store/cache_test.go") {
		t.Fatalf("the approved edit did not run:\n%s", out)
	}
	if strings.Contains(out, "Edit declined") {
		t.Fatalf("an approved run reported a refusal:\n%s", out)
	}
}

func TestRunJSONIsOneObjectPerLineEndingWithTheRun(t *testing.T) {
	rt := instant()
	out, _, err := exec(t, rt, "", "run", "--json", "--approve-all", "-s", firstSession(t, rt), "why?")
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected a stream of frames, got:\n%s", out)
	}
	var last struct {
		Type    string `json:"type"`
		Outcome struct {
			Status string `json:"status"`
		} `json:"outcome"`
	}
	for i, line := range lines {
		var probe struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &probe); err != nil {
			t.Fatalf("line %d is not JSON: %v\n%s", i, err, line)
		}
		if probe.Type == "" {
			t.Fatalf("line %d has no type: %s", i, line)
		}
	}
	if err := json.Unmarshal([]byte(lines[0]), &last); err != nil || last.Type != "run.started" {
		t.Fatalf("first frame = %s, want run.started", lines[0])
	}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &last); err != nil {
		t.Fatalf("last frame is not JSON: %v", err)
	}
	if last.Type != "run.finished" || last.Outcome.Status != "completed" {
		t.Fatalf("last frame = %s, want a completed run.finished", lines[len(lines)-1])
	}
}

func TestRunReadsAPipedPromptAndCombinesItWithTheArgument(t *testing.T) {
	var captured string
	rt := instant()
	rt.Script = func(prompt string) mock.Script {
		captured = prompt
		return mock.Script{Prelude: []mock.Step{{Event: client.RunFinished{
			Outcome: client.Outcome{Status: client.OutcomeCompleted},
		}}}}
	}
	if _, _, err := exec(t, rt, "file contents\n", "run", "-s", firstSession(t, rt), "explain this"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if captured != "explain this\n\nfile contents" {
		t.Fatalf("prompt = %q, want the argument then the piped text", captured)
	}
}

func TestRunWithNothingToSay(t *testing.T) {
	rt := instant()
	_, _, err := exec(t, rt, "", "run", "-s", firstSession(t, rt))
	if !errors.Is(err, errNoPrompt) {
		t.Fatalf("err = %v, want errNoPrompt", err)
	}
}

func TestRunRejectsAnUnknownSession(t *testing.T) {
	_, _, err := exec(t, instant(), "", "run", "-s", "ses_nope", "why?")
	if !errors.Is(err, client.ErrSessionNotFound) {
		t.Fatalf("err = %v, want ErrSessionNotFound", err)
	}
}

func TestRunCreatesASessionWhenNoneIsNamed(t *testing.T) {
	rt := instant()
	before, _ := rt.ListSessions(context.Background(), client.SessionQuery{Limit: 100})
	if _, _, err := exec(t, rt, "", "run", "--approve-all", "why?"); err != nil {
		t.Fatalf("run: %v", err)
	}
	after, _ := rt.ListSessions(context.Background(), client.SessionQuery{Limit: 100})
	if len(after.Items) != len(before.Items)+1 {
		t.Fatalf("session count went %d -> %d, want one more", len(before.Items), len(after.Items))
	}
}

func TestSessionsList(t *testing.T) {
	out, _, err := exec(t, instant(), "", "sessions", "ls")
	if err != nil {
		t.Fatalf("sessions ls: %v", err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("listed %d sessions, want the 3 seeded ones:\n%s", len(lines), out)
	}
	for _, want := range []string{"ses_demo_1", "Flaky cache expiry test", "/tmp/demo/store"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output is missing %q:\n%s", want, out)
		}
	}
}

func TestSessionsListAliasAndArity(t *testing.T) {
	if _, _, err := exec(t, instant(), "", "session", "list"); err != nil {
		t.Fatalf("aliases should reach the same command: %v", err)
	}
	if _, _, err := exec(t, instant(), "", "sessions", "ls", "extra"); err == nil {
		t.Fatal("sessions ls accepted an argument it has no use for")
	}
}

func TestSessionManagementCommands(t *testing.T) {
	runtime := instant()
	id := firstSession(t, runtime)

	shown, _, err := exec(t, runtime, "", "sessions", "show", id)
	if err != nil {
		t.Fatalf("sessions show: %v", err)
	}
	if !strings.Contains(shown, "The fixed sleep races the janitor") {
		t.Fatalf("saved transcript was not restored:\n%s", shown)
	}

	renamed, _, err := exec(t, runtime, "", "sessions", "rename", id, "Investigate cache")
	if err != nil || !strings.Contains(renamed, "Investigate cache") {
		t.Fatalf("sessions rename = %q, %v", renamed, err)
	}

	forked, _, err := exec(t, runtime, "", "sessions", "fork", id, "--at", "2", "--title", "Alternative")
	if err != nil {
		t.Fatalf("sessions fork: %v", err)
	}
	forkID := strings.TrimSpace(forked)
	snapshot, err := runtime.GetSession(t.Context(), forkID)
	if err != nil || snapshot.Session.Title != "Alternative" || len(snapshot.Events) != 2 {
		t.Fatalf("fork snapshot = %+v, %v", snapshot, err)
	}

	if _, _, err := exec(t, runtime, "", "sessions", "delete", forkID); err == nil {
		t.Fatal("sessions delete did not require confirmation")
	}
	if _, _, err := exec(t, runtime, "", "sessions", "rm", "--yes", forkID); err != nil {
		t.Fatalf("sessions rm --yes: %v", err)
	}
}

func TestSessionsListPaginatesAndSearches(t *testing.T) {
	runtime := instant()
	out, errOut, err := exec(t, runtime, "", "sessions", "ls", "--limit", "1", "--search", "store")
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.Split(strings.TrimSpace(out), "\n")) != 1 || !strings.Contains(errOut, "more sessions: --cursor") {
		t.Fatalf("page output=%q stderr=%q", out, errOut)
	}
}

// TestHelpDoesNotResolveARuntime pins the reason backend is a function: help must
// not open a database, a socket or anything else a real runtime needs.
func TestHelpDoesNotResolveARuntime(t *testing.T) {
	var resolved bool
	root := newRootWithBackend(func(*cobra.Command) (client.Runtime, error) {
		resolved = true
		return instant(), nil
	})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--help"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("--help: %v", err)
	}
	if resolved {
		t.Fatal("--help resolved a runtime")
	}
}

func TestMockNoticeGoesToStderrNotStdout(t *testing.T) {
	// A nil runtime is what a real build gets, and the notice must not land in a
	// pipe that is being parsed.
	out, errb, err := exec(t, nil, "", "sessions", "ls")
	if err != nil {
		t.Fatalf("sessions ls: %v", err)
	}
	if strings.Contains(out, "mock") {
		t.Fatalf("the mock notice leaked into stdout:\n%s", out)
	}
	if !strings.Contains(errb, mockNotice) {
		t.Fatalf("stderr = %q, want the mock notice", errb)
	}
}
