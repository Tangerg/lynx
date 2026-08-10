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
	"time"

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

func TestRunStreamingJSONIsOneObjectPerLineEndingWithTheRun(t *testing.T) {
	rt := instant()
	out, _, err := exec(t, rt, "", "run", "--output-format", "streaming-json", "--approve-all", "-s", firstSession(t, rt), "why?")
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected a stream of frames, got:\n%s", out)
	}
	requireTypedJSONLines(t, lines)
	first := decodeRunFrame(t, lines[0])
	if first.Type != "run.started" {
		t.Fatalf("first frame = %s, want run.started", lines[0])
	}
	last := decodeRunFrame(t, lines[len(lines)-1])
	if last.Type != "run.finished" || last.Outcome.Status != "completed" {
		t.Fatalf("last frame = %s, want a completed run.finished", lines[len(lines)-1])
	}
}

func TestRunJSONIsOneFinalResult(t *testing.T) {
	runtime := instant()
	out, _, err := exec(t, runtime, "", "run", "--json", "--approve-all", "-s", firstSession(t, runtime), "why?")
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
	Interaction struct {
		Kind  string `json:"kind"`
		Title string `json:"title"`
	} `json:"interaction"`
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
	for _, fault := range []mock.FaultKind{mock.FaultDisconnect, mock.FaultDuplicate, mock.FaultGap} {
		t.Run(string(fault), func(t *testing.T) {
			rt := instant()
			rt.Script = shortCompletedScript
			rt.Faults = []mock.SubscriptionFault{{Kind: fault, After: 1}}
			out, _, err := exec(t, rt, "", "run", "--output-format", "streaming-json", "-s", firstSession(t, rt), "recover")
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

type delayedStartResponse struct {
	*mock.Runtime

	accepted chan struct{}
}

func (r *delayedStartResponse) StartRun(ctx context.Context, input client.StartRun) (client.Run, error) {
	if _, err := r.Runtime.StartRun(ctx, input); err != nil {
		return client.Run{}, err
	}
	close(r.accepted)
	<-ctx.Done()
	return client.Run{}, context.Cause(ctx)
}

type discardRenderer struct{}

func (discardRenderer) Render(client.Envelope) error { return nil }
func (discardRenderer) Close() error                 { return nil }

type lostStartResponses struct{ *mock.Runtime }

func (r *lostStartResponses) StartRun(ctx context.Context, input client.StartRun) (client.Run, error) {
	if _, err := r.Runtime.StartRun(ctx, input); err != nil {
		return client.Run{}, err
	}
	return client.Run{}, fmt.Errorf("start response lost: %w", client.ErrDisconnected)
}

type invalidLifecycleRuntime struct {
	*mock.Runtime

	sessionID string
}

type invalidStartRuntime struct{ *mock.Runtime }

func (r *invalidStartRuntime) StartRun(context.Context, client.StartRun) (client.Run, error) {
	return client.Run{SessionID: "wrong", Status: client.RunActive}, nil
}

func (r *invalidLifecycleRuntime) StartRun(_ context.Context, input client.StartRun) (client.Run, error) {
	return client.Run{ID: "run_invalid", SessionID: input.SessionID, Status: client.RunActive}, nil
}

func (r *invalidLifecycleRuntime) FollowRun(_ context.Context, input client.FollowRun) (client.Stream, error) {
	return func(yield func(client.Envelope, error) bool) {
		if !yield(client.Envelope{
			ID: "event_started", Cursor: input.After + 1,
			RunID: input.RunID, SessionID: r.sessionID,
			Event: client.RunStarted{RunID: input.RunID, SessionID: r.sessionID},
		}, nil) {
			return
		}
		yield(client.Envelope{
			ID: "event_invalid", Cursor: input.After + 2,
			RunID: input.RunID, SessionID: r.sessionID,
			Event: client.RunStarted{RunID: input.RunID, SessionID: r.sessionID},
		}, nil)
	}, nil
}

type countingRenderer struct {
	rendered int
	closed   int
}

func (r *countingRenderer) Render(client.Envelope) error { r.rendered++; return nil }
func (r *countingRenderer) Close() error                 { r.closed++; return nil }

func TestRunRejectsEventsThatViolateTheConversationLifecycle(t *testing.T) {
	base := mock.New()
	sessionID := firstSession(t, base)
	renderer := new(countingRenderer)
	err := follow(t.Context(), &invalidLifecycleRuntime{Runtime: base, sessionID: sessionID}, renderer, client.StartRun{
		SessionID: sessionID, Message: client.Message{Text: "invalid lifecycle"},
	}, false, 0)
	if !errors.Is(err, client.ErrInvalidTransition) {
		t.Fatalf("follow error = %v, want invalid transition", err)
	}
	if renderer.rendered != 1 {
		t.Fatalf("renderer received %d events, want only the valid prefix", renderer.rendered)
	}
	if renderer.closed != 1 {
		t.Fatalf("renderer closed %d times, want once", renderer.closed)
	}
}

func TestRunRejectsAnInvalidStartProjection(t *testing.T) {
	base := mock.New()
	err := follow(t.Context(), &invalidStartRuntime{Runtime: base}, discardRenderer{}, client.StartRun{
		SessionID: firstSession(t, base), Message: client.Message{Text: "invalid start"},
	}, false, 0)
	if err == nil || !strings.Contains(err.Error(), "start run response") {
		t.Fatalf("follow error = %v", err)
	}
}

func TestRunCancellationTargetsAStartWhoseResponseWasLost(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Delay: time.Hour, Event: client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCompleted}}},
		}}
	}
	sessionID := firstSession(t, base)
	runtime := &delayedStartResponse{Runtime: base, accepted: make(chan struct{})}
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		result <- follow(ctx, runtime, discardRenderer{}, client.StartRun{
			SessionID: sessionID,
			Message:   client.Message{Text: "cancel after acceptance"},
		}, false, 0)
	}()

	select {
	case <-runtime.accepted:
	case <-time.After(time.Second):
		t.Fatal("start was not accepted")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("follow error = %v, want context cancellation", err)
	}

	requireCanceledSession(t, base, sessionID)
}

func TestRunCancellationTargetsAStartAfterItsRetryBudgetIsExhausted(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Delay: time.Hour, Event: client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCompleted}}},
		}}
	}
	sessionID := firstSession(t, base)
	err := follow(t.Context(), &lostStartResponses{Runtime: base}, discardRenderer{}, client.StartRun{
		SessionID: sessionID,
		Message:   client.Message{Text: "lose every response"},
	}, false, 1)
	if !errors.Is(err, client.ErrDisconnected) {
		t.Fatalf("follow error = %v, want exhausted transport failure", err)
	}
	requireCanceledSession(t, base, sessionID)
}

func requireCanceledSession(t *testing.T, runtime client.Runtime, sessionID string) {
	t.Helper()
	snapshot, err := runtime.GetSession(t.Context(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Active != nil {
		t.Fatalf("canceled start left an active run: %+v", snapshot.Active)
	}
	finished, ok := snapshot.Events[len(snapshot.Events)-1].Event.(client.RunFinished)
	if !ok || finished.Outcome.Status != client.OutcomeCanceled {
		t.Fatalf("last event = %+v, want canceled run", snapshot.Events[len(snapshot.Events)-1])
	}
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
	out, _, err := exec(t, runtime, "", "run", "--output-format", "streaming-json", "--approve-all", "-s", firstSession(t, runtime), "recover controls")
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	starts, resumes, requestIDs, interruptIDs := runtime.calls()
	requireRetriedControlIdentity(t, starts, resumes, requestIDs, interruptIDs)
	counts := countUniqueFrames(t, out)
	if counts["run.started"] != 1 || counts["run.resumed"] != 1 || counts["run.finished"] != 1 {
		t.Fatalf("control event counts = %+v\n%s", counts, out)
	}
}

func requireRetriedControlIdentity(t *testing.T, starts, resumes int, requestIDs, interruptIDs []string) {
	t.Helper()
	if starts != 2 || resumes != 2 {
		t.Fatalf("control calls = start %d, resume %d; want one retry each", starts, resumes)
	}
	if requestIDs[0] == "" || requestIDs[0] != requestIDs[1] {
		t.Fatalf("start retries used request identities %q", requestIDs)
	}
	if interruptIDs[0] == "" || interruptIDs[0] != interruptIDs[1] {
		t.Fatalf("resume retries used interrupt identities %q", interruptIDs)
	}
}

func countUniqueFrames(t *testing.T, output string) map[string]int {
	t.Helper()
	seen := make(map[string]bool)
	counts := make(map[string]int)
	for line := range strings.SplitSeq(strings.TrimSpace(output), "\n") {
		var frame struct {
			Type    string `json:"type"`
			EventID string `json:"eventId"`
		}
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Fatal(err)
		}
		if seen[frame.EventID] {
			t.Fatalf("event %s rendered twice:\n%s", frame.EventID, output)
		}
		seen[frame.EventID] = true
		counts[frame.Type]++
	}
	return counts
}

func TestRunRejectsConflictingReplay(t *testing.T) {
	rt := instant()
	rt.Script = shortCompletedScript
	rt.Faults = []mock.SubscriptionFault{{Kind: mock.FaultConflict, After: 1}}
	_, _, err := exec(t, rt, "", "run", "--output-format", "streaming-json", "-s", firstSession(t, rt), "conflict")
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
	out, _, err := exec(t, rt, "", "run", "--json", "-s", id, "ask me")
	if err == nil || !strings.Contains(err.Error(), "--session "+id) || strings.Contains(err.Error(), "<session-id>") {
		t.Fatalf("question error = %v", err)
	}
	result := decodeResult(t, out)
	if result.Status != "interrupted" || result.Interaction.Kind != "question" || result.Interaction.Title != "Choose a strategy" {
		t.Fatalf("interrupted result = %+v", result)
	}
	snapshot, getErr := rt.GetSession(t.Context(), id)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if snapshot.Active == nil || snapshot.Active.Status != client.RunWaiting {
		t.Fatalf("question did not leave a resumable waiting run: %+v", snapshot.Active)
	}
	if _, ok := snapshot.Events[len(snapshot.Events)-1].Event.(client.RunInterrupted); !ok {
		t.Fatalf("question was followed by an unexpected cleanup event: %+v", snapshot.Events[len(snapshot.Events)-1])
	}
}

func TestRunReturnsAnErrorForNonCompletedOutcomes(t *testing.T) {
	for _, test := range []struct {
		name    string
		outcome client.Outcome
		want    string
	}{
		{name: "failed", outcome: client.Outcome{Status: client.OutcomeFailed, Error: "provider refused"}, want: "run failed: provider refused"},
		{name: "canceled", outcome: client.Outcome{Status: client.OutcomeCanceled}, want: "run canceled"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := instant()
			runtime.Script = func(string) mock.Script {
				return mock.Script{Prelude: []mock.Step{{Event: client.RunFinished{Outcome: test.outcome}}}}
			}
			id := firstSession(t, runtime)
			out, _, err := exec(t, runtime, "", "run", "--json", "-s", id, "finish this way")
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
			if snapshot.Active != nil {
				t.Fatalf("settled outcome left an active run: %+v", snapshot.Active)
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
			runtime := instant()
			before, _ := runtime.ListSessions(t.Context(), client.SessionQuery{Limit: 100})
			if _, _, err := exec(t, runtime, "", args...); err == nil {
				t.Fatalf("arguments %v were accepted", args)
			}
			after, _ := runtime.ListSessions(t.Context(), client.SessionQuery{Limit: 100})
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

type alwaysDisconnected struct{ client.Runtime }

func (alwaysDisconnected) FollowRun(context.Context, client.FollowRun) (client.Stream, error) {
	return nil, fmt.Errorf("test transport: %w", client.ErrDisconnected)
}

func TestRunStopsAfterReconnectBudgetIsExhausted(t *testing.T) {
	rt := instant()
	rt.Script = shortCompletedScript
	out, _, err := exec(t, alwaysDisconnected{Runtime: rt}, "", "--reconnect-attempts", "2", "run", "--json", "-s", firstSession(t, rt), "offline")
	if !errors.Is(err, client.ErrDisconnected) {
		t.Fatalf("run error = %v, want ErrDisconnected", err)
	}
	result := decodeResult(t, out)
	if result.Status != "incomplete" || result.RunID == "" || result.SessionID == "" {
		t.Fatalf("incomplete result = %+v", result)
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
	writeCommandFixture(t, first, []byte("notes"))
	writeCommandFixture(t, second, append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...))
	runtime := instant()
	id := firstSession(t, runtime)
	if _, _, err := exec(t, runtime, "", "-C", workspace, "run", "--approve-all", "-s", id, "-f", "notes.txt", "-f", "diagram.png"); err != nil {
		t.Fatalf("attachment-only run: %v", err)
	}
	snapshot, err := runtime.GetSession(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	prompt := userPromptBlock(t, snapshot.Events)
	if prompt.Text != "" || len(prompt.Attachments) != 2 {
		t.Fatalf("prompt block = %+v", prompt)
	}
	if prompt.Attachments[0].Kind != client.AttachmentText || prompt.Attachments[1].Kind != client.AttachmentImage {
		t.Fatalf("attachment kinds = %+v", prompt.Attachments)
	}
}

func writeCommandFixture(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func userPromptBlock(t *testing.T, events []client.Envelope) client.Block {
	t.Helper()
	for _, envelope := range slices.Backward(events) {
		if event, ok := envelope.Event.(client.BlockCompleted); ok && event.Block.Kind == client.BlockUser {
			return event.Block
		}
	}
	t.Fatal("user prompt block was not emitted")
	return client.Block{}
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

func TestWorkspaceFlagIsNormalizedBeforeCreatingASession(t *testing.T) {
	runtime := instant()
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
	before, _ := runtime.ListSessions(t.Context(), client.SessionQuery{Limit: 100})
	if _, _, err := exec(t, runtime, "", "-C", relative, "run", "--approve-all", "normalize workspace"); err != nil {
		t.Fatal(err)
	}
	after, _ := runtime.ListSessions(t.Context(), client.SessionQuery{Limit: 100})
	if len(after.Items) != len(before.Items)+1 || after.Items[0].Workspace != want {
		t.Fatalf("newest session workspace = %q, want %q", after.Items[0].Workspace, want)
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
	requireSessionShow(t, runtime, id)
	requireSessionRename(t, runtime, id)
	forkID := forkTestSession(t, runtime, id)
	requireSessionDelete(t, runtime, forkID)
}

func requireSessionShow(t *testing.T, runtime client.Runtime, id string) {
	t.Helper()
	shown, _, err := exec(t, runtime, "", "sessions", "show", id)
	if err != nil || !strings.Contains(shown, "The fixed sleep races the janitor") {
		t.Fatalf("sessions show = %q, %v", shown, err)
	}
}

func requireSessionRename(t *testing.T, runtime client.Runtime, id string) {
	t.Helper()
	renamed, _, err := exec(t, runtime, "", "sessions", "rename", id, "Investigate cache")
	if err != nil || !strings.Contains(renamed, "Investigate cache") {
		t.Fatalf("sessions rename = %q, %v", renamed, err)
	}
}

func forkTestSession(t *testing.T, runtime client.Runtime, id string) string {
	t.Helper()
	forked, _, err := exec(t, runtime, "", "sessions", "fork", id, "--at", "4", "--title", "Alternative")
	if err != nil {
		t.Fatalf("sessions fork: %v", err)
	}
	forkID := strings.TrimSpace(forked)
	snapshot, err := runtime.GetSession(t.Context(), forkID)
	if err != nil || snapshot.Session.Title != "Alternative" || len(snapshot.Events) != 4 {
		t.Fatalf("fork snapshot = %+v, %v", snapshot, err)
	}
	return forkID
}

func requireSessionDelete(t *testing.T, runtime client.Runtime, id string) {
	t.Helper()
	if _, _, err := exec(t, runtime, "", "sessions", "delete", id); err == nil {
		t.Fatal("sessions delete did not require confirmation")
	}
	if _, _, err := exec(t, runtime, "", "sessions", "rm", "--yes", id); err != nil {
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

func TestSessionsListJSONKeepsPaginationOnStdout(t *testing.T) {
	runtime := instant()
	out, errOut, err := exec(t, runtime, "", "sessions", "ls", "--limit", "1", "--json")
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
	runtime := instant()
	sessionID := firstSession(t, runtime)
	ruleID := createProjectApprovalRule(t, runtime, sessionID)
	requireApprovalList(t, runtime)
	jsonOut, _, jsonErr := exec(t, runtime, "", "approvals", "ls", "--json")
	if jsonErr != nil {
		t.Fatal(jsonErr)
	}
	var rules approvalRulesJSON
	if err := json.Unmarshal([]byte(jsonOut), &rules); err != nil || len(rules.Rules) != 1 || rules.Rules[0].ID != ruleID {
		t.Fatalf("approval JSON = %+v, %v", rules, err)
	}
	out, _, err := exec(t, runtime, "", "__complete", "approvals", "delete", "")
	if err != nil || !strings.Contains(out, ruleID) {
		t.Fatalf("approval completion = %q, %v", out, err)
	}
	if _, _, err := exec(t, runtime, "", "approvals", "forget", ruleID); err == nil {
		t.Fatal("approvals forget did not require --yes")
	}
	if _, _, err := exec(t, runtime, "", "approvals", "forget", "--yes", ruleID); err != nil {
		t.Fatal(err)
	}
}

func createProjectApprovalRule(t *testing.T, runtime client.Runtime, sessionID string) string {
	t.Helper()
	run, err := runtime.StartRun(t.Context(), client.StartRun{SessionID: sessionID, Message: client.Message{Text: "remember this"}})
	if err != nil {
		t.Fatal(err)
	}
	interrupted, after := followApprovalInterrupt(t, runtime, run)
	resumeProjectApproval(t, runtime, run.ID, interrupted)
	drainContinuation(t, runtime, run.ID, after)
	return onlyApprovalRule(t, runtime).ID
}

func followApprovalInterrupt(t *testing.T, runtime client.Runtime, run client.Run) (client.Approval, client.Cursor) {
	t.Helper()
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
	if interrupted.InterruptID == "" {
		t.Fatal("run did not request approval")
	}
	return interrupted, after
}

func resumeProjectApproval(t *testing.T, runtime client.Runtime, runID string, interrupted client.Approval) {
	t.Helper()
	if err := runtime.ResumeRun(t.Context(), client.ResumeRun{
		RunID: runID, InterruptID: interrupted.InterruptID,
		Answer: client.ApprovalAnswer{Decision: client.ApprovalAllow, Remember: client.RememberProject},
	}); err != nil {
		t.Fatal(err)
	}
}

func drainContinuation(t *testing.T, runtime client.Runtime, runID string, after client.Cursor) {
	t.Helper()
	continuation, err := runtime.FollowRun(t.Context(), client.FollowRun{RunID: runID, After: after})
	if err != nil {
		t.Fatal(err)
	}
	for _, streamErr := range continuation {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
	}
}

func onlyApprovalRule(t *testing.T, runtime client.Runtime) client.ApprovalRule {
	t.Helper()
	rules, err := runtime.ListApprovalRules(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("approval rules = %+v, want one", rules)
	}
	return rules[0]
}

func requireApprovalList(t *testing.T, runtime client.Runtime) {
	t.Helper()
	out, _, err := exec(t, runtime, "", "approvals", "ls")
	if err != nil || !strings.Contains(out, "edit:internal/store/cache_test.go") || !strings.Contains(out, "project") {
		t.Fatalf("approvals ls = %q, %v", out, err)
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
	root := buildRoot(runtimeProvider{factory: func(context.Context) (client.Runtime, error) {
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
