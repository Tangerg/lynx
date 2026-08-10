package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
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
	err := root.ExecuteContext(t.Context())
	return out.String(), errb.String(), err
}

func instant() *mock.Runtime {
	rt := mock.New()
	rt.Instant = true
	return rt
}

func firstSession(t *testing.T, rt client.Runtime) string {
	t.Helper()
	sessions, err := rt.ListSessions(t.Context(), client.SessionQuery{})
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

func TestRunRecoversTransportFaultsWithoutRenderingDuplicates(t *testing.T) {
	for _, fault := range []mock.FaultKind{mock.FaultDisconnect, mock.FaultDuplicate, mock.FaultGap} {
		t.Run(string(fault), func(t *testing.T) {
			rt := instant()
			rt.Script = shortCompletedScript
			rt.Faults = []mock.SubscriptionFault{{Kind: fault, After: 1}}
			out, _, err := exec(t, rt, "", "run", "--json", "-s", firstSession(t, rt), "recover")
			if err != nil {
				t.Fatalf("run: %v\n%s", err, out)
			}
			seen := make(map[string]bool)
			var lastType string
			for i, line := range strings.Split(strings.TrimSpace(out), "\n") {
				var frame struct {
					Type    string `json:"type"`
					EventID string `json:"eventId"`
				}
				if err := json.Unmarshal([]byte(line), &frame); err != nil {
					t.Fatalf("line %d: %v", i, err)
				}
				if seen[frame.EventID] {
					t.Fatalf("event %s rendered twice:\n%s", frame.EventID, out)
				}
				seen[frame.EventID] = true
				lastType = frame.Type
			}
			if lastType != "run.finished" {
				t.Fatalf("last event = %q, want run.finished\n%s", lastType, out)
			}
		})
	}
}

type ambiguousControls struct {
	*mock.Runtime
	mu          sync.Mutex
	starts      int
	resumes     int
	requestIDs  []string
	interruptID []string
}

func (r *ambiguousControls) StartRun(ctx context.Context, input client.StartRun) (client.Run, error) {
	run, err := r.Runtime.StartRun(ctx, input)
	if err != nil {
		return client.Run{}, err
	}
	r.mu.Lock()
	r.starts++
	r.requestIDs = append(r.requestIDs, input.RequestID)
	lost := r.starts == 1
	r.mu.Unlock()
	if lost {
		return client.Run{}, fmt.Errorf("lost start response: %w", client.ErrDisconnected)
	}
	return run, nil
}

func (r *ambiguousControls) ResumeRun(ctx context.Context, input client.ResumeRun) error {
	if err := r.Runtime.ResumeRun(ctx, input); err != nil {
		return err
	}
	r.mu.Lock()
	r.resumes++
	r.interruptID = append(r.interruptID, input.InterruptID)
	lost := r.resumes == 1
	r.mu.Unlock()
	if lost {
		return fmt.Errorf("lost resume response: %w", client.ErrDisconnected)
	}
	return nil
}

func (r *ambiguousControls) calls() (int, int, []string, []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.starts, r.resumes, slices.Clone(r.requestIDs), slices.Clone(r.interruptID)
}

func TestRunRecoversAmbiguousControlResponsesWithoutDuplicatingAControl(t *testing.T) {
	base := instant()
	runtime := &ambiguousControls{Runtime: base}
	out, _, err := exec(t, runtime, "", "run", "--json", "--approve-all", "-s", firstSession(t, runtime), "recover controls")
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	starts, resumes, requestIDs, interruptIDs := runtime.calls()
	if starts != 2 || resumes != 2 {
		t.Fatalf("control calls = start %d, resume %d; want one retry each", starts, resumes)
	}
	if requestIDs[0] == "" || requestIDs[0] != requestIDs[1] {
		t.Fatalf("start retries used request identities %q", requestIDs)
	}
	if interruptIDs[0] == "" || interruptIDs[0] != interruptIDs[1] {
		t.Fatalf("resume retries used interrupt identities %q", interruptIDs)
	}

	seen := make(map[string]bool)
	counts := make(map[string]int)
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		var frame struct {
			Type    string `json:"type"`
			EventID string `json:"eventId"`
		}
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Fatal(err)
		}
		if seen[frame.EventID] {
			t.Fatalf("event %s rendered twice:\n%s", frame.EventID, out)
		}
		seen[frame.EventID] = true
		counts[frame.Type]++
	}
	if counts["run.started"] != 1 || counts["run.resumed"] != 1 || counts["run.finished"] != 1 {
		t.Fatalf("control event counts = %+v\n%s", counts, out)
	}
}

func TestRunRejectsConflictingReplay(t *testing.T) {
	rt := instant()
	rt.Script = shortCompletedScript
	rt.Faults = []mock.SubscriptionFault{{Kind: mock.FaultConflict, After: 1}}
	_, _, err := exec(t, rt, "", "run", "--json", "-s", firstSession(t, rt), "conflict")
	if !errors.Is(err, client.ErrEventConflict) {
		t.Fatalf("run error = %v, want ErrEventConflict", err)
	}
}

func TestRunQuestionNamesTheResumableSession(t *testing.T) {
	rt := instant()
	rt.Script = func(string) mock.Script {
		return mock.Script{Interaction: client.Question{
			InterruptID: "question_1", Title: "Choose a strategy",
			Fields: []client.QuestionField{{ID: "strategy", Label: "Strategy", Kind: client.QuestionText}},
		}}
	}
	id := firstSession(t, rt)
	_, _, err := exec(t, rt, "", "run", "-s", id, "ask me")
	if err == nil || !strings.Contains(err.Error(), "--session "+id) || strings.Contains(err.Error(), "<session-id>") {
		t.Fatalf("question error = %v", err)
	}
}

type alwaysDisconnected struct{ client.Runtime }

func (alwaysDisconnected) FollowRun(context.Context, client.FollowRun) (client.Stream, error) {
	return nil, fmt.Errorf("test transport: %w", client.ErrDisconnected)
}

func TestRunStopsAfterReconnectBudgetIsExhausted(t *testing.T) {
	rt := instant()
	rt.Script = shortCompletedScript
	_, _, err := exec(t, alwaysDisconnected{Runtime: rt}, "", "--reconnect-attempts", "2", "run", "-s", firstSession(t, rt), "offline")
	if !errors.Is(err, client.ErrDisconnected) {
		t.Fatalf("run error = %v, want ErrDisconnected", err)
	}
}

func shortCompletedScript(string) mock.Script {
	return mock.Script{Prelude: []mock.Step{
		{Event: client.BlockCompleted{Block: client.Block{ID: "answer", Kind: client.BlockAssistant, Text: "done"}}},
		{Event: client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCompleted}}},
	}}
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

func TestRunAttachesRepeatedFilesAndAllowsAttachmentOnlyPrompts(t *testing.T) {
	workspace := t.TempDir()
	first := filepath.Join(workspace, "notes.txt")
	second := filepath.Join(workspace, "diagram.png")
	if err := os.WriteFile(first, []byte("notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime := instant()
	id := firstSession(t, runtime)
	if _, _, err := exec(t, runtime, "", "-C", workspace, "run", "--approve-all", "-s", id, "-f", "notes.txt", "-f", "diagram.png"); err != nil {
		t.Fatalf("attachment-only run: %v", err)
	}
	snapshot, err := runtime.GetSession(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	var prompt client.Block
	for _, envelope := range snapshot.Events {
		if event, ok := envelope.Event.(client.BlockCompleted); ok && event.Block.Kind == client.BlockUser {
			prompt = event.Block
		}
	}
	if prompt.Text != "" || len(prompt.Attachments) != 2 {
		t.Fatalf("prompt block = %+v", prompt)
	}
	if prompt.Attachments[0].Kind != client.AttachmentText || prompt.Attachments[1].Kind != client.AttachmentImage {
		t.Fatalf("attachment kinds = %+v", prompt.Attachments)
	}
}

func TestRunRejectsInvalidAttachmentBeforeCreatingASession(t *testing.T) {
	runtime := instant()
	before, _ := runtime.ListSessions(t.Context(), client.SessionQuery{Limit: 100})
	_, _, err := exec(t, runtime, "", "-C", t.TempDir(), "run", "-f", "missing.txt")
	if err == nil || !strings.Contains(err.Error(), "missing.txt") {
		t.Fatalf("error = %v", err)
	}
	after, _ := runtime.ListSessions(t.Context(), client.SessionQuery{Limit: 100})
	if len(after.Items) != len(before.Items) {
		t.Fatalf("invalid input created a session: %d -> %d", len(before.Items), len(after.Items))
	}
}

func TestResolveAttachmentsDeduplicatesCanonicalPathsBeforeApplyingTheLimit(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "one.txt"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths := make([]string, 20)
	for i := range paths {
		paths[i] = "one.txt"
	}
	got, err := resolveAttachments(t.Context(), workspace, paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("attachments = %+v, want one canonical file", got)
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
	before, _ := rt.ListSessions(t.Context(), client.SessionQuery{Limit: 100})
	if _, _, err := exec(t, rt, "", "run", "--approve-all", "why?"); err != nil {
		t.Fatalf("run: %v", err)
	}
	after, _ := rt.ListSessions(t.Context(), client.SessionQuery{Limit: 100})
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

	forked, _, err := exec(t, runtime, "", "sessions", "fork", id, "--at", "4", "--title", "Alternative")
	if err != nil {
		t.Fatalf("sessions fork: %v", err)
	}
	forkID := strings.TrimSpace(forked)
	snapshot, err := runtime.GetSession(t.Context(), forkID)
	if err != nil || snapshot.Session.Title != "Alternative" || len(snapshot.Events) != 4 {
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

func TestApprovalRuleCommandsInspectAndForget(t *testing.T) {
	runtime := instant()
	sessionID := firstSession(t, runtime)
	run, err := runtime.StartRun(t.Context(), client.StartRun{SessionID: sessionID, Message: client.Message{Text: "remember this"}})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := runtime.FollowRun(t.Context(), client.FollowRun{RunID: run.ID, After: run.StartedAfter})
	if err != nil {
		t.Fatal(err)
	}
	var interrupted client.Approval
	var after client.Cursor
	for envelope, streamErr := range stream {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
		after = envelope.Cursor
		if event, ok := envelope.Event.(client.RunInterrupted); ok {
			interrupted = event.Interaction.(client.Approval)
		}
	}
	if err := runtime.ResumeRun(t.Context(), client.ResumeRun{
		RunID: run.ID, InterruptID: interrupted.InterruptID,
		Answer: client.ApprovalAnswer{Decision: client.ApprovalAllow, Remember: client.RememberProject},
	}); err != nil {
		t.Fatal(err)
	}
	continuation, err := runtime.FollowRun(t.Context(), client.FollowRun{RunID: run.ID, After: after})
	if err != nil {
		t.Fatal(err)
	}
	for _, streamErr := range continuation {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
	}

	out, _, err := exec(t, runtime, "", "approvals", "ls")
	if err != nil || !strings.Contains(out, "edit:internal/store/cache_test.go") || !strings.Contains(out, "project") {
		t.Fatalf("approvals ls = %q, %v", out, err)
	}
	rules, _ := runtime.ListApprovalRules(t.Context())
	if _, _, err := exec(t, runtime, "", "approvals", "forget", rules[0].ID); err == nil {
		t.Fatal("approvals forget did not require --yes")
	}
	if _, _, err := exec(t, runtime, "", "approvals", "forget", "--yes", rules[0].ID); err != nil {
		t.Fatal(err)
	}
}

func TestCompletionCommand(t *testing.T) {
	out, _, err := exec(t, instant(), "", "completion", "zsh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "compdef") || !strings.Contains(out, "lyra") {
		t.Fatalf("zsh completion is incomplete:\n%s", out)
	}
}

// TestHelpDoesNotResolveARuntime pins lazy resolution: help must not open a
// database, a socket, or anything else a real runtime needs.
func TestHelpDoesNotResolveARuntime(t *testing.T) {
	var resolved bool
	root := newRootWithBackend(backend{open: func(*cobra.Command) (client.Runtime, error) {
		resolved = true
		return instant(), nil
	}})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--help"})
	if err := root.ExecuteContext(t.Context()); err != nil {
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

func TestMockCompletionDoesNotPrintRuntimeNotice(t *testing.T) {
	out, errb, err := exec(t, nil, "", "__complete", "sessions", "show", "")
	if err != nil {
		t.Fatalf("complete sessions: %v", err)
	}
	if !strings.Contains(out, "ses_demo_") {
		t.Fatalf("completion output has no session ids:\n%s", out)
	}
	if strings.Contains(errb, mockNotice) {
		t.Fatalf("completion stderr contains runtime notice: %q", errb)
	}
}
