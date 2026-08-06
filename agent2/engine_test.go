package agent2

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type engineTestInput struct {
	Value string `json:"value"`
}

type engineTestOutput struct {
	Value string `json:"value"`
}

type engineTestState struct {
	Phase  string `json:"phase"`
	Value  string `json:"value"`
	WaitID string `json:"wait_id,omitempty"`
}

type engineTestMessage struct {
	Kind  string `json:"kind"`
	Value string `json:"value,omitempty"`
}

type engineTestDefinition struct {
	descriptor Descriptor
	mode       string
}

func newEngineTestDefinition(t testing.TB, name, mode string) *engineTestDefinition {
	t.Helper()
	inputSchema, err := SchemaFor[engineTestInput]()
	if err != nil {
		t.Fatal(err)
	}
	outputSchema, err := SchemaFor[engineTestOutput]()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := NewDescriptor(DescriptorConfig{
		Name: name, Description: "Exercises the Engine lifecycle contract.", Version: "0.1.0",
		InputSchema: inputSchema, OutputSchema: outputSchema,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &engineTestDefinition{descriptor: descriptor, mode: mode}
}

func (definition *engineTestDefinition) Descriptor() Descriptor { return definition.descriptor }

func (definition *engineTestDefinition) Start(input Input) (Execution, error) {
	value, err := DecodeInput[engineTestInput](input)
	if err != nil {
		return nil, err
	}
	return &engineTestExecution{mode: definition.mode, state: engineTestState{Phase: "ready", Value: value.Value}}, nil
}

func (definition *engineTestDefinition) Restore(state ExecutionState) (Execution, error) {
	if state.Kind() != definition.descriptor.Name() || state.SchemaVersion() != 1 {
		return nil, ErrInvalidExecutionState
	}
	value, err := decodeJSON[engineTestState](state.Payload())
	if err != nil {
		return nil, err
	}
	return &engineTestExecution{mode: definition.mode, state: value}, nil
}

type engineTestExecution struct {
	mode  string
	state engineTestState
}

func (execution *engineTestExecution) Step(_ context.Context, signals []Signal) (Transition, error) {
	switch execution.mode {
	case "effect":
		return execution.stepEffect(signals)
	case "wait":
		return execution.stepWait(signals)
	case "batch":
		return execution.stepBatch(signals)
	case "fail":
		execution.state.Phase = "corrupted"
		return Transition{}, errors.New("injected Step failure")
	default:
		return Transition{}, errors.New("unknown test mode")
	}
}

func (execution *engineTestExecution) stepBatch(signals []Signal) (Transition, error) {
	switch execution.state.Phase {
	case "ready":
		execution.state.Phase = "batch"
		var effects []Effect
		for _, value := range []string{"first", "second"} {
			payload, _ := json.Marshal(engineTestMessage{Kind: "request", Value: value})
			effect, err := NewDispatcherEffect(payload)
			if err != nil {
				return Transition{}, err
			}
			effects = append(effects, effect)
		}
		return Continue(0, effects...)
	case "batch":
		if len(signals) != 2 {
			return Transition{}, errors.New("batch phase requires two settlement Signals")
		}
		first, err := decodeJSON[engineTestMessage](signals[0].Payload())
		if err != nil {
			return Transition{}, err
		}
		second, err := decodeJSON[engineTestMessage](signals[1].Payload())
		if err != nil {
			return Transition{}, err
		}
		execution.state.Phase = "done"
		output, _ := EncodeOutput(engineTestOutput{Value: first.Value + "+" + second.Value})
		return Complete(2, output)
	default:
		return Transition{}, errors.New("batch execution cannot advance")
	}
}

func (execution *engineTestExecution) stepEffect(signals []Signal) (Transition, error) {
	switch execution.state.Phase {
	case "ready":
		if len(signals) != 0 {
			return Transition{}, errors.New("ready phase expected no Signal")
		}
		execution.state.Phase = "effect"
		payload, _ := json.Marshal(engineTestMessage{Kind: "request", Value: execution.state.Value})
		effect, err := NewDispatcherEffect(payload)
		if err != nil {
			return Transition{}, err
		}
		return Continue(0, effect)
	case "effect":
		if len(signals) == 0 {
			return Transition{}, errors.New("effect phase requires settlement Signal")
		}
		message, err := decodeJSON[engineTestMessage](signals[0].Payload())
		if err != nil {
			return Transition{}, err
		}
		execution.state.Phase = "done"
		output, _ := EncodeOutput(engineTestOutput{Value: message.Value})
		return Complete(1, output)
	default:
		return Transition{}, errors.New("effect execution cannot advance")
	}
}

func (execution *engineTestExecution) stepWait(signals []Signal) (Transition, error) {
	switch execution.state.Phase {
	case "ready":
		execution.state.Phase = "wait_id"
		key, _ := ParseWaitKey("approval")
		payload, _ := json.Marshal(engineTestMessage{Kind: "wait_opened"})
		effect, err := RequestWait(key, payload)
		if err != nil {
			return Transition{}, err
		}
		return Continue(0, effect)
	case "wait_id":
		if len(signals) == 0 {
			return Transition{}, errors.New("wait identity Signal is required")
		}
		waitID, ok := signals[0].WaitID()
		if !ok {
			return Transition{}, errors.New("wait identity Signal has no WaitID")
		}
		execution.state.Phase = "answer"
		execution.state.WaitID = waitID.String()
		return Wait(1, waitID)
	case "answer":
		if len(signals) == 0 {
			return Transition{}, errors.New("answer Signal is required")
		}
		waitID, _ := signals[0].WaitID()
		if waitID.String() != execution.state.WaitID {
			return Transition{}, errors.New("answer addressed another wait")
		}
		message, err := decodeJSON[engineTestMessage](signals[0].Payload())
		if err != nil {
			return Transition{}, err
		}
		execution.state.Phase = "done"
		output, _ := EncodeOutput(engineTestOutput{Value: message.Value})
		return Complete(1, output)
	default:
		return Transition{}, errors.New("wait execution cannot advance")
	}
}

func (execution *engineTestExecution) Snapshot() (ExecutionState, error) {
	payload, err := json.Marshal(execution.state)
	if err != nil {
		return ExecutionState{}, err
	}
	name := "engine.effect"
	if execution.mode == "wait" {
		name = "engine.wait"
	} else if execution.mode == "fail" {
		name = "engine.fail"
	} else if execution.mode == "batch" {
		name = "engine.batch"
	}
	return NewExecutionState(name, 1, payload)
}

type engineTestDispatcher struct {
	policy ReplayPolicy
	calls  atomic.Int32
	block  <-chan struct{}
	check  func() error
	deltas int
}

func (dispatcher *engineTestDispatcher) Dispatch(
	_ context.Context,
	request EffectRequest,
	emit DeltaEmitter,
) (Settlement, error) {
	dispatcher.calls.Add(1)
	if dispatcher.check != nil {
		if err := dispatcher.check(); err != nil {
			return Settlement{}, err
		}
	}
	if dispatcher.block != nil {
		<-dispatcher.block
	}
	message, err := decodeJSON[engineTestMessage](request.Effect().Payload())
	if err != nil {
		return Settlement{}, err
	}
	delta, _ := json.Marshal(engineTestMessage{Kind: "delta", Value: message.Value})
	count := dispatcher.deltas
	if count == 0 {
		count = 1
	}
	for range count {
		emit(delta)
	}
	payload, _ := json.Marshal(engineTestMessage{Kind: "result", Value: message.Value + ":done"})
	return NewSettlement(request.ID(), SettlementStatusSucceeded, payload)
}

func (dispatcher *engineTestDispatcher) ReplayPolicy(Effect) ReplayPolicy { return dispatcher.policy }

type failingEngineTestDispatcher struct {
	calls atomic.Int32
}

type partialBatchDispatcher struct {
	calls atomic.Int32
}

func (dispatcher *partialBatchDispatcher) Dispatch(
	_ context.Context,
	request EffectRequest,
	_ DeltaEmitter,
) (Settlement, error) {
	dispatcher.calls.Add(1)
	message, err := decodeJSON[engineTestMessage](request.Effect().Payload())
	if err != nil {
		return Settlement{}, err
	}
	if request.BatchIndex() == 1 {
		return Settlement{}, errors.New("second Effect result is unknown")
	}
	payload, _ := json.Marshal(engineTestMessage{Kind: "result", Value: message.Value})
	return NewSettlement(request.ID(), SettlementStatusSucceeded, payload)
}

func (*partialBatchDispatcher) ReplayPolicy(Effect) ReplayPolicy { return ReplayPolicyNever }

func (dispatcher *failingEngineTestDispatcher) Dispatch(
	context.Context,
	EffectRequest,
	DeltaEmitter,
) (Settlement, error) {
	dispatcher.calls.Add(1)
	return Settlement{}, errors.New("external result is unknown")
}

func (*failingEngineTestDispatcher) ReplayPolicy(Effect) ReplayPolicy { return ReplayPolicyNever }

type engineTestAcknowledger struct {
	mu       sync.Mutex
	snapshot Snapshot
	called   atomic.Bool
}

func (acknowledger *engineTestAcknowledger) AcknowledgePreparedStep(_ context.Context, snapshot Snapshot) error {
	acknowledger.mu.Lock()
	acknowledger.snapshot = snapshot
	acknowledger.mu.Unlock()
	acknowledger.called.Store(true)
	return nil
}

func (acknowledger *engineTestAcknowledger) captured() Snapshot {
	acknowledger.mu.Lock()
	defer acknowledger.mu.Unlock()
	return acknowledger.snapshot
}

func TestEngineRunsEffectToValidatedOutput(t *testing.T) {
	definition := newEngineTestDefinition(t, "engine.effect", "effect")
	dispatcher := &engineTestDispatcher{policy: ReplayPolicyNever}
	deployment := engineTestDeployment(t, definition, dispatcher)
	engine, err := NewEngine(EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	input, _ := EncodeInput(engineTestInput{Value: "hello"})
	result, err := engine.Run(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid() || result.Status() != StatusCompleted || dispatcher.calls.Load() != 1 {
		t.Fatalf("result=%+v calls=%d", result, dispatcher.calls.Load())
	}
	output, ok := result.Output()
	if !ok {
		t.Fatal("completed result has no Output")
	}
	value, err := DecodeOutput[engineTestOutput](output)
	if err != nil || value.Value != "hello:done" {
		t.Fatalf("output=%+v err=%v", value, err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestEngineMintsWaitIDAndRequiresAddressedAnswer(t *testing.T) {
	definition := newEngineTestDefinition(t, "engine.wait", "wait")
	deployment := engineTestDeployment(t, definition, &engineTestDispatcher{policy: ReplayPolicyNever})
	engine, _ := NewEngine(EngineConfig{})
	input, _ := EncodeInput(engineTestInput{Value: "question"})
	process, err := engine.Start(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, process, StatusWaiting)
	waitID, ok := process.WaitID()
	if !ok {
		t.Fatal("Waiting Process did not expose Engine-minted WaitID")
	}
	plainID, _ := ParseSignalID("signal:plain")
	plain, _ := NewSignalRequest(plainID, WaitID{}, json.RawMessage(`{"kind":"answer","value":"wrong"}`))
	if _, err := process.DeliverSignal(context.Background(), plain); !errors.Is(err, ErrSignalRejected) {
		t.Fatalf("unaddressed answer error=%v", err)
	}
	answerID, _ := ParseSignalID("signal:answer")
	answer, _ := NewSignalRequest(answerID, waitID, json.RawMessage(`{"kind":"answer","value":"approved"}`))
	accepted, err := process.DeliverSignal(context.Background(), answer)
	if err != nil || !accepted {
		t.Fatalf("answer accepted=%t err=%v", accepted, err)
	}
	result := awaitResult(t, process)
	output, _ := result.Output()
	value, _ := DecodeOutput[engineTestOutput](output)
	if value.Value != "approved" {
		t.Fatalf("output=%q", value.Value)
	}
}

func TestEngineAcknowledgesPreparedSnapshotBeforeDispatch(t *testing.T) {
	definition := newEngineTestDefinition(t, "engine.effect", "effect")
	acknowledger := &engineTestAcknowledger{}
	dispatcher := &engineTestDispatcher{policy: ReplayPolicyNever}
	dispatcher.check = func() error {
		if !acknowledger.called.Load() {
			return errors.New("dispatch happened before prepared acknowledgment")
		}
		return nil
	}
	deployment := engineTestDeployment(t, definition, dispatcher)
	engine, err := NewEngine(EngineConfig{PreparedStepAcknowledger: acknowledger})
	if err != nil {
		t.Fatal(err)
	}
	input, _ := EncodeInput(engineTestInput{Value: "durable"})
	result, err := engine.Run(context.Background(), deployment, input)
	if err != nil || result.Status() != StatusCompleted {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	snapshot := acknowledger.captured()
	wire, err := snapshot.wire()
	if err != nil || wire.Prepared == nil || wire.Prepared.Effects[0].Settlement != nil {
		t.Fatalf("acknowledged snapshot does not contain pre-dispatch prepared boundary: %v", err)
	}
}

func TestUnknownSettlementRequiresExplicitResolutionAndSurvivesRestore(t *testing.T) {
	definition := newEngineTestDefinition(t, "engine.effect", "effect")
	dispatcher := &failingEngineTestDispatcher{}
	deployment := engineTestDeployment(t, definition, dispatcher)
	engine, _ := NewEngine(EngineConfig{})
	input, _ := EncodeInput(engineTestInput{Value: "uncertain"})
	process, err := engine.Start(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := waitForUnknownSettlement(t, process)
	wire, _ := snapshot.wire()
	effectID := wire.Prepared.Effects[0].ID

	restoredEngine, _ := NewEngine(EngineConfig{})
	restored, err := restoredEngine.Restore(context.Background(), deployment, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if dispatcher.calls.Load() != 1 {
		t.Fatalf("ReplayPolicyNever dispatcher calls=%d, want 1", dispatcher.calls.Load())
	}
	unknown, err := restored.UnknownEffectIDs(context.Background())
	if err != nil || len(unknown) != 1 || unknown[0] != effectID {
		t.Fatalf("unknown Effects=%v err=%v", unknown, err)
	}
	payload, _ := json.Marshal(engineTestMessage{Kind: "result", Value: "resolved"})
	settlement, _ := NewSettlement(effectID, SettlementStatusSucceeded, payload)
	if err := restored.ResolveEffect(context.Background(), settlement); err != nil {
		t.Fatal(err)
	}
	result := awaitResult(t, restored)
	output, _ := result.Output()
	value, _ := DecodeOutput[engineTestOutput](output)
	if value.Value != "resolved" {
		t.Fatalf("resolved output=%q", value.Value)
	}
	_ = process.Kill(context.Background(), "test cleanup")
	_ = awaitResult(t, process)
}

func TestPartialEffectBatchPreservesSettlementsAndDeclarationOrder(t *testing.T) {
	definition := newEngineTestDefinition(t, "engine.batch", "batch")
	dispatcher := &partialBatchDispatcher{}
	deployment := engineTestDeployment(t, definition, dispatcher)
	engine, _ := NewEngine(EngineConfig{})
	input, _ := EncodeInput(engineTestInput{Value: "batch"})
	process, err := engine.Start(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := waitForUnknownSettlement(t, process)
	wire, _ := snapshot.wire()
	if len(wire.Prepared.Effects) != 2 || wire.Prepared.Effects[0].Settlement == nil ||
		wire.Prepared.Effects[0].Settlement.Status() != SettlementStatusSucceeded ||
		wire.Prepared.Effects[1].Settlement == nil ||
		wire.Prepared.Effects[1].Settlement.Status() != SettlementStatusUnknown {
		t.Fatalf("prepared batch=%+v", wire.Prepared.Effects)
	}
	restoredEngine, _ := NewEngine(EngineConfig{})
	restored, err := restoredEngine.Restore(context.Background(), deployment, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if dispatcher.calls.Load() != 2 {
		t.Fatalf("dispatcher calls=%d, want 2", dispatcher.calls.Load())
	}
	payload, _ := json.Marshal(engineTestMessage{Kind: "result", Value: "second"})
	settlement, _ := NewSettlement(wire.Prepared.Effects[1].ID, SettlementStatusSucceeded, payload)
	if err := restored.ResolveEffect(context.Background(), settlement); err != nil {
		t.Fatal(err)
	}
	result := awaitResult(t, restored)
	output, _ := result.Output()
	value, _ := DecodeOutput[engineTestOutput](output)
	if value.Value != "first+second" {
		t.Fatalf("ordered batch output=%q", value.Value)
	}
	_ = process.Kill(context.Background(), "test cleanup")
	_ = awaitResult(t, process)
}

func TestPausedProcessCapturesRestoresAndResumesAtSafeBoundary(t *testing.T) {
	definition := newEngineTestDefinition(t, "engine.effect", "effect")
	release := make(chan struct{})
	dispatcher := &engineTestDispatcher{policy: ReplayPolicySameIdentity, block: release}
	deployment := engineTestDeployment(t, definition, dispatcher)
	engine, _ := NewEngine(EngineConfig{})
	input, _ := EncodeInput(engineTestInput{Value: "paused"})
	process, err := engine.Start(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	waitForCalls(t, &dispatcher.calls, 1)
	if err := process.Pause(context.Background(), "operator inspection"); err != nil {
		t.Fatal(err)
	}
	if process.Status() == StatusPaused {
		t.Fatal("Pause became visible before the in-flight Effect settled")
	}
	close(release)
	waitForStatus(t, process, StatusPaused)
	snapshot, err := process.Capture(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	restoredEngine, _ := NewEngine(EngineConfig{})
	restored, err := restoredEngine.Restore(context.Background(), deployment, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Status() != StatusPaused {
		t.Fatalf("restored status=%s", restored.Status())
	}
	if err := restored.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	result := awaitResult(t, restored)
	if result.Status() != StatusCompleted {
		t.Fatalf("result status=%s", result.Status())
	}
	_ = process.Kill(context.Background(), "test cleanup")
	_ = awaitResult(t, process)
}

func TestWaitingProcessRestoresWithSameWaitIdentity(t *testing.T) {
	definition := newEngineTestDefinition(t, "engine.wait", "wait")
	deployment := engineTestDeployment(t, definition, &engineTestDispatcher{policy: ReplayPolicyNever})
	engine, _ := NewEngine(EngineConfig{})
	input, _ := EncodeInput(engineTestInput{Value: "question"})
	process, err := engine.Start(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, process, StatusWaiting)
	waitID, _ := process.WaitID()
	snapshot, err := process.Capture(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	restoredEngine, _ := NewEngine(EngineConfig{})
	restored, err := restoredEngine.Restore(context.Background(), deployment, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	restoredWaitID, ok := restored.WaitID()
	if !ok || restoredWaitID != waitID {
		t.Fatalf("restored WaitID=%s want=%s", restoredWaitID, waitID)
	}
	answerID, _ := ParseSignalID("signal:restored-answer")
	answer, _ := NewSignalRequest(answerID, restoredWaitID, json.RawMessage(`{"kind":"answer","value":"restored"}`))
	if accepted, err := restored.DeliverSignal(context.Background(), answer); err != nil || !accepted {
		t.Fatalf("accepted=%t err=%v", accepted, err)
	}
	if result := awaitResult(t, restored); result.Status() != StatusCompleted {
		t.Fatalf("result status=%s", result.Status())
	}
	_ = process.Kill(context.Background(), "test cleanup")
	_ = awaitResult(t, process)
}

func TestRestoredPreparedEffectReplaysOnlyWithSameIdentityPolicy(t *testing.T) {
	definition := newEngineTestDefinition(t, "engine.effect", "effect")
	acknowledger := &engineTestAcknowledger{}
	dispatcher := &engineTestDispatcher{policy: ReplayPolicySameIdentity}
	deployment := engineTestDeployment(t, definition, dispatcher)
	engine, _ := NewEngine(EngineConfig{PreparedStepAcknowledger: acknowledger})
	input, _ := EncodeInput(engineTestInput{Value: "replay"})
	if _, err := engine.Run(context.Background(), deployment, input); err != nil {
		t.Fatal(err)
	}
	snapshot := acknowledger.captured()
	restoredEngine, _ := NewEngine(EngineConfig{})
	restored, err := restoredEngine.Restore(context.Background(), deployment, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if result := awaitResult(t, restored); result.Status() != StatusCompleted {
		t.Fatalf("result status=%s", result.Status())
	}
	if dispatcher.calls.Load() != 2 {
		t.Fatalf("same-identity dispatcher calls=%d, want 2", dispatcher.calls.Load())
	}
}

func TestStartContextCancellationMapsToHostCancellation(t *testing.T) {
	definition := newEngineTestDefinition(t, "engine.wait", "wait")
	deployment := engineTestDeployment(t, definition, &engineTestDispatcher{policy: ReplayPolicyNever})
	engine, _ := NewEngine(EngineConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	input, _ := EncodeInput(engineTestInput{Value: "cancel"})
	process, err := engine.Start(ctx, deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, process, StatusWaiting)
	cancel()
	result := awaitResult(t, process)
	if result.Status() != StatusCancelled || result.Termination().Cause() != TerminationCauseHostCancellation {
		t.Fatalf("termination=%+v", result.Termination())
	}
}

func TestKillWaitsForInflightEffectSettlement(t *testing.T) {
	definition := newEngineTestDefinition(t, "engine.effect", "effect")
	release := make(chan struct{})
	dispatcher := &engineTestDispatcher{policy: ReplayPolicyNever, block: release}
	deployment := engineTestDeployment(t, definition, dispatcher)
	engine, _ := NewEngine(EngineConfig{})
	input, _ := EncodeInput(engineTestInput{Value: "slow"})
	process, err := engine.Start(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	waitForCalls(t, &dispatcher.calls, 1)
	if err := process.Kill(context.Background(), "operator requested stop"); err != nil {
		t.Fatal(err)
	}
	if process.Status().Terminal() {
		t.Fatal("Kill abandoned an in-flight Effect")
	}
	close(release)
	result := awaitResult(t, process)
	if result.Status() != StatusKilled || result.Termination().Cause() != TerminationCauseEngineKill {
		t.Fatalf("termination=%+v", result.Termination())
	}
}

func TestStepFailureDiscardsMutatedExecutionAndPreservesCursor(t *testing.T) {
	definition := newEngineTestDefinition(t, "engine.fail", "fail")
	deployment := engineTestDeployment(t, definition, &engineTestDispatcher{policy: ReplayPolicyNever})
	engine, _ := NewEngine(EngineConfig{})
	input, _ := EncodeInput(engineTestInput{Value: "stable"})
	process, err := engine.Start(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	result := awaitResult(t, process)
	if result.Status() != StatusFailed || result.Termination().Cause() != TerminationCauseExecutionFailure {
		t.Fatalf("termination=%+v", result.Termination())
	}
	snapshot, err := process.Capture(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wire, _ := snapshot.wire()
	state, _ := decodeJSON[engineTestState](wire.LastStableState.Payload())
	if state.Phase != "ready" || wire.Mailbox.SignalCursor != 0 || wire.Prepared != nil {
		t.Fatalf("last stable state=%+v cursor=%d prepared=%v", state, wire.Mailbox.SignalCursor, wire.Prepared)
	}
}

func TestEngineEnforcesStepLimitAndReportsMonotonicUsage(t *testing.T) {
	definition := newEngineTestDefinition(t, "engine.effect", "effect")
	deployment := engineTestDeployment(t, definition, &engineTestDispatcher{policy: ReplayPolicyNever})
	engine, err := NewEngine(EngineConfig{Limits: Limits{
		MaxSteps: 1, MaxEffects: 1, MaxSignals: 1, MaxPendingSignals: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	input, _ := EncodeInput(engineTestInput{Value: "bounded"})
	result, err := engine.Run(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status() != StatusFailed {
		t.Fatalf("status=%s", result.Status())
	}
	failure, ok := result.Termination().Failure()
	if !ok || failure.Code() != "engine.limit.steps" {
		t.Fatalf("failure=%+v", failure)
	}
	usage := result.Usage()
	if usage.CommittedSteps != 1 || usage.PreparedEffects != 1 || usage.AcceptedSignals != 1 {
		t.Fatalf("usage=%+v", usage)
	}
}

type recordingEventListener struct {
	mu     sync.Mutex
	events []Event
	panic  bool
}

func (listener *recordingEventListener) snapshot() []Event {
	listener.mu.Lock()
	defer listener.mu.Unlock()
	return append([]Event(nil), listener.events...)
}

func (listener *recordingEventListener) OnEvent(_ context.Context, event Event) {
	listener.mu.Lock()
	listener.events = append(listener.events, event)
	shouldPanic := listener.panic
	listener.mu.Unlock()
	if shouldPanic {
		panic("observer failure")
	}
}

func (listener *recordingEventListener) has(name string) bool {
	listener.mu.Lock()
	defer listener.mu.Unlock()
	for _, event := range listener.events {
		if event.Name() == name {
			return true
		}
	}
	return false
}

type blockingDeltaListener struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (listener *blockingDeltaListener) OnDelta(context.Context, Delta) {
	listener.once.Do(func() { close(listener.entered) })
	<-listener.release
}

func TestDeltaBufferDropsAreObservableAndListenerPanicIsIsolated(t *testing.T) {
	definition := newEngineTestDefinition(t, "engine.effect", "effect")
	dispatcher := &engineTestDispatcher{policy: ReplayPolicyNever, deltas: 100}
	deployment := engineTestDeployment(t, definition, dispatcher)
	events := &recordingEventListener{panic: true}
	deltas := &blockingDeltaListener{entered: make(chan struct{}), release: make(chan struct{})}
	engine, err := NewEngine(EngineConfig{
		EventListeners: []EventListener{events}, DeltaListeners: []DeltaListener{deltas}, DeltaBufferCapacity: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	input, _ := EncodeInput(engineTestInput{Value: "stream"})
	result, err := engine.Run(context.Background(), deployment, input)
	if err != nil || result.Status() != StatusCompleted {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if result.Usage().DroppedDeltas == 0 || !events.has("agent.delta.dropped") {
		t.Fatalf("usage=%+v dropped event=%t", result.Usage(), events.has("agent.delta.dropped"))
	}
	close(deltas.release)
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestEventLifecycleCarriesExactBindingAndAttemptDurations(t *testing.T) {
	definition := newEngineTestDefinition(t, "engine.effect", "effect")
	deployment := engineTestDeployment(t, definition, &engineTestDispatcher{policy: ReplayPolicyNever})
	listener := &recordingEventListener{}
	engine, err := NewEngine(EngineConfig{EventListeners: []EventListener{listener}})
	if err != nil {
		t.Fatal(err)
	}
	input, _ := EncodeInput(engineTestInput{Value: "events"})
	result, err := engine.Run(context.Background(), deployment, input)
	if err != nil || result.Status() != StatusCompleted {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	events := listener.snapshot()
	wantNames := []string{
		EventProcessStarted,
		EventStepStarted, EventStepFinished, EventStepPrepared,
		EventEffectStarted, EventEffectFinished, EventStepCommitted,
		EventStepStarted, EventStepFinished, EventStepPrepared, EventStepCommitted,
		EventProcessFinished,
	}
	if len(events) != len(wantNames) {
		t.Fatalf("events = %d, want %d: %#v", len(events), len(wantNames), eventNames(events))
	}
	for index, event := range events {
		if event.Name() != wantNames[index] {
			t.Fatalf("event[%d] = %q, want %q", index, event.Name(), wantNames[index])
		}
		if event.ProcessSequence() != uint64(index+1) || event.ProcessID() != result.ProcessID() ||
			event.DeploymentRef() != deployment.DeploymentRef() ||
			event.Relation().ProcessID() != result.ProcessID() || !event.Relation().IsRoot() {
			t.Fatalf("event[%d] envelope = %+v", index, event)
		}
	}
	for _, event := range events {
		if event.Name() != EventStepFinished && event.Name() != EventEffectFinished {
			continue
		}
		var payload struct {
			Target     string `json:"target"`
			Status     string `json:"status"`
			DurationMS int64  `json:"duration_ms"`
		}
		if err := json.Unmarshal(event.Payload(), &payload); err != nil || payload.DurationMS < 0 || payload.Status != "succeeded" {
			t.Fatalf("event %q payload = %s, error = %v", event.Name(), event.Payload(), err)
		}
		if event.Name() == EventEffectFinished && payload.Target != "dispatcher" {
			t.Fatalf("Effect target = %q", payload.Target)
		}
	}
}

func TestFrameworkEffectPublishesTheSameLifecycleContract(t *testing.T) {
	deployment := newChildTestDeployment(t)
	listener := &recordingEventListener{}
	engine, err := NewEngine(EngineConfig{EventListeners: []EventListener{listener}})
	if err != nil {
		t.Fatal(err)
	}
	input, _ := EncodeInput(childTestInput{Mode: "parent"})
	root, err := engine.Start(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	output := childTestResult(t, mustAwait(t, root))
	if len(output.ChildIDs) != 1 {
		t.Fatalf("root output = %#v", output)
	}
	childID, _ := ParseProcessID(output.ChildIDs[0])
	child, _ := engine.Process(childID)
	_ = mustAwait(t, child)
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	var started, finished bool
	for _, event := range listener.snapshot() {
		if event.ProcessID() != root.ID() ||
			(event.Name() != EventEffectStarted && event.Name() != EventEffectFinished) {
			continue
		}
		var payload struct {
			Target string `json:"target"`
		}
		if err := json.Unmarshal(event.Payload(), &payload); err != nil || payload.Target != "framework" {
			continue
		}
		started = started || event.Name() == EventEffectStarted
		finished = finished || event.Name() == EventEffectFinished
	}
	if !started || !finished {
		t.Fatalf("Framework Effect lifecycle started=%t finished=%t", started, finished)
	}
}

func eventNames(events []Event) []string {
	names := make([]string, len(events))
	for index, event := range events {
		names[index] = event.Name()
	}
	return names
}

func engineTestDeployment(t testing.TB, definition Definition, dispatcher Dispatcher) Deployment {
	t.Helper()
	deployment, err := NewDeployment(DeploymentConfig{
		Definition: definition, Dispatcher: dispatcher,
		ImplementationDigest: ComputeDigest([]byte(definition.Descriptor().Name() + ":implementation")),
		ConfigurationDigest:  ComputeDigest([]byte(definition.Descriptor().Name() + ":configuration")),
	})
	if err != nil {
		t.Fatal(err)
	}
	return deployment
}

func awaitResult(t *testing.T, process *Process) Result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := process.Await(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func waitForStatus(t *testing.T, process *Process, want Status) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if process.Status() == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("status=%s, want %s", process.Status(), want)
}

func waitForCalls(t *testing.T, calls *atomic.Int32, want int32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if calls.Load() >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("dispatcher calls=%d, want at least %d", calls.Load(), want)
}

func waitForUnknownSettlement(t *testing.T, process *Process) Snapshot {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, err := process.Capture(context.Background())
		if err == nil {
			wire, _ := snapshot.wire()
			if wire.Prepared != nil {
				for _, effect := range wire.Prepared.Effects {
					if effect.Settlement != nil && effect.Settlement.Status() == SettlementStatusUnknown {
						return snapshot
					}
				}
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("Process never exposed unknown Effect settlement")
	return Snapshot{}
}
