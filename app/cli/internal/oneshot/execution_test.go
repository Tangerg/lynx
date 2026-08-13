package oneshot

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
)

type invalidOpeningRuntime struct{ *mock.Runtime }

type refusingCancellationRuntime struct {
	*mock.Runtime

	failure  error
	mu       sync.Mutex
	attempts []agent.CancelRun
}

type misdirectedCancellationRuntime struct{ *mock.Runtime }

type uncertainAcknowledgementRuntime struct {
	*mock.Runtime

	mu             sync.Mutex
	startAttempts  []agent.StartRun
	resumeAttempts []agent.ResumeRun
	cancelAttempts []agent.CancelRun
	startStream    agent.SegmentStream
	resumeStream   agent.SegmentStream
	cancelResult   agent.RunCancellation
}

func (runtime *uncertainAcknowledgementRuntime) StartRun(ctx context.Context, input agent.StartRun) (agent.SegmentStream, error) {
	runtime.mu.Lock()
	runtime.startAttempts = append(runtime.startAttempts, input.Clone())
	attempt := len(runtime.startAttempts)
	cached := runtime.startStream
	runtime.mu.Unlock()
	if attempt > 1 {
		return cached, nil
	}
	opened, err := runtime.Runtime.StartRun(ctx, input)
	if err != nil {
		return agent.SegmentStream{}, err
	}
	runtime.mu.Lock()
	runtime.startStream = opened
	runtime.mu.Unlock()
	return agent.SegmentStream{}, fmt.Errorf("start acknowledgement timed out: %w", context.DeadlineExceeded)
}

func (runtime *uncertainAcknowledgementRuntime) ResumeRun(ctx context.Context, input agent.ResumeRun) (agent.SegmentStream, error) {
	runtime.mu.Lock()
	runtime.resumeAttempts = append(runtime.resumeAttempts, input.Clone())
	attempt := len(runtime.resumeAttempts)
	cached := runtime.resumeStream
	runtime.mu.Unlock()
	if attempt > 1 {
		return cached, nil
	}
	continued, err := runtime.Runtime.ResumeRun(ctx, input)
	if err != nil {
		return agent.SegmentStream{}, err
	}
	runtime.mu.Lock()
	runtime.resumeStream = continued
	runtime.mu.Unlock()
	return agent.SegmentStream{}, fmt.Errorf("resume acknowledgement timed out: %w", context.DeadlineExceeded)
}

func (runtime *uncertainAcknowledgementRuntime) CancelRun(
	ctx context.Context,
	input agent.CancelRun,
) (agent.RunCancellation, error) {
	runtime.mu.Lock()
	runtime.cancelAttempts = append(runtime.cancelAttempts, input)
	attempt := len(runtime.cancelAttempts)
	cached := runtime.cancelResult
	runtime.mu.Unlock()
	if attempt > 1 {
		return cached, nil
	}
	result, err := runtime.Runtime.CancelRun(ctx, input)
	if err != nil {
		return agent.RunCancellation{}, err
	}
	runtime.mu.Lock()
	runtime.cancelResult = result
	runtime.mu.Unlock()
	return agent.RunCancellation{}, fmt.Errorf("cancel acknowledgement timed out: %w", context.DeadlineExceeded)
}

func (runtime *uncertainAcknowledgementRuntime) attempts() ([]agent.StartRun, []agent.ResumeRun) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	starts := make([]agent.StartRun, len(runtime.startAttempts))
	for index, attempt := range runtime.startAttempts {
		starts[index] = attempt.Clone()
	}
	resumes := make([]agent.ResumeRun, len(runtime.resumeAttempts))
	for index, attempt := range runtime.resumeAttempts {
		resumes[index] = attempt.Clone()
	}
	return starts, resumes
}

func (runtime *uncertainAcknowledgementRuntime) cancellationAttempts() []agent.CancelRun {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return slices.Clone(runtime.cancelAttempts)
}

func (runtime invalidOpeningRuntime) StartRun(ctx context.Context, input agent.StartRun) (agent.SegmentStream, error) {
	opened, err := runtime.Runtime.StartRun(ctx, input)
	if err == nil {
		opened.Events = nil
	}
	return opened, err
}

func (runtime *refusingCancellationRuntime) CancelRun(
	_ context.Context,
	input agent.CancelRun,
) (agent.RunCancellation, error) {
	runtime.mu.Lock()
	runtime.attempts = append(runtime.attempts, input)
	runtime.mu.Unlock()
	return agent.RunCancellation{}, runtime.failure
}

func (runtime *refusingCancellationRuntime) cancellationAttempts() []agent.CancelRun {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return slices.Clone(runtime.attempts)
}

func (runtime misdirectedCancellationRuntime) CancelRun(
	ctx context.Context,
	input agent.CancelRun,
) (agent.RunCancellation, error) {
	result, err := runtime.Runtime.CancelRun(ctx, input)
	if err == nil {
		result.Canceled.ID = "run_misdirected"
		result.Root.ID = "run_misdirected"
	}
	return result, err
}

type recordingRenderer struct {
	events []agent.RunEvent
	err    error
}

func (r *recordingRenderer) Begin(agent.Run, agent.RunOptions) error { return r.err }

func (r *recordingRenderer) Render(event agent.RunEvent) error {
	if r.err != nil {
		return r.err
	}
	r.events = append(r.events, event.Clone())
	return nil
}

func (r *recordingRenderer) Reconcile(snapshot agent.SessionSnapshot) error {
	if r.err != nil {
		return r.err
	}
	for _, block := range snapshot.Transcript {
		r.events = append(r.events, agent.RunEvent{EventID: "snapshot:" + block.ID, RunID: "snapshot", SegmentID: "snapshot", Event: agent.BlockCompleted{Block: block.Clone()}})
	}
	return nil
}

func (*recordingRenderer) Close() error { return nil }

func TestExecuteDrivesApprovalAcrossSegments(t *testing.T) {
	runtime := mock.New()
	runtime.Instant = true
	session, _ := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: t.TempDir()})
	renderer := new(recordingRenderer)
	err := Execute(t.Context(), Invocation{
		Runtime: runtime, Renderer: renderer,
		Start:      agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "fix it"}},
		ApproveAll: true, ReconnectAttempts: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	segments := make(map[string]struct{})
	for _, event := range renderer.events {
		segments[event.SegmentID] = struct{}{}
	}
	if len(segments) != 2 {
		t.Fatalf("segments = %v", segments)
	}
}

func TestExecuteConfirmsTimedOutMutationsWithoutChangingIdentity(t *testing.T) {
	base := mock.New()
	base.Instant = true
	runtime := &uncertainAcknowledgementRuntime{Runtime: base}
	session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	err = Execute(t.Context(), Invocation{
		Runtime: runtime, Renderer: new(recordingRenderer),
		Start:      agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "confirm every mutation"}},
		ApproveAll: true,
		// Acknowledgement confirmation is not a stream reconnect budget.
		ReconnectAttempts: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	starts, resumes := runtime.attempts()
	if len(starts) != 2 || starts[0].CommandID == "" || starts[0].CommandID != starts[1].CommandID {
		t.Fatalf("start confirmation attempts = %+v", starts)
	}
	if len(resumes) != 2 || resumes[0].CommandID == "" || resumes[0].CommandID != resumes[1].CommandID {
		t.Fatalf("resume confirmation attempts = %+v", resumes)
	}
}

func TestExecuteLeavesQuestionsParked(t *testing.T) {
	runtime := mock.New()
	runtime.Instant = true
	runtime.Script = func(string) mock.Script {
		return mock.Script{
			Interactions: []agent.Interaction{agent.Question{
				ItemID: "q_1", Title: "Target",
				Fields: []agent.QuestionField{{Prompt: "Target", Kind: agent.QuestionSingle, Options: []agent.QuestionOption{{Label: "linux"}, {Label: "darwin"}}}},
			}},
			Continue: func([]agent.InterruptAnswer) []mock.Step {
				return []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}
			},
		}
	}
	session, _ := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: t.TempDir()})
	err := Execute(t.Context(), Invocation{
		Runtime: runtime, Renderer: new(recordingRenderer),
		Start: agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "ask"}},
	})
	if _, ok := errors.AsType[*interactionRequiredError](err); !ok {
		t.Fatalf("error = %v", err)
	}
	snapshot, snapshotErr := runtime.GetSession(t.Context(), session.ID)
	active, activeOK := snapshot.ActiveRun()
	if snapshotErr != nil || !activeOK || active.Status != agent.RunStatusWaiting {
		t.Fatalf("snapshot = %+v, %v", snapshot, snapshotErr)
	}
}

func TestExecuteReconnectsOnlyTheCurrentSegment(t *testing.T) {
	runtime := mock.New()
	runtime.Faults = []mock.SubscriptionFault{{Kind: mock.FaultDisconnect, After: 1}}
	runtime.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Delay: 30 * time.Millisecond, Event: agent.BlockCompleted{Block: agent.Block{ID: "answer", Kind: agent.BlockAssistant, Text: "done"}}},
			{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	session, _ := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: t.TempDir()})
	err := Execute(t.Context(), Invocation{
		Runtime: runtime, Renderer: new(recordingRenderer),
		Start:             agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "fix"}},
		ReconnectAttempts: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestExecutePropagatesRendererFailureAndCancelsRun(t *testing.T) {
	runtime := mock.New()
	runtime.Instant = true
	session, _ := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: t.TempDir()})
	want := errors.New("write failed")
	err := Execute(t.Context(), Invocation{
		Runtime: runtime, Renderer: &recordingRenderer{err: want},
		Start: agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "fix"}},
	})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v", err)
	}
}

func TestExecuteReportsAbandonedRunCancellationFailure(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{
			Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}},
		}}}
	}
	cleanupFailure := errors.New("cancellation refused")
	runtime := &refusingCancellationRuntime{Runtime: base, failure: cleanupFailure}
	session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	renderFailure := errors.New("output unavailable")
	err = Execute(t.Context(), Invocation{
		Runtime: runtime, Renderer: &recordingRenderer{err: renderFailure},
		Start: agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "fail and clean up"}},
	})
	if !errors.Is(err, renderFailure) || !errors.Is(err, cleanupFailure) {
		t.Fatalf("joined execution failure = %v", err)
	}
	attempts := runtime.cancellationAttempts()
	if len(attempts) != 1 || attempts[0].CommandID == "" || attempts[0].RunID == "" ||
		attempts[0].Reason != "CLI execution ended before the run settled" {
		t.Fatalf("abandoned run cleanup attempts = %+v", attempts)
	}
	commandID, commandErr := agent.NewCommandID()
	if commandErr != nil {
		t.Fatal(commandErr)
	}
	if _, cancelErr := base.CancelRun(t.Context(), agent.CancelRun{
		CommandID: commandID, RunID: attempts[0].RunID, Reason: "test cleanup",
	}); cancelErr != nil {
		t.Fatal(cancelErr)
	}
}

func TestExecuteConfirmsTimedOutCleanupWithoutChangingIdentity(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{
			Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}},
		}}}
	}
	runtime := &uncertainAcknowledgementRuntime{Runtime: base}
	session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	renderFailure := errors.New("output unavailable")
	err = Execute(t.Context(), Invocation{
		Runtime: runtime, Renderer: &recordingRenderer{err: renderFailure},
		Start: agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "confirm cleanup"}},
	})
	if !errors.Is(err, renderFailure) {
		t.Fatalf("execution error = %v", err)
	}
	attempts := runtime.cancellationAttempts()
	if len(attempts) != 2 || attempts[0].CommandID == "" || attempts[0].CommandID != attempts[1].CommandID ||
		attempts[0].RunID != attempts[1].RunID {
		t.Fatalf("cleanup confirmation attempts = %+v", attempts)
	}
}

func TestExecuteRejectsAMisdirectedAbandonedRunCancellation(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{
			Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}},
		}}}
	}
	session, err := base.CreateSession(t.Context(), agent.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	renderFailure := errors.New("output unavailable")
	err = Execute(t.Context(), Invocation{
		Runtime: misdirectedCancellationRuntime{Runtime: base}, Renderer: &recordingRenderer{err: renderFailure},
		Start: agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "validate cleanup receipt"}},
	})
	if !errors.Is(err, renderFailure) || !strings.Contains(err.Error(), "cancel abandoned run") ||
		!strings.Contains(err.Error(), "run_misdirected") {
		t.Fatalf("misdirected cleanup failure = %v", err)
	}
}

func TestExecuteCancelsARunWhoseOpeningStreamIsInvalid(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}}
	}
	session, _ := base.CreateSession(t.Context(), agent.CreateSession{Workspace: t.TempDir()})
	err := Execute(t.Context(), Invocation{
		Runtime: invalidOpeningRuntime{Runtime: base}, Renderer: new(recordingRenderer),
		Start: agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "start"}},
	})
	if err == nil {
		t.Fatal("invalid opening stream was accepted")
	}
	snapshot, snapshotErr := base.GetSession(t.Context(), session.ID)
	if snapshotErr != nil {
		t.Fatal(snapshotErr)
	}
	latest, ok := snapshot.LatestRun()
	if !ok || latest.Status != agent.RunStatusFinished || latest.Outcome.Status != agent.OutcomeCanceled {
		t.Fatalf("invalid opening left run active: %+v", snapshot.Runs)
	}
}
