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
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
)

// executeCommand runs the CLI in memory and returns stdout, stderr and the command error.
// Nothing here touches the process's own streams, which is what lets the tests
// run in parallel and assert on exact output.
func executeCommand(t *testing.T, rt agent.Runtime, stdin string, args ...string) (string, string, error) {
	t.Helper()
	var out, errb bytes.Buffer
	dependencies := Dependencies{OpenRuntime: func(context.Context) (agent.Runtime, error) { return rt, nil }}
	if rt == nil {
		dependencies.OpenRuntime = func(context.Context) (agent.Runtime, error) { return mock.New(), nil }
		dependencies.RuntimeNotice = testRuntimeNotice
	}
	root := NewRoot(dependencies)
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	err := root.ExecuteContext(t.Context())
	return out.String(), errb.String(), err
}

func instantRuntime() *mock.Runtime {
	rt := mock.New()
	rt.Instant = true
	return rt
}

func firstSession(t *testing.T, rt agent.Runtime) string {
	t.Helper()
	sessions, err := rt.ListSessions(t.Context(), agent.SessionQuery{})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	return sessions.Items[0].ID
}

func TestRunDeclinesApprovalWhenUnattended(t *testing.T) {
	rt := instantRuntime()
	out, _, err := executeCommand(t, rt, "", "run", "-s", firstSession(t, rt), "why?")
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "? edit internal/store/cache_test.go") {
		t.Fatalf("output does not show the approval request:\n%s", out)
	}
	if !strings.Contains(out, "Edit declined") {
		t.Fatalf("output does not show the run continuing after the refusal:\n%s", out)
	}
	if !strings.Contains(out, "● edit") || !strings.Contains(out, "✗") || strings.Contains(out, "count=50") {
		t.Fatalf("the denied tool lifecycle is incorrect:\n%s", out)
	}
	if !strings.Contains(out, "↑ ") {
		t.Fatalf("output has no usage footer, so the run did not finish:\n%s", out)
	}
}

func TestRunApproveAllLetsTheRunThrough(t *testing.T) {
	rt := instantRuntime()
	out, _, err := executeCommand(t, rt, "", "run", "--approve-all", "-s", firstSession(t, rt), "why?")
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

func TestRunStreamingJSONIsOneObjectPerLineEndingWithTheRun(t *testing.T) {
	rt := instantRuntime()
	out, _, err := executeCommand(t, rt, "", "run", "--output-format", "streaming-json", "--approve-all", "-s", firstSession(t, rt), "why?")
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected a stream of frames, got:\n%s", out)
	}
	requireTypedJSONLines(t, lines)
	first := decodeRunFrame(t, lines[0])
	if first.Type != "segment.started" {
		t.Fatalf("first frame = %s, want segment.started", lines[0])
	}
	last := decodeRunFrame(t, lines[len(lines)-1])
	if last.Type != "run.finished" || last.Outcome.Status != "completed" {
		t.Fatalf("last frame = %s, want a completed run.finished", lines[len(lines)-1])
	}
}

func TestRunJSONIsOneFinalResult(t *testing.T) {
	runtime := instantRuntime()
	out, _, err := executeCommand(t, runtime, "", "run", "--json", "--approve-all", "-s", firstSession(t, runtime), "why?")
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if strings.Count(strings.TrimSpace(out), "\n") != 0 {
		t.Fatalf("result JSON spans multiple records:\n%s", out)
	}
	result := decodeResult(t, out)
	if result.Type != "result" || result.Status != "completed" || result.RunID == "" || result.SessionID == "" {
		t.Fatalf("result identity = %+v", result)
	}
	if result.Outcome.Status != "completed" || !strings.Contains(result.Text, "Replaced the sleep") {
		t.Fatalf("result payload = %+v", result)
	}
}

type commandResult struct {
	Type      string `json:"type"`
	Status    string `json:"status"`
	RunID     string `json:"runId"`
	SessionID string `json:"sessionId"`
	Text      string `json:"text"`
	Outcome   struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	} `json:"outcome"`
	Interactions []struct {
		Kind  string `json:"kind"`
		Title string `json:"title"`
	} `json:"interactions"`
}

func decodeResult(t *testing.T, output string) commandResult {
	t.Helper()
	var result commandResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("result is not JSON: %v\n%s", err, output)
	}
	return result
}

type runFrame struct {
	Type    string `json:"type"`
	Outcome struct {
		Status string `json:"status"`
	} `json:"outcome"`
}

func requireTypedJSONLines(t *testing.T, lines []string) {
	t.Helper()
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
}

func decodeRunFrame(t *testing.T, line string) runFrame {
	t.Helper()
	var last struct {
		Type    string `json:"type"`
		Outcome struct {
			Status string `json:"status"`
		} `json:"outcome"`
	}
	if err := json.Unmarshal([]byte(line), &last); err != nil {
		t.Fatalf("frame is not JSON: %v", err)
	}
	return runFrame(last)
}

func TestRunRecoversTransportFaultsWithoutRenderingDuplicates(t *testing.T) {
	for _, fault := range []mock.FaultKind{mock.FaultDisconnect, mock.FaultDuplicate} {
		t.Run(string(fault), func(t *testing.T) {
			rt := instantRuntime()
			rt.Script = shortCompletedScript
			rt.Faults = []mock.SubscriptionFault{{Kind: fault, After: 1}}
			out, _, err := executeCommand(t, rt, "", "run", "--output-format", "streaming-json", "-s", firstSession(t, rt), "recover")
			if err != nil {
				t.Fatalf("run: %v\n%s", err, out)
			}
			seen := make(map[string]bool)
			var lastType, lastStatus string
			for i, line := range strings.Split(strings.TrimSpace(out), "\n") {
				var frame struct {
					Type    string `json:"type"`
					EventID string `json:"eventId"`
					Status  string `json:"status"`
				}
				if err := json.Unmarshal([]byte(line), &frame); err != nil {
					t.Fatalf("line %d: %v", i, err)
				}
				if frame.EventID != "" && seen[frame.EventID] {
					t.Fatalf("event %s rendered twice:\n%s", frame.EventID, out)
				}
				if frame.EventID != "" {
					seen[frame.EventID] = true
				}
				lastType, lastStatus = frame.Type, frame.Status
			}
			if lastType != "run.finished" && (lastType != "run.snapshot" || lastStatus != "finished") {
				t.Fatalf("last event = %q (%q), want run.finished or a finished run.snapshot\n%s", lastType, lastStatus, out)
			}
		})
	}
}

type ambiguousControls struct {
	*mock.Runtime

	mu         sync.Mutex
	starts     int
	resumes    int
	loseStart  bool
	loseResume bool
}

func (r *ambiguousControls) StartRun(ctx context.Context, input agent.StartRun) (agent.SegmentStream, error) {
	stream, err := r.Runtime.StartRun(ctx, input)
	if err != nil {
		return agent.SegmentStream{}, err
	}
	r.mu.Lock()
	r.starts++
	lost := r.loseStart
	r.mu.Unlock()
	if lost {
		return agent.SegmentStream{}, fmt.Errorf("lost start response: %w", agent.ErrDisconnected)
	}
	return stream, nil
}

func (r *ambiguousControls) ResumeRun(ctx context.Context, input agent.ResumeRun) (agent.SegmentStream, error) {
	stream, err := r.Runtime.ResumeRun(ctx, input)
	if err != nil {
		return agent.SegmentStream{}, err
	}
	r.mu.Lock()
	r.resumes++
	lost := r.loseResume
	r.mu.Unlock()
	if lost {
		return agent.SegmentStream{}, fmt.Errorf("lost resume response: %w", agent.ErrDisconnected)
	}
	return stream, nil
}

func (r *ambiguousControls) calls() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.starts, r.resumes
}

func TestRunDoesNotRetryAmbiguousControlOperations(t *testing.T) {
	t.Run("start", func(t *testing.T) {
		runtime := &ambiguousControls{Runtime: instantRuntime(), loseStart: true}
		_, _, err := executeCommand(t, runtime, "", "run", "-s", firstSession(t, runtime), "start once")
		if !errors.Is(err, agent.ErrDisconnected) {
			t.Fatalf("run error = %v, want ErrDisconnected", err)
		}
		starts, resumes := runtime.calls()
		if starts != 1 || resumes != 0 {
			t.Fatalf("control calls = start %d, resume %d; want 1, 0", starts, resumes)
		}
	})

	t.Run("resume", func(t *testing.T) {
		runtime := &ambiguousControls{Runtime: instantRuntime(), loseResume: true}
		_, _, err := executeCommand(t, runtime, "", "run", "--approve-all", "-s", firstSession(t, runtime), "resume once")
		if !errors.Is(err, agent.ErrDisconnected) {
			t.Fatalf("run error = %v, want ErrDisconnected", err)
		}
		starts, resumes := runtime.calls()
		if starts != 1 || resumes != 1 {
			t.Fatalf("control calls = start %d, resume %d; want 1, 1", starts, resumes)
		}
	})
}

func TestRunRejectsConflictingReplay(t *testing.T) {
	rt := instantRuntime()
	rt.Script = shortCompletedScript
	rt.Faults = []mock.SubscriptionFault{{Kind: mock.FaultConflict, After: 1}}
	_, _, err := executeCommand(t, rt, "", "run", "--output-format", "streaming-json", "-s", firstSession(t, rt), "conflict")
	if !errors.Is(err, agent.ErrEventConflict) {
		t.Fatalf("run error = %v, want ErrEventConflict", err)
	}
}

func TestRunQuestionNamesTheResumableSession(t *testing.T) {
	rt := instantRuntime()
	rt.Script = func(string) mock.Script {
		return mock.Script{Interactions: []agent.Interaction{agent.Question{
			ItemID: "question_1", Title: "Choose a strategy",
			Fields: []agent.QuestionField{{Prompt: "Strategy", Kind: agent.QuestionText}},
		}}}
	}
	id := firstSession(t, rt)
	out, _, err := executeCommand(t, rt, "", "run", "--json", "-s", id, "ask me")
	if err == nil || !strings.Contains(err.Error(), "--session "+id) || strings.Contains(err.Error(), "<session-id>") {
		t.Fatalf("question error = %v", err)
	}
	result := decodeResult(t, out)
	if result.Status != "interrupted" || len(result.Interactions) != 1 || result.Interactions[0].Kind != "question" || result.Interactions[0].Title != "Choose a strategy" {
		t.Fatalf("interrupted result = %+v", result)
	}
	snapshot, getErr := rt.GetSession(t.Context(), id)
	if getErr != nil {
		t.Fatal(getErr)
	}
	active, activeOK := snapshot.ActiveRun()
	if !activeOK || active.Status != agent.RunStatusWaiting {
		t.Fatalf("question did not leave a resumable waiting run: %+v", snapshot.Runs)
	}
	if len(snapshot.Interactions) != 1 || agent.InteractionItemID(snapshot.Interactions[0]) == "" {
		t.Fatalf("question waiting set = %+v, want one pending interaction", snapshot.Interactions)
	}
}

func TestRunReturnsAnErrorForNonCompletedOutcomes(t *testing.T) {
	for _, test := range []struct {
		name    string
		outcome agent.Outcome
		want    string
	}{
		{name: "failed", outcome: agent.Outcome{Status: agent.OutcomeFailed, Error: "provider refused"}, want: "run failed: provider refused"},
		{name: "canceled", outcome: agent.Outcome{Status: agent.OutcomeCanceled}, want: "run canceled"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := instantRuntime()
			runtime.Script = func(string) mock.Script {
				return mock.Script{Prelude: []mock.Step{{Event: agent.RunFinished{Outcome: test.outcome}}}}
			}
			id := firstSession(t, runtime)
			out, _, err := executeCommand(t, runtime, "", "run", "--json", "-s", id, "finish this way")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("run error = %v, want %q", err, test.want)
			}
			result := decodeResult(t, out)
			if result.Type != "result" || result.Status != string(test.outcome.Status) || result.Outcome.Status != string(test.outcome.Status) {
				t.Fatalf("result = %+v", result)
			}
			snapshot, getErr := runtime.GetSession(t.Context(), id)
			if getErr != nil {
				t.Fatal(getErr)
			}
			if _, active := snapshot.ActiveRun(); active {
				t.Fatalf("settled outcome left an active run: %+v", snapshot.Runs)
			}
		})
	}
}

func TestRunRejectsInvalidAndConflictingOutputFormatsBeforeCreatingASession(t *testing.T) {
	for _, args := range [][]string{
		{"run", "--output-format", "xml", "prompt"},
		{"run", "--json", "--output-format", "streaming-json", "prompt"},
	} {
		t.Run(strings.Join(args[1:3], " "), func(t *testing.T) {
			runtime := instantRuntime()
			before, _ := runtime.ListSessions(t.Context(), agent.SessionQuery{Limit: 100})
			if _, _, err := executeCommand(t, runtime, "", args...); err == nil {
				t.Fatalf("arguments %v were accepted", args)
			}
			after, _ := runtime.ListSessions(t.Context(), agent.SessionQuery{Limit: 100})
			if len(after.Items) != len(before.Items) {
				t.Fatalf("invalid output format created a session: %d -> %d", len(before.Items), len(after.Items))
			}
		})
	}
}

func TestOutputFormatCompletionFiltersCandidates(t *testing.T) {
	items, directive := completeOutputFormat(nil, nil, "stream")
	if directive != cobra.ShellCompDirectiveNoFileComp || len(items) != 1 || !strings.HasPrefix(items[0], "streaming-json\t") {
		t.Fatalf("output format completion = %v, %v", items, directive)
	}
}

type alwaysDisconnected struct{ agent.Runtime }

func (runtime alwaysDisconnected) StartRun(ctx context.Context, input agent.StartRun) (agent.SegmentStream, error) {
	stream, err := runtime.Runtime.StartRun(ctx, input)
	if err != nil {
		return agent.SegmentStream{}, err
	}
	upstream := stream.Events
	stream.Events = func(yield func(agent.RunEvent, error) bool) {
		for event, streamErr := range upstream {
			if !yield(event, streamErr) || streamErr != nil {
				return
			}
			yield(agent.RunEvent{}, fmt.Errorf("test transport: %w", agent.ErrDisconnected))
			return
		}
	}
	return stream, nil
}

func (alwaysDisconnected) SubscribeRun(context.Context, agent.SubscribeRun) (agent.SegmentStream, error) {
	return agent.SegmentStream{}, fmt.Errorf("test transport: %w", agent.ErrDisconnected)
}

func TestRunStopsAfterReconnectBudgetIsExhausted(t *testing.T) {
	rt := instantRuntime()
	rt.Script = shortCompletedScript
	out, _, err := executeCommand(t, alwaysDisconnected{Runtime: rt}, "", "--reconnect-attempts", "2", "run", "--json", "-s", firstSession(t, rt), "offline")
	if !errors.Is(err, agent.ErrDisconnected) {
		t.Fatalf("run error = %v, want ErrDisconnected", err)
	}
	result := decodeResult(t, out)
	if result.Status != "incomplete" || result.RunID == "" || result.SessionID == "" {
		t.Fatalf("incomplete result = %+v", result)
	}
}

func shortCompletedScript(string) mock.Script {
	return mock.Script{Prelude: []mock.Step{
		{Event: agent.BlockCompleted{Block: agent.Block{ID: "answer", Kind: agent.BlockAssistant, Text: "done"}}},
		{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
	}}
}

func TestRunReadsAPipedPromptAndCombinesItWithTheArgument(t *testing.T) {
	var captured string
	rt := instantRuntime()
	rt.Script = func(prompt string) mock.Script {
		captured = prompt
		return mock.Script{Prelude: []mock.Step{{Event: agent.RunFinished{
			Outcome: agent.Outcome{Status: agent.OutcomeCompleted},
		}}}}
	}
	if _, _, err := executeCommand(t, rt, "file contents\n", "run", "-s", firstSession(t, rt), "explain this"); err != nil {
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
	writeCommandFixture(t, first, []byte("notes"))
	writeCommandFixture(t, second, append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...))
	runtime := instantRuntime()
	id := firstSession(t, runtime)
	if _, _, err := executeCommand(t, runtime, "", "-C", workspace, "run", "--approve-all", "-s", id, "-f", "notes.txt", "-f", "diagram.png"); err != nil {
		t.Fatalf("attachment-only run: %v", err)
	}
	snapshot, err := runtime.GetSession(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	prompt := userPromptBlock(t, snapshot.Transcript)
	if prompt.Text != "" || len(prompt.Attachments) != 2 {
		t.Fatalf("prompt block = %+v", prompt)
	}
	if prompt.Attachments[0].Kind != agent.AttachmentText || prompt.Attachments[1].Kind != agent.AttachmentImage {
		t.Fatalf("attachment kinds = %+v", prompt.Attachments)
	}
}

func writeCommandFixture(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func userPromptBlock(t *testing.T, blocks []agent.Block) agent.Block {
	t.Helper()
	for _, block := range slices.Backward(blocks) {
		if block.Kind == agent.BlockUser {
			return block
		}
	}
	t.Fatal("user prompt block was not emitted")
	return agent.Block{}
}

func TestRunRejectsInvalidAttachmentBeforeCreatingASession(t *testing.T) {
	runtime := instantRuntime()
	before, _ := runtime.ListSessions(t.Context(), agent.SessionQuery{Limit: 100})
	_, _, err := executeCommand(t, runtime, "", "-C", t.TempDir(), "run", "-f", "missing.txt")
	if err == nil || !strings.Contains(err.Error(), "missing.txt") {
		t.Fatalf("error = %v", err)
	}
	after, _ := runtime.ListSessions(t.Context(), agent.SessionQuery{Limit: 100})
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
	rt := instantRuntime()
	_, _, err := executeCommand(t, rt, "", "run", "-s", firstSession(t, rt))
	if !errors.Is(err, errNoPrompt) {
		t.Fatalf("err = %v, want errNoPrompt", err)
	}
}

func TestRunRejectsAnUnknownSession(t *testing.T) {
	_, _, err := executeCommand(t, instantRuntime(), "", "run", "-s", "ses_nope", "why?")
	if !errors.Is(err, agent.ErrSessionNotFound) {
		t.Fatalf("err = %v, want ErrSessionNotFound", err)
	}
}

func TestRunCreatesASessionWhenNoneIsNamed(t *testing.T) {
	rt := instantRuntime()
	before, _ := rt.ListSessions(t.Context(), agent.SessionQuery{Limit: 100})
	if _, _, err := executeCommand(t, rt, "", "run", "--approve-all", "why?"); err != nil {
		t.Fatalf("run: %v", err)
	}
	after, _ := rt.ListSessions(t.Context(), agent.SessionQuery{Limit: 100})
	if len(after.Items) != len(before.Items)+1 {
		t.Fatalf("session count went %d -> %d, want one more", len(before.Items), len(after.Items))
	}
}

func TestWorkspaceFlagIsNormalizedBeforeCreatingASession(t *testing.T) {
	runtime := instantRuntime()
	want, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	current, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(current, want)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := runtime.ListSessions(t.Context(), agent.SessionQuery{Limit: 100})
	if _, _, err := executeCommand(t, runtime, "", "-C", relative, "run", "--approve-all", "normalize workspace"); err != nil {
		t.Fatal(err)
	}
	after, _ := runtime.ListSessions(t.Context(), agent.SessionQuery{Limit: 100})
	if len(after.Items) != len(before.Items)+1 || after.Items[0].Workspace != want {
		t.Fatalf("newest session workspace = %q, want %q", after.Items[0].Workspace, want)
	}
}

func TestSessionsList(t *testing.T) {
	out, _, err := executeCommand(t, instantRuntime(), "", "sessions", "ls")
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
	if _, _, err := executeCommand(t, instantRuntime(), "", "session", "list"); err != nil {
		t.Fatalf("aliases should reach the same command: %v", err)
	}
	if _, _, err := executeCommand(t, instantRuntime(), "", "sessions", "ls", "extra"); err == nil {
		t.Fatal("sessions ls accepted an argument it has no use for")
	}
}

func TestSessionManagementCommands(t *testing.T) {
	runtime := instantRuntime()
	id := firstSession(t, runtime)
	requireSessionShow(t, runtime, id)
	requireSessionRename(t, runtime, id)
	forkID := forkTestSession(t, runtime, id)
	requireSessionDelete(t, runtime, forkID)
}

func TestSessionShowJSONUsesTheCLISnapshotContract(t *testing.T) {
	runtime := instantRuntime()
	out, _, err := executeCommand(t, runtime, "", "sessions", "show", firstSession(t, runtime), "--json")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
		Transcript []json.RawMessage `json:"transcript"`
		Runs       []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(out), &snapshot); err != nil {
		t.Fatalf("session snapshot is not JSON: %v\n%s", err, out)
	}
	if snapshot.Session.ID == "" || len(snapshot.Transcript) != 2 || len(snapshot.Runs) != 1 || snapshot.Runs[0].Status != "finished" {
		t.Fatalf("session snapshot = %+v", snapshot)
	}
	if strings.Contains(out, `"ID"`) || !strings.Contains(out, `"sessionId"`) {
		t.Fatalf("session snapshot leaked Go field names:\n%s", out)
	}
}

func requireSessionShow(t *testing.T, runtime agent.Runtime, id string) {
	t.Helper()
	shown, _, err := executeCommand(t, runtime, "", "sessions", "show", id)
	if err != nil || !strings.Contains(shown, "The fixed sleep races the janitor") {
		t.Fatalf("sessions show = %q, %v", shown, err)
	}
}

func requireSessionRename(t *testing.T, runtime agent.Runtime, id string) {
	t.Helper()
	snapshot, err := runtime.GetSession(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	renamed, _, err := executeCommand(t, runtime, "", "sessions", "rename", id, "Investigate cache", "--revision", strconv.FormatUint(snapshot.Session.Revision, 10))
	if err != nil || !strings.Contains(renamed, "Investigate cache") {
		t.Fatalf("sessions rename = %q, %v", renamed, err)
	}
}

func forkTestSession(t *testing.T, runtime agent.Runtime, id string) string {
	t.Helper()
	forked, _, err := executeCommand(t, runtime, "", "sessions", "fork", id, "--from-run", "run_demo_history", "--title", "Alternative")
	if err != nil {
		t.Fatalf("sessions fork: %v", err)
	}
	forkID := strings.TrimSpace(forked)
	snapshot, err := runtime.GetSession(t.Context(), forkID)
	if err != nil || snapshot.Session.Title != "Alternative" || len(snapshot.Transcript) != 0 || len(snapshot.Runs) != 0 {
		t.Fatalf("fork snapshot = %+v, %v", snapshot, err)
	}
	return forkID
}

func requireSessionDelete(t *testing.T, runtime agent.Runtime, id string) {
	t.Helper()
	if _, _, err := executeCommand(t, runtime, "", "sessions", "delete", id); err == nil {
		t.Fatal("sessions delete did not require confirmation")
	}
	if _, _, err := executeCommand(t, runtime, "", "sessions", "rm", "--yes", id); err != nil {
		t.Fatalf("sessions rm --yes: %v", err)
	}
}

func TestSessionsListPaginatesAndSearches(t *testing.T) {
	runtime := instantRuntime()
	out, errOut, err := executeCommand(t, runtime, "", "sessions", "ls", "--limit", "1", "--search", "store")
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.Split(strings.TrimSpace(out), "\n")) != 1 || !strings.Contains(errOut, "more sessions: --cursor") {
		t.Fatalf("page output=%q stderr=%q", out, errOut)
	}
}

func TestSessionsListJSONKeepsPaginationOnStdout(t *testing.T) {
	runtime := instantRuntime()
	out, errOut, err := executeCommand(t, runtime, "", "sessions", "ls", "--limit", "1", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if errOut != "" {
		t.Fatalf("JSON session list wrote pagination to stderr: %q", errOut)
	}
	var page sessionPageJSON
	if err := json.Unmarshal([]byte(out), &page); err != nil {
		t.Fatalf("session page is not JSON: %v\n%s", err, out)
	}
	if len(page.Items) != 1 || page.Items[0].ID == "" || page.NextCursor == "" {
		t.Fatalf("session page = %+v", page)
	}
}

func TestApprovalRuleCommandsInspectAndForget(t *testing.T) {
	runtime := instantRuntime()
	sessionID := firstSession(t, runtime)
	ruleID := createProjectApprovalRule(t, runtime, sessionID)
	requireApprovalList(t, runtime, sessionID)
	jsonOut, _, jsonErr := executeCommand(t, runtime, "", "approvals", "ls", "--session", sessionID, "--json")
	if jsonErr != nil {
		t.Fatal(jsonErr)
	}
	var rules struct {
		Rules []approvalRuleJSON `json:"rules"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &rules); err != nil || len(rules.Rules) != 1 || rules.Rules[0].ID != ruleID || rules.Rules[0].Decision != "allow" {
		t.Fatalf("approval JSON = %+v, %v", rules, err)
	}
	out, _, err := executeCommand(t, runtime, "", "__complete", "approvals", "delete", "--session", sessionID, "")
	if err != nil || !strings.Contains(out, ruleID) {
		t.Fatalf("approval completion = %q, %v", out, err)
	}
	if _, _, err := executeCommand(t, runtime, "", "approvals", "forget", "--session", sessionID, ruleID); err == nil {
		t.Fatal("approvals forget did not require --yes")
	}
	if _, _, err := executeCommand(t, runtime, "", "approvals", "forget", "--session", sessionID, "--yes", ruleID); err != nil {
		t.Fatal(err)
	}
}

func createProjectApprovalRule(t *testing.T, runtime agent.Runtime, sessionID string) string {
	t.Helper()
	stream, err := runtime.StartRun(t.Context(), agent.StartRun{SessionID: sessionID, Message: agent.Message{Text: "remember this"}})
	if err != nil {
		t.Fatal(err)
	}
	interrupted := followApprovalInterrupt(t, stream)
	continuation := resumeProjectApproval(t, runtime, stream.RunID, interrupted)
	drainContinuation(t, continuation)
	return onlyApprovalRule(t, runtime, sessionID).ID
}

func followApprovalInterrupt(t *testing.T, stream agent.SegmentStream) agent.Approval {
	t.Helper()
	var interrupted agent.Approval
	for event, streamErr := range stream.Events {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
		if parked, ok := event.Event.(agent.RunInterrupted); ok {
			if len(parked.Interactions) != 1 {
				t.Fatalf("pending interactions = %+v, want one", parked.Interactions)
			}
			interrupted, _ = parked.Interactions[0].(agent.Approval)
		}
	}
	if interrupted.ItemID == "" {
		t.Fatal("run did not request approval")
	}
	return interrupted
}

func resumeProjectApproval(t *testing.T, runtime agent.Runtime, runID string, interrupted agent.Approval) agent.SegmentStream {
	t.Helper()
	stream, err := runtime.ResumeRun(t.Context(), agent.ResumeRun{
		RunID: runID, Answers: []agent.InterruptAnswer{{
			ItemID: interrupted.ItemID,
			Answer: agent.ApprovalAnswer{Decision: agent.ApprovalApprove, Remember: agent.RememberProject},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return stream
}

func drainContinuation(t *testing.T, stream agent.SegmentStream) {
	t.Helper()
	for _, streamErr := range stream.Events {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
	}
}

func onlyApprovalRule(t *testing.T, runtime agent.Runtime, sessionID string) agent.ApprovalRule {
	t.Helper()
	rules, err := runtime.ListApprovalRules(t.Context(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("approval rules = %+v, want one", rules)
	}
	return rules[0]
}

func requireApprovalList(t *testing.T, runtime agent.Runtime, sessionID string) {
	t.Helper()
	out, _, err := executeCommand(t, runtime, "", "approvals", "ls", "--session", sessionID)
	if err != nil || !strings.Contains(out, "edit") || !strings.Contains(out, "internal/store/cache_test.go") || !strings.Contains(out, "project") {
		t.Fatalf("approvals ls = %q, %v", out, err)
	}
}

func TestCompletionCommand(t *testing.T) {
	out, _, err := executeCommand(t, instantRuntime(), "", "completion", "zsh")
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
	root := NewRoot(Dependencies{OpenRuntime: func(context.Context) (agent.Runtime, error) {
		resolved = true
		return instantRuntime(), nil
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

const testRuntimeNotice = "lyra: scripted mock runtime — no backend is wired in yet"

func TestRuntimeNoticeGoesToStderrNotStdout(t *testing.T) {
	// A nil runtime is what a real build gets, and the notice must not land in a
	// pipe that is being parsed.
	out, errb, err := executeCommand(t, nil, "", "sessions", "ls")
	if err != nil {
		t.Fatalf("sessions ls: %v", err)
	}
	if strings.Contains(out, "mock") {
		t.Fatalf("the mock notice leaked into stdout:\n%s", out)
	}
	if !strings.Contains(errb, testRuntimeNotice) {
		t.Fatalf("stderr = %q, want the mock notice", errb)
	}
}

func TestCompletionDoesNotPrintRuntimeNotice(t *testing.T) {
	out, errb, err := executeCommand(t, nil, "", "__complete", "sessions", "show", "")
	if err != nil {
		t.Fatalf("complete sessions: %v", err)
	}
	if !strings.Contains(out, "ses_demo_") {
		t.Fatalf("completion output has no session ids:\n%s", out)
	}
	if strings.Contains(errb, testRuntimeNotice) {
		t.Fatalf("completion stderr contains runtime notice: %q", errb)
	}
}
