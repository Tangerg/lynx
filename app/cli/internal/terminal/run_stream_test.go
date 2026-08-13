package terminal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
	"github.com/Tangerg/lynx/app/cli/internal/mutation"
	"github.com/Tangerg/lynx/app/cli/internal/workbench"
)

type sessionReadFailureRuntime struct {
	*mock.Runtime
	reads     atomic.Int32
	failureAt int32
}

type replayingStartRuntime struct {
	*mock.Runtime

	mu         sync.Mutex
	attempts   int
	inputs     []agent.StartRun
	stream     agent.SegmentStream
	failure    error
	afterFirst func()
}

type idempotentStartRuntime struct {
	*mock.Runtime

	mu       sync.Mutex
	receipts map[agent.CommandID]agent.SegmentStream
	inputs   []agent.StartRun
}

type heldCancellationResultRuntime struct {
	*idempotentStartRuntime
	settled chan struct{}
	release chan struct{}
}

func (runtime *heldCancellationResultRuntime) CancelRun(
	ctx context.Context,
	input agent.CancelRun,
) (agent.RunCancellation, error) {
	result, err := runtime.Runtime.CancelRun(ctx, input)
	select {
	case runtime.settled <- struct{}{}:
	default:
	}
	select {
	case <-runtime.release:
		return result, err
	case <-ctx.Done():
		return agent.RunCancellation{}, context.Cause(ctx)
	}
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

type invalidAcceptedStartRuntime struct {
	agent.Runtime

	mu            sync.Mutex
	starts        []agent.StartRun
	cancellations []agent.CancelRun
	refuseFirst   bool
}

func (runtime *invalidAcceptedStartRuntime) StartRun(ctx context.Context, input agent.StartRun) (agent.SegmentStream, error) {
	opened, err := runtime.Runtime.StartRun(ctx, input)
	if err != nil {
		return agent.SegmentStream{}, err
	}
	runtime.mu.Lock()
	runtime.starts = append(runtime.starts, input.Clone())
	runtime.mu.Unlock()
	opened.UserItemID = ""
	return agent.SegmentStream{}, agent.NewAcceptedMutationError(
		opened, fmt.Errorf("start run: %w", opened.ValidateStart()),
	)
}

func (runtime *invalidAcceptedStartRuntime) CancelRun(ctx context.Context, input agent.CancelRun) (agent.RunCancellation, error) {
	runtime.mu.Lock()
	runtime.cancellations = append(runtime.cancellations, input)
	refuse := runtime.refuseFirst && len(runtime.cancellations) == 1
	runtime.mu.Unlock()
	if refuse {
		return agent.RunCancellation{}, errors.New("temporary malformed-receipt cleanup failure")
	}
	return runtime.Runtime.CancelRun(ctx, input)
}

func (runtime *invalidAcceptedStartRuntime) attempts() ([]agent.StartRun, []agent.CancelRun) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	starts := make([]agent.StartRun, len(runtime.starts))
	for index, input := range runtime.starts {
		starts[index] = input.Clone()
	}
	return starts, slices.Clone(runtime.cancellations)
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
		if runtime.afterFirst != nil {
			runtime.afterFirst()
		}
		failure := runtime.failure
		if failure == nil {
			failure = agent.ErrDisconnected
		}
		return agent.SegmentStream{}, fmt.Errorf("lost start acknowledgement: %w", failure)
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

func TestStartHandshakeTreatsDeadlineAsAnUnknownAcknowledgement(t *testing.T) {
	base := mock.New()
	base.Script = stableCompletedScript
	runtime := &replayingStartRuntime{Runtime: base, failure: context.DeadlineExceeded}
	host, stop := runUIWith(t, runtime)
	host.Shows(t, "Ask lyra")
	host.Type("recover a timed out acknowledgement")
	host.Press(input.Enter)
	host.Shows(t, "stable answer")
	host.Shows(t, "complete")
	host.Hides(t, "1 queued")

	attempts := runtime.startAttempts()
	if len(attempts) != 2 || attempts[0].CommandID == "" || attempts[0].CommandID != attempts[1].CommandID {
		t.Fatalf("deadline recovery attempts = %+v", attempts)
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

func TestInvalidAcceptedStartReceiptCancelsAndSettlesTheExactMutation(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{
			Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}},
		}}}
	}
	runtime := &invalidAcceptedStartRuntime{Runtime: base}
	stateDirectory := t.TempDir()
	host, stop := runUIWithState(t, runtime, "/tmp/lyra-cli-test", "ses_demo_1", stateDirectory)
	host.Shows(t, "Ask lyra")
	host.Type("cancel malformed accepted start")
	host.Press(input.Enter)
	host.Shows(t, "start segment stream: user item id is empty")
	host.Until(t, "the malformed accepted start to be canceled", func() bool {
		starts, cancellations := runtime.attempts()
		if len(starts) != 1 || len(cancellations) != 1 {
			return false
		}
		reopened, err := workbench.Open(stateDirectory, workbench.Config{})
		return err == nil && len(reopened.PendingRuns(starts[0].SessionID)) == 0 && host.Repaint()
	})
	starts, cancellations := runtime.attempts()
	if starts[0].CommandID == "" || cancellations[0].CommandID == "" || cancellations[0].RunID == "" ||
		cancellations[0].Reason != "runtime returned an invalid start receipt" {
		t.Fatalf("malformed receipt cleanup = starts %+v, cancellations %+v", starts, cancellations)
	}
	reopened, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if pending := reopened.PendingRuns(starts[0].SessionID); len(pending) != 0 {
		t.Fatalf("canceled malformed start remains durable: %+v", pending)
	}
	history := reopened.History()
	if len(history) != 1 || !history[0].Equal(starts[0].Message) {
		t.Fatalf("canceled accepted start history = %+v", history)
	}
	snapshot, err := base.GetSession(t.Context(), starts[0].SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, active := snapshot.ActiveRun(); active {
		t.Fatalf("malformed accepted receipt left a runtime run active: %+v", snapshot.Runs)
	}
	stop()
}

func TestInvalidAcceptedStartReceiptSettlesTheMemoryOnlyQueue(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{
			Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}},
		}}}
	}
	runtime := &invalidAcceptedStartRuntime{Runtime: base}
	host, stop := runUIWith(t, runtime)
	host.Shows(t, "Ask lyra")
	host.Type("settle malformed start without files")
	host.Press(input.Enter)
	host.Shows(t, "start segment stream: user item id is empty")
	host.Until(t, "the memory-only malformed start cleanup", func() bool {
		starts, cancellations := runtime.attempts()
		return len(starts) == 1 && len(cancellations) == 1 && host.Repaint()
	})
	host.Hides(t, "1 queued")
	stop()
}

func TestRetryingInvalidAcceptedStartCleanupPreservesIdentityAndFailurePolicy(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{
			Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}},
		}}}
	}
	runtime := &invalidAcceptedStartRuntime{Runtime: base, refuseFirst: true}
	stateDirectory := t.TempDir()
	host, stop := runUIWithState(t, runtime, "/tmp/lyra-cli-test", "ses_demo_1", stateDirectory)
	host.Shows(t, "Ask lyra")
	host.Type("retry malformed accepted start cleanup")
	host.Press(input.Enter)
	host.Shows(t, "could not cancel run: temporary malformed-receipt cleanup failure")
	host.Press(input.Esc)
	host.Until(t, "the retried malformed receipt cleanup", func() bool {
		starts, cancellations := runtime.attempts()
		if len(starts) != 1 || len(cancellations) != 2 {
			return false
		}
		reopened, err := workbench.Open(stateDirectory, workbench.Config{})
		return err == nil && len(reopened.PendingRuns(starts[0].SessionID)) == 0 && host.Repaint()
	})
	_, cancellations := runtime.attempts()
	if cancellations[0].CommandID == "" || cancellations[0].CommandID != cancellations[1].CommandID ||
		cancellations[0].RunID != cancellations[1].RunID {
		t.Fatalf("malformed receipt cleanup retry identities = %+v", cancellations)
	}
	host.Shows(t, "start segment stream: user item id is empty")
	host.Hides(t, "apply runtime event")
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
	stageDispatchingRun(t, store, command)
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
	stageDispatchingRun(t, store, command)
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
	stageDispatchingRun(t, store, command)
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
	idempotent := &idempotentStartRuntime{Runtime: base}
	runtime := &heldCancellationResultRuntime{
		idempotentStartRuntime: idempotent,
		settled:                make(chan struct{}, 1),
		release:                make(chan struct{}),
	}
	release := sync.OnceFunc(func() { close(runtime.release) })
	t.Cleanup(release)
	if _, err := runtime.StartRun(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	stateDirectory := t.TempDir()
	store, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	stageDispatchingRun(t, store, command)
	cancelID, err := store.MarkPendingRunCanceling(command.SessionID, command.CommandID, workbench.ReplayGuard{})
	if err != nil {
		t.Fatal(err)
	}

	host, stop := runUIWithState(t, runtime, "/tmp/lyra-cli-test", command.SessionID, stateDirectory)
	awaitSignal(t, runtime.settled, "runtime cancellation settlement")
	reopened, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	held := reopened.PendingRuns(command.SessionID)
	if len(held) != 1 || held[0].State != workbench.PendingRunCanceling || held[0].CancelCommandID != cancelID {
		t.Fatalf("unacknowledged cancellation ownership = %+v", held)
	}
	release()
	host.Until(t, "the canceled opening command to leave the durable outbox", func() bool {
		current, openErr := workbench.Open(stateDirectory, workbench.Config{})
		return openErr == nil && len(current.PendingRuns(command.SessionID)) == 0 && host.Repaint()
	})
	if cancelID == "" {
		t.Fatal("cancel command identity is empty")
	}
	stop()
}

func TestCanceledStartRetainsOwnershipUntilDurableSettlementRecovers(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{Delay: time.Hour, Event: agent.RunFinished{
			Outcome: agent.Outcome{Status: agent.OutcomeCompleted},
		}}}}
	}
	command := agent.StartRun{
		CommandID: agent.CommandID("cli_99999999999999999999999999999999"),
		SessionID: "ses_demo_1", Message: agent.Message{Text: "recover canceled start ownership"},
	}
	runtime := &heldCancellationResultRuntime{
		idempotentStartRuntime: &idempotentStartRuntime{Runtime: base},
		settled:                make(chan struct{}, 1),
		release:                make(chan struct{}),
	}
	releaseRuntime := sync.OnceFunc(func() { close(runtime.release) })
	t.Cleanup(releaseRuntime)
	if _, err := runtime.StartRun(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	stateDirectory := t.TempDir()
	store, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	stageDispatchingRun(t, store, command)
	cancelID, err := store.MarkPendingRunCanceling(command.SessionID, command.CommandID, workbench.ReplayGuard{})
	if err != nil {
		t.Fatal(err)
	}

	host, stop := runUIWithState(t, runtime, "/tmp/lyra-cli-test", command.SessionID, stateDirectory)
	awaitSignal(t, runtime.settled, "runtime cancellation before failed local settlement")
	states, err := os.ReadDir(filepath.Join(stateDirectory, "sessions"))
	if err != nil || len(states) != 1 {
		t.Fatalf("session state files = %d, %v", len(states), err)
	}
	statePath := filepath.Join(stateDirectory, "sessions", states[0].Name())
	backupPath := statePath + ".backup"
	if err := os.Rename(statePath, backupPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(statePath, 0o700); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(statePath, "blocker")
	if err := os.WriteFile(blocker, []byte("block canceled ownership settlement"), 0o600); err != nil {
		t.Fatal(err)
	}

	releaseRuntime()
	host.Shows(t, "workbench: retire canceled runtime ownership")
	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(statePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backupPath, statePath); err != nil {
		t.Fatal(err)
	}
	host.Until(t, "the canceled opening ownership to settle after storage recovers", func() bool {
		reopened, openErr := workbench.Open(stateDirectory, workbench.Config{})
		return openErr == nil && len(reopened.PendingRuns(command.SessionID)) == 0 && host.Repaint()
	})
	host.Hides(t, "workbench:")
	if cancelID == "" {
		t.Fatal("cancel command identity is empty")
	}
	stop()
}

func TestLaunchCancelsAnAcceptedRunWithAnInvalidRecoveredReceipt(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{Delay: time.Hour, Event: agent.RunFinished{
			Outcome: agent.Outcome{Status: agent.OutcomeCompleted},
		}}}}
	}
	command := agent.StartRun{
		CommandID: agent.CommandID("cli_88888888888888888888888888888888"),
		SessionID: "ses_demo_1", Message: agent.Message{Text: "cancel malformed start after restart"},
	}
	stateDirectory := t.TempDir()
	store, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	stageDispatchingRun(t, store, command)
	if _, err := store.MarkPendingRunCanceling(command.SessionID, command.CommandID, workbench.ReplayGuard{}); err != nil {
		t.Fatal(err)
	}
	runtime := &invalidAcceptedStartRuntime{Runtime: base}

	host, stop := runUIWithState(t, runtime, "/tmp/lyra-cli-test", command.SessionID, stateDirectory)
	host.Shows(t, "canceled")
	host.Until(t, "the invalid recovered start to leave the durable outbox", func() bool {
		reopened, openErr := workbench.Open(stateDirectory, workbench.Config{})
		return openErr == nil && len(reopened.PendingRuns(command.SessionID)) == 0 && host.Repaint()
	})
	starts, cancellations := runtime.attempts()
	if len(starts) != 1 || len(cancellations) != 1 || cancellations[0].RunID == "" ||
		cancellations[0].Reason != "canceled while start delivery was unconfirmed" {
		t.Fatalf("invalid recovered start cleanup = starts %+v, cancellations %+v", starts, cancellations)
	}
	snapshot, err := base.GetSession(t.Context(), command.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, active := snapshot.ActiveRun(); active {
		t.Fatalf("invalid recovered receipt left a run active: %+v", snapshot.Runs)
	}
	stop()
}

func stageDispatchingRun(t *testing.T, store *workbench.Store, command agent.StartRun) {
	t.Helper()
	if err := store.StagePendingRun(workbench.PendingRun{State: workbench.PendingRunQueued, Command: command}); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkPendingRunDispatching(command.SessionID, command.CommandID, workbench.ReplayGuard{}); err != nil {
		t.Fatal(err)
	}
}

func TestCommandReplayGuaranteeExpiresAtItsDeadline(t *testing.T) {
	deadline := time.Date(2026, 8, 13, 10, 1, 0, 0, time.UTC)
	profile := steerReplayTestProfile("/workspace")
	profile.Limits.IdempotencyNamespace = "runtime-a"
	guard := workbench.ReplayGuard{Namespace: "runtime-a", Until: deadline}
	if commandReplaySafeAt(guard, &profile, deadline) {
		t.Fatal("run command replay remained safe at its retention deadline")
	}
}

func TestRecoveredStartStopsBeforeRetryingOutsideItsReplayStore(t *testing.T) {
	base := mock.New()
	profile := steerReplayTestProfile("/tmp/lyra-cli-test")
	profile.Limits.IdempotencyNamespace = "runtime-a"
	runtime := &replayingStartRuntime{Runtime: base}
	runtime.afterFirst = func() { profile.Limits.IdempotencyNamespace = "runtime-b" }
	command := agent.StartRun{
		CommandID: "cli_cccccccccccccccccccccccccccccccc", SessionID: "ses_demo_1",
		Message: agent.Message{Text: "do not replay outside the owning store"},
	}
	_, err := openStartRunWithBackoff(
		t.Context(), runtime, command,
		workbench.ReplayGuard{Namespace: "runtime-a", Until: time.Now().UTC().Add(time.Hour)},
		&profile, runtimeRecoveryBackoff,
	)
	if !errors.Is(err, mutation.ErrReplayGuaranteeUnavailable) {
		t.Fatalf("recovered start error = %v", err)
	}
	if attempts := runtime.startAttempts(); len(attempts) != 1 {
		t.Fatalf("unowned start reached runtime %d times", len(attempts))
	}
}

func TestLaunchDoesNotReplayRunOrResumeOwnershipIntoAnotherRuntimeStore(t *testing.T) {
	for _, test := range []struct {
		name  string
		stage func(*testing.T, *workbench.Store)
		want  string
	}{
		{
			name: "run start",
			stage: func(t *testing.T, store *workbench.Store) {
				command := agent.StartRun{
					CommandID: "cli_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SessionID: "ses_demo_1",
					Message: agent.Message{Text: "do not replay across stores"},
				}
				if err := store.StagePendingRun(workbench.PendingRun{
					State: workbench.PendingRunQueued, Command: command,
				}); err != nil {
					t.Fatal(err)
				}
				if err := store.MarkPendingRunDispatching(command.SessionID, command.CommandID, workbench.ReplayGuard{
					Namespace: "runtime-a", Until: time.Now().UTC().Add(time.Hour),
				}); err != nil {
					t.Fatal(err)
				}
			},
			want: "recover pending run: replay guarantee expired or belongs to another runtime",
		},
		{
			name: "interaction resume",
			stage: func(t *testing.T, store *workbench.Store) {
				approval := agent.Approval{
					RunID: "run_waiting", ItemID: "approval_1", Title: "Proceed?",
					Tool: &agent.ToolCall{Kind: agent.ToolShell, Name: "shell", Status: agent.ToolRunning},
				}
				pending := workbench.PendingResume{
					Command: agent.ResumeRun{
						CommandID: "cli_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", RunID: approval.RunID,
						Answers: []agent.InterruptAnswer{{
							ItemID: approval.ItemID, Answer: agent.ApprovalAnswer{Decision: agent.ApprovalDeny},
						}},
					},
					Interactions: []agent.Interaction{approval},
					Replay: workbench.ReplayGuard{
						Namespace: "runtime-a", Until: time.Now().UTC().Add(time.Hour),
					},
				}
				if err := store.StagePendingResume("ses_demo_1", pending); err != nil {
					t.Fatal(err)
				}
			},
			want: "recover interaction decisions: command belongs to another runtime",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			stateDirectory := t.TempDir()
			store, err := workbench.Open(stateDirectory, workbench.Config{})
			if err != nil {
				t.Fatal(err)
			}
			test.stage(t, store)
			base := mock.New()
			runtime := &recordingRuntime{Runtime: base}
			profile := steerReplayTestProfile("/tmp/lyra-cli-test")
			profile.Limits.IdempotencyNamespace = "runtime-b"
			host, stop := runUIFromConfig(t, Config{
				Runtime: runtime, RuntimeProfile: &profile, SessionID: "ses_demo_1",
				Workspace: "/tmp/lyra-cli-test", StateDirectory: stateDirectory,
			})
			host.Shows(t, test.want)
			if runtime.startCount() != 0 {
				t.Fatalf("cross-store recovery opened %d runs", runtime.startCount())
			}
			reopened, err := workbench.Open(stateDirectory, workbench.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if test.name == "run start" && len(reopened.PendingRuns("ses_demo_1")) != 1 {
				t.Fatal("cross-store recovery retired pending run ownership")
			}
			if test.name == "interaction resume" {
				if _, found := reopened.PendingResume("ses_demo_1"); !found {
					t.Fatal("cross-store recovery retired pending resume ownership")
				}
			}
			stop()
		})
	}
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
