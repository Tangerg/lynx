package terminal

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
	"github.com/Tangerg/lynx/app/cli/internal/workbench"
)

type sessionReadFailureRuntime struct {
	*mock.Runtime
	reads     atomic.Int32
	failureAt int32
}

type replayingStartRuntime struct {
	*mock.Runtime

	mu       sync.Mutex
	attempts int
	inputs   []agent.StartRun
	stream   agent.SegmentStream
}

type idempotentStartRuntime struct {
	*mock.Runtime

	mu       sync.Mutex
	receipts map[agent.CommandID]agent.SegmentStream
	inputs   []agent.StartRun
}

type activeConflictRuntime struct {
	agent.Runtime
	attempted chan agent.StartRun
	conflict  agent.CommandID
}

type refusingFirstCommandRuntime struct {
	*mock.Runtime

	mu      sync.Mutex
	refused agent.CommandID
	inputs  []agent.StartRun
}

func (runtime *refusingFirstCommandRuntime) StartRun(ctx context.Context, input agent.StartRun) (agent.SegmentStream, error) {
	runtime.mu.Lock()
	if runtime.refused == "" {
		runtime.refused = input.CommandID
	}
	runtime.inputs = append(runtime.inputs, input.Clone())
	refused := runtime.refused
	runtime.mu.Unlock()
	if input.CommandID == refused {
		return agent.SegmentStream{}, fmt.Errorf("runtime refused start: %w", agent.ErrSessionHasActiveRun)
	}
	return runtime.Runtime.StartRun(ctx, input)
}

func (runtime *refusingFirstCommandRuntime) refusedCommand() agent.StartRun {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if len(runtime.inputs) == 0 {
		return agent.StartRun{}
	}
	return runtime.inputs[0].Clone()
}

func (runtime *activeConflictRuntime) StartRun(ctx context.Context, input agent.StartRun) (agent.SegmentStream, error) {
	if input.CommandID != runtime.conflict {
		return runtime.Runtime.StartRun(ctx, input)
	}
	select {
	case runtime.attempted <- input.Clone():
	default:
	}
	return agent.SegmentStream{}, fmt.Errorf("active run owns session: %w", agent.ErrSessionHasActiveRun)
}

func (runtime *idempotentStartRuntime) StartRun(ctx context.Context, input agent.StartRun) (agent.SegmentStream, error) {
	runtime.mu.Lock()
	runtime.inputs = append(runtime.inputs, input.Clone())
	opened, replay := runtime.receipts[input.CommandID]
	runtime.mu.Unlock()
	if replay {
		return opened, nil
	}
	opened, err := runtime.Runtime.StartRun(ctx, input)
	if err != nil {
		return agent.SegmentStream{}, err
	}
	runtime.mu.Lock()
	if runtime.receipts == nil {
		runtime.receipts = make(map[agent.CommandID]agent.SegmentStream)
	}
	runtime.receipts[input.CommandID] = opened
	runtime.mu.Unlock()
	return opened, nil
}

func (runtime *idempotentStartRuntime) attempts() []agent.StartRun {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	out := make([]agent.StartRun, len(runtime.inputs))
	for index, input := range runtime.inputs {
		out[index] = input.Clone()
	}
	return out
}

func (runtime *replayingStartRuntime) StartRun(ctx context.Context, input agent.StartRun) (agent.SegmentStream, error) {
	runtime.mu.Lock()
	runtime.attempts++
	runtime.inputs = append(runtime.inputs, cloneStartRun(input))
	attempt := runtime.attempts
	cached := runtime.stream
	runtime.mu.Unlock()
	if attempt == 1 {
		opened, err := runtime.Runtime.StartRun(ctx, input)
		if err != nil {
			return agent.SegmentStream{}, err
		}
		runtime.mu.Lock()
		runtime.stream = opened
		runtime.mu.Unlock()
		return agent.SegmentStream{}, fmt.Errorf("lost start acknowledgement: %w", agent.ErrDisconnected)
	}
	return cached, nil
}

func (runtime *replayingStartRuntime) startAttempts() []agent.StartRun {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	out := make([]agent.StartRun, len(runtime.inputs))
	for index, input := range runtime.inputs {
		out[index] = cloneStartRun(input)
	}
	return out
}

func (runtime *sessionReadFailureRuntime) GetSession(ctx context.Context, sessionID string) (agent.SessionSnapshot, error) {
	if runtime.reads.Add(1) == runtime.failureAt {
		return agent.SessionSnapshot{}, agent.ErrDisconnected
	}
	return runtime.Runtime.GetSession(ctx, sessionID)
}

func TestRecoveredSessionRetriesATransientAttachRead(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{Delay: time.Hour, Event: agent.RunFinished{
			Outcome: agent.Outcome{Status: agent.OutcomeCompleted},
		}}}}
	}
	_, err := base.StartRun(t.Context(), agent.StartRun{
		SessionID: "ses_demo_1", Message: agent.Message{Text: "recover attach"},
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := &sessionReadFailureRuntime{Runtime: base, failureAt: 2}
	host, stop := runUIWithRuntimeChanges(t, runtime, nil, "ses_demo_1")
	host.Until(t, "the recovered session attach retry", func() bool {
		return runtime.reads.Load() >= 4 && host.Repaint()
	})
	host.Shows(t, "recover attach")
	stop()
}

func TestStartHandshakeRetriesTheSameMutationIdentity(t *testing.T) {
	base := mock.New()
	base.Script = stableCompletedScript
	runtime := &replayingStartRuntime{Runtime: base}
	host, stop := runUIWith(t, runtime)
	host.Shows(t, "Ask lyra")
	host.Type("retry one start")
	host.Press(input.Enter)
	host.Shows(t, "stable answer")
	host.Shows(t, "complete")

	attempts := runtime.startAttempts()
	if len(attempts) != 2 {
		t.Fatalf("start attempts = %d, want 2", len(attempts))
	}
	if attempts[0].CommandID == "" || attempts[0].CommandID != attempts[1].CommandID {
		t.Fatalf("start command identities = %q, %q", attempts[0].CommandID, attempts[1].CommandID)
	}
	if !attempts[0].Message.Equal(attempts[1].Message) {
		t.Fatalf("start retry changed message: %+v, %+v", attempts[0].Message, attempts[1].Message)
	}
	stop()
}

func TestDefinitivelyRefusedStartReturnsToTheDurableQueueWithANewIdentity(t *testing.T) {
	base := mock.New()
	base.Instant = true
	runtime := &refusingFirstCommandRuntime{Runtime: base}
	stateDirectory := t.TempDir()
	host, stop := runUIFromConfig(t, Config{
		Runtime: runtime, Workspace: "/tmp/lyra-cli-test", StateDirectory: stateDirectory,
	})
	host.Shows(t, "Ask lyra")
	host.Type("preserve a refused start")
	host.Press(input.Enter)
	host.Shows(t, "runtime refused start")
	host.Shows(t, "1 queued")

	var pending []workbench.PendingRun
	var refused agent.StartRun
	host.Until(t, "the refused start to return to the durable FIFO", func() bool {
		refused = runtime.refusedCommand()
		if refused.SessionID == "" {
			return false
		}
		store, err := workbench.Open(stateDirectory, workbench.Config{})
		if err != nil {
			return false
		}
		pending = store.PendingRuns(refused.SessionID)
		return host.Repaint() && len(pending) == 1 && pending[0].State == workbench.PendingRunQueued
	})
	if refused.CommandID == "" || len(pending) != 1 || pending[0].Command.CommandID == refused.CommandID ||
		pending[0].Command.Message.Text != "preserve a refused start" {
		t.Fatalf("requeued command = %+v, refused command = %+v", pending, refused)
	}

	stop()
}

func TestLaunchReplaysADispatchingRunFromTheDurableOutbox(t *testing.T) {
	base := mock.New()
	base.Instant = true
	base.Script = stableCompletedScript
	stateDirectory := t.TempDir()
	store, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	command := agent.StartRun{
		CommandID: agent.CommandID("cli_0123456789abcdef0123456789abcdef"),
		SessionID: "ses_demo_1", Message: agent.Message{Text: "replay after launch"},
	}
	if err := store.StagePendingRun(workbench.PendingRun{State: workbench.PendingRunDispatching, Command: command}); err != nil {
		t.Fatal(err)
	}
	runtime := &recordingRuntime{Runtime: base}
	host, stop := runUIWithState(t, runtime, "/tmp/lyra-cli-test", command.SessionID, stateDirectory)
	host.Shows(t, "stable answer")
	host.Shows(t, "complete")
	if started := runtime.startInput(); started.CommandID != command.CommandID || started.Message.Text != command.Message.Text {
		t.Fatalf("replayed start = %+v, want %+v", started, command)
	}
	reopened, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if pending := reopened.PendingRuns(command.SessionID); len(pending) != 0 {
		t.Fatalf("acknowledged outbox = %+v", pending)
	}
	stop()
}

func TestLaunchDoesNotReplayAnOutboxCommandAlreadyVisibleInRuntime(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}}
	}
	command := agent.StartRun{
		CommandID: agent.CommandID("cli_abcdef0123456789abcdef0123456789"),
		SessionID: "ses_demo_1", Message: agent.Message{Text: "already accepted"},
	}
	runtime := &idempotentStartRuntime{Runtime: base}
	if _, err := runtime.StartRun(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	stateDirectory := t.TempDir()
	store, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StagePendingRun(workbench.PendingRun{State: workbench.PendingRunDispatching, Command: command}); err != nil {
		t.Fatal(err)
	}
	host, stop := runUIWithState(t, runtime, "/tmp/lyra-cli-test", command.SessionID, stateDirectory)
	host.Shows(t, "already accepted")
	host.Shows(t, "reconnected")
	attempts := runtime.attempts()
	if len(attempts) != 2 || attempts[0].CommandID != attempts[1].CommandID {
		t.Fatalf("launch reconciliation attempts = %+v", attempts)
	}
	reopened, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if pending := reopened.PendingRuns(command.SessionID); len(pending) != 0 {
		t.Fatalf("reconciled outbox = %+v", pending)
	}
	stop()
}

func TestLaunchRequeuesARejectedHandshakeBehindAnotherActiveRun(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{Delay: time.Hour, Event: agent.RunFinished{
			Outcome: agent.Outcome{Status: agent.OutcomeCompleted},
		}}}}
	}
	active := agent.StartRun{SessionID: "ses_demo_1", Message: agent.Message{Text: "already active"}}
	if _, err := base.StartRun(t.Context(), active); err != nil {
		t.Fatal(err)
	}
	stateDirectory := t.TempDir()
	store, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	original := agent.CommandID("cli_22222222222222222222222222222222")
	command := agent.StartRun{
		CommandID: original, SessionID: active.SessionID, Message: agent.Message{Text: "queue after recovery"},
	}
	if err := store.StagePendingRun(workbench.PendingRun{State: workbench.PendingRunDispatching, Command: command}); err != nil {
		t.Fatal(err)
	}
	runtime := &activeConflictRuntime{Runtime: base, attempted: make(chan agent.StartRun, 1), conflict: original}
	host, stop := runUIWithState(t, runtime, "/tmp/lyra-cli-test", command.SessionID, stateDirectory)
	host.Shows(t, "already active")
	host.Shows(t, "1 queued")
	select {
	case attempted := <-runtime.attempted:
		if attempted.CommandID != original {
			t.Fatalf("reconciliation command = %+v", attempted)
		}
	case <-t.Context().Done():
		t.Fatal("reconciliation did not retry the original command")
	}
	var pending []workbench.PendingRun
	host.Until(t, "the refused command to become an ordinary queued intent", func() bool {
		reopened, openErr := workbench.Open(stateDirectory, workbench.Config{})
		if openErr != nil {
			return false
		}
		pending = reopened.PendingRuns(command.SessionID)
		return host.Repaint() && len(pending) == 1 && pending[0].State == workbench.PendingRunQueued
	})
	if len(pending) != 1 || pending[0].State != workbench.PendingRunQueued ||
		pending[0].Command.CommandID == original || pending[0].Command.Message.Text != command.Message.Text {
		t.Fatalf("requeued launch command = %+v", pending)
	}
	stop()
}

func TestLaunchFinishesCancellationOfAnUnconfirmedRunStart(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{Delay: time.Hour, Event: agent.RunFinished{
			Outcome: agent.Outcome{Status: agent.OutcomeCompleted},
		}}}}
	}
	command := agent.StartRun{
		CommandID: agent.CommandID("cli_77777777777777777777777777777777"),
		SessionID: "ses_demo_1", Message: agent.Message{Text: "cancel after restart"},
	}
	runtime := &idempotentStartRuntime{Runtime: base}
	if _, err := runtime.StartRun(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	stateDirectory := t.TempDir()
	store, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StagePendingRun(workbench.PendingRun{
		State: workbench.PendingRunDispatching, Command: command,
	}); err != nil {
		t.Fatal(err)
	}
	cancelID, err := store.MarkPendingRunCanceling(command.SessionID, command.CommandID)
	if err != nil {
		t.Fatal(err)
	}

	host, stop := runUIWithState(t, runtime, "/tmp/lyra-cli-test", command.SessionID, stateDirectory)
	host.Shows(t, "canceled")
	host.Until(t, "the canceled opening command to leave the durable outbox", func() bool {
		reopened, openErr := workbench.Open(stateDirectory, workbench.Config{})
		return openErr == nil && len(reopened.PendingRuns(command.SessionID)) == 0 && host.Repaint()
	})
	if cancelID == "" {
		t.Fatal("cancel command identity is empty")
	}
	stop()
}

func TestActiveDurationClockStartsFromDurableExecutionTime(t *testing.T) {
	startedAt := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	clock := activeDurationClock{}
	if got := clock.elapsed(startedAt); got != 0 {
		t.Fatalf("zero clock elapsed = %v, want zero", got)
	}

	clock.start(1400*time.Millisecond, startedAt)
	if got := clock.elapsed(startedAt.Add(600 * time.Millisecond)); got != 2*time.Second {
		t.Fatalf("resumed elapsed = %v, want 2s", got)
	}
	if got := clock.elapsed(startedAt.Add(-time.Second)); got != 1400*time.Millisecond {
		t.Fatalf("clock before local segment start = %v, want carried duration", got)
	}
}

func TestActiveDurationClockExcludesHumanWaitBetweenSegments(t *testing.T) {
	firstSegment := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	resume := firstSegment.Add(24 * time.Hour)
	clock := activeDurationClock{}
	clock.start(0, firstSegment)
	clock.start(3*time.Second, resume)

	if got := clock.elapsed(resume.Add(time.Second)); got != 4*time.Second {
		t.Fatalf("elapsed after overnight wait = %v, want 4s active time", got)
	}
}
