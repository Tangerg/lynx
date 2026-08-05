package agent2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"
)

var (
	errPreparedAcknowledgment = errors.New("prepared snapshot is not acknowledged")
	errUnknownSettlement      = errors.New("effect settlement is unknown")
)

type preparedSpikeInput struct {
	Value string `json:"value"`
}

type preparedSpikeOutput struct {
	Value string `json:"value"`
}

type preparedSpikeState struct {
	Phase string `json:"phase"`
	Value string `json:"value"`
}

type preparedSpikeSignal struct {
	Kind  string `json:"kind"`
	Value string `json:"value,omitempty"`
}

type preparedSpikeDefinition struct {
	descriptor Descriptor
}

func newPreparedSpikeDefinition(t *testing.T) *preparedSpikeDefinition {
	t.Helper()
	inputSchema, err := SchemaFor[preparedSpikeInput]()
	if err != nil {
		t.Fatal(err)
	}
	outputSchema, err := SchemaFor[preparedSpikeOutput]()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := NewDescriptor(DescriptorConfig{
		Name:         "prepared.spike",
		Description:  "Validates prepared Step and durable recovery boundaries.",
		Version:      "0.1.0",
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &preparedSpikeDefinition{descriptor: descriptor}
}

func (definition *preparedSpikeDefinition) Descriptor() Descriptor { return definition.descriptor }

func (definition *preparedSpikeDefinition) Start(input Input) (Execution, error) {
	if err := definition.descriptor.ValidateInput(input); err != nil {
		return nil, err
	}
	value, err := DecodeInput[preparedSpikeInput](input)
	if err != nil {
		return nil, err
	}
	return &preparedSpikeExecution{state: preparedSpikeState{Phase: "ready", Value: value.Value}}, nil
}

func (definition *preparedSpikeDefinition) Restore(state ExecutionState) (Execution, error) {
	if state.Kind() != definition.descriptor.Name() || state.SchemaVersion() != 1 {
		return nil, errors.New("unsupported prepared spike state")
	}
	value, err := decodeJSON[preparedSpikeState](state.Payload())
	if err != nil {
		return nil, err
	}
	if value.Phase != "ready" && value.Phase != "settlement" {
		return nil, errors.New("invalid prepared spike phase")
	}
	return &preparedSpikeExecution{state: value}, nil
}

type preparedSpikeExecution struct {
	state preparedSpikeState
}

func (execution *preparedSpikeExecution) Step(ctx context.Context, signals []Signal) (Transition, error) {
	if err := ctx.Err(); err != nil {
		return Transition{}, err
	}
	if len(signals) != 1 {
		return Transition{}, errors.New("prepared spike requires one signal")
	}
	value, err := decodeJSON[preparedSpikeSignal](signals[0].Payload())
	if err != nil {
		return Transition{}, err
	}
	switch execution.state.Phase {
	case "ready":
		execution.state.Phase = "corrupted if Step fails"
		if value.Kind == "fail" {
			return Transition{}, errors.New("injected Step failure")
		}
		if value.Kind != "start" {
			return Transition{}, errors.New("expected start signal")
		}
		execution.state.Phase = "settlement"
		payload, err := json.Marshal(preparedSpikeSignal{Kind: "effect", Value: execution.state.Value})
		if err != nil {
			return Transition{}, err
		}
		effect, err := NewDispatcherEffect(payload)
		if err != nil {
			return Transition{}, err
		}
		return Continue(1, effect)
	case "settlement":
		if value.Kind != "result" {
			return Transition{}, errors.New("expected settlement result")
		}
		output, err := EncodeOutput(preparedSpikeOutput{Value: value.Value})
		if err != nil {
			return Transition{}, err
		}
		return Complete(1, output)
	default:
		return Transition{}, errors.New("untrusted prepared spike instance")
	}
}

func (execution *preparedSpikeExecution) Snapshot() (ExecutionState, error) {
	payload, err := json.Marshal(execution.state)
	if err != nil {
		return ExecutionState{}, err
	}
	return NewExecutionState("prepared.spike", 1, payload)
}

type preparedSpikeDeployment struct {
	reference  DeploymentRef
	definition Definition
}

func newPreparedSpikeDeployment(t *testing.T, definition Definition, implementation, configuration string) preparedSpikeDeployment {
	t.Helper()
	reference, err := NewDeploymentRef(definition.Descriptor(), digestBytes([]byte(implementation)), digestBytes([]byte(configuration)))
	if err != nil {
		t.Fatal(err)
	}
	return preparedSpikeDeployment{reference: reference, definition: definition}
}

type preparedSpikeSignalRecord struct {
	Sequence uint64 `json:"sequence"`
	Signal   Signal `json:"signal"`
}

type preparedSpikeEffect struct {
	ID         EffectID    `json:"id"`
	Effect     Effect      `json:"effect"`
	Settlement *Settlement `json:"settlement,omitempty"`
}

type preparedSpikeStep struct {
	Sequence         uint64                `json:"sequence"`
	LastStableDigest Digest                `json:"last_stable_digest"`
	CandidateState   ExecutionState        `json:"candidate_state"`
	ConsumeThrough   uint64                `json:"consume_through"`
	Transition       Transition            `json:"transition"`
	Effects          []preparedSpikeEffect `json:"effects,omitempty"`
}

type preparedSpikeSnapshot struct {
	SchemaVersion uint16                      `json:"schema_version"`
	ProcessID     ProcessID                   `json:"process_id"`
	Deployment    DeploymentRef               `json:"deployment"`
	Status        Status                      `json:"status"`
	Step          uint64                      `json:"step"`
	LastStable    ExecutionState              `json:"last_stable"`
	Signals       []preparedSpikeSignalRecord `json:"signals,omitempty"`
	Cursor        uint64                      `json:"cursor"`
	Prepared      *preparedSpikeStep          `json:"prepared,omitempty"`
}

type preparedSpikeEngine struct {
	snapshot     preparedSpikeSnapshot
	deployment   preparedSpikeDeployment
	execution    Execution
	acknowledged bool
	output       Output
}

func newPreparedSpikeEngine(t *testing.T, deployment preparedSpikeDeployment, value string) *preparedSpikeEngine {
	t.Helper()
	input, err := EncodeInput(preparedSpikeInput{Value: value})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := deployment.definition.Start(input)
	if err != nil {
		t.Fatal(err)
	}
	state, err := execution.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	return &preparedSpikeEngine{
		deployment: deployment,
		execution:  execution,
		snapshot: preparedSpikeSnapshot{
			SchemaVersion: 1,
			ProcessID:     mustProcessID("process:prepared"),
			Deployment:    deployment.reference,
			Status:        StatusRunning,
			Step:          1,
			LastStable:    state,
		},
	}
}

func restorePreparedSpikeEngine(data []byte, deployment preparedSpikeDeployment) (*preparedSpikeEngine, error) {
	var snapshot preparedSpikeSnapshot
	if err := decodeJSONInto(data, &snapshot); err != nil {
		return nil, err
	}
	if err := validatePreparedSpikeSnapshot(snapshot, deployment.reference); err != nil {
		return nil, err
	}
	state := snapshot.LastStable
	if snapshot.Prepared != nil && len(snapshot.Prepared.Effects) == 0 {
		state = snapshot.Prepared.CandidateState
	}
	execution, err := deployment.definition.Restore(state)
	if err != nil {
		return nil, err
	}
	return &preparedSpikeEngine{
		snapshot:     snapshot,
		deployment:   deployment,
		execution:    execution,
		acknowledged: snapshot.Prepared != nil,
	}, nil
}

func validatePreparedSpikeSnapshot(snapshot preparedSpikeSnapshot, reference DeploymentRef) error {
	if snapshot.SchemaVersion != 1 || !snapshot.ProcessID.Valid() || snapshot.Deployment != reference ||
		snapshot.Status != StatusRunning || !snapshot.LastStable.Valid() || snapshot.Cursor > uint64(len(snapshot.Signals)) {
		return errors.New("invalid prepared Process snapshot")
	}
	for index, record := range snapshot.Signals {
		if record.Sequence != uint64(index+1) || !record.Signal.Valid() {
			return errors.New("invalid prepared Signal record")
		}
	}
	if snapshot.Prepared == nil {
		return nil
	}
	prepared := snapshot.Prepared
	if prepared.Sequence != snapshot.Step || !prepared.CandidateState.Valid() || !prepared.Transition.Valid() ||
		prepared.ConsumeThrough < snapshot.Cursor || prepared.ConsumeThrough > uint64(len(snapshot.Signals)) {
		return errors.New("invalid prepared Step")
	}
	lastStableDigest, err := digestExecutionState(snapshot.LastStable)
	if err != nil || lastStableDigest != prepared.LastStableDigest {
		return errors.New("prepared Step does not identify the last stable state")
	}
	transitionEffects := prepared.Transition.Effects()
	if len(transitionEffects) != len(prepared.Effects) {
		return errors.New("prepared Effect count does not match Transition")
	}
	for index, effect := range prepared.Effects {
		wantID := preparedEffectID(snapshot.ProcessID, prepared.Sequence, index)
		if effect.ID != wantID || string(effect.Effect.Payload()) != string(transitionEffects[index].Payload()) || effect.Effect.Target() != transitionEffects[index].Target() {
			return errors.New("prepared Effect identity or frozen payload changed")
		}
		if effect.Settlement != nil && effect.Settlement.EffectID() != effect.ID {
			return errors.New("prepared settlement addresses another Effect")
		}
	}
	return nil
}

func (engine *preparedSpikeEngine) enqueue(signal Signal) {
	engine.snapshot.Signals = append(engine.snapshot.Signals, preparedSpikeSignalRecord{
		Sequence: uint64(len(engine.snapshot.Signals) + 1),
		Signal:   signal,
	})
}

func (engine *preparedSpikeEngine) prepare(ctx context.Context) error {
	if engine.snapshot.Prepared != nil {
		return errors.New("a Step is already prepared")
	}
	pending := engine.snapshot.Signals[engine.snapshot.Cursor:]
	signals := make([]Signal, len(pending))
	for index := range pending {
		signals[index] = pending[index].Signal
	}
	transition, err := engine.execution.Step(ctx, signals)
	if err != nil {
		engine.execution, _ = engine.deployment.definition.Restore(engine.snapshot.LastStable)
		return err
	}
	if !transition.Valid() || uint64(transition.Consumed()) > uint64(len(signals)) {
		engine.execution, _ = engine.deployment.definition.Restore(engine.snapshot.LastStable)
		return errors.New("Step returned an invalid Transition")
	}
	candidate, err := engine.execution.Snapshot()
	if err != nil {
		engine.execution, _ = engine.deployment.definition.Restore(engine.snapshot.LastStable)
		return err
	}
	lastStableDigest, err := digestExecutionState(engine.snapshot.LastStable)
	if err != nil {
		return err
	}
	prepared := &preparedSpikeStep{
		Sequence:         engine.snapshot.Step,
		LastStableDigest: lastStableDigest,
		CandidateState:   candidate,
		ConsumeThrough:   engine.snapshot.Cursor + uint64(transition.Consumed()),
		Transition:       transition,
	}
	for index, effect := range transition.Effects() {
		prepared.Effects = append(prepared.Effects, preparedSpikeEffect{
			ID:     preparedEffectID(engine.snapshot.ProcessID, engine.snapshot.Step, index),
			Effect: effect,
		})
	}
	engine.snapshot.Prepared = prepared
	engine.acknowledged = len(prepared.Effects) == 0
	return nil
}

func (engine *preparedSpikeEngine) acknowledge(acknowledge func([]byte) error) error {
	if engine.snapshot.Prepared == nil {
		return errors.New("no prepared Step")
	}
	data, err := engine.capture()
	if err != nil {
		return err
	}
	if err := acknowledge(data); err != nil {
		return err
	}
	engine.acknowledged = true
	return nil
}

type preparedSpikeDispatcher struct {
	calls   int
	unknown bool
}

func (dispatcher *preparedSpikeDispatcher) dispatch(id EffectID, effect Effect) (Settlement, error) {
	dispatcher.calls++
	value, err := decodeJSON[preparedSpikeSignal](effect.Payload())
	if err != nil || value.Kind != "effect" {
		return Settlement{}, errors.New("invalid prepared spike Effect")
	}
	status := SettlementStatusSucceeded
	if dispatcher.unknown {
		status = SettlementStatusUnknown
	}
	payload, err := json.Marshal(preparedSpikeSignal{Kind: "result", Value: value.Value + ":done"})
	if err != nil {
		return Settlement{}, err
	}
	return NewSettlement(id, status, payload)
}

func (engine *preparedSpikeEngine) dispatchPrepared(dispatcher *preparedSpikeDispatcher) error {
	if engine.snapshot.Prepared == nil {
		return errors.New("no prepared Step")
	}
	if !engine.acknowledged {
		return errPreparedAcknowledgment
	}
	for index := range engine.snapshot.Prepared.Effects {
		prepared := &engine.snapshot.Prepared.Effects[index]
		if prepared.Settlement != nil {
			if prepared.Settlement.Status() == SettlementStatusUnknown {
				return errUnknownSettlement
			}
			continue
		}
		settlement, err := dispatcher.dispatch(prepared.ID, prepared.Effect)
		if err != nil {
			return err
		}
		prepared.Settlement = &settlement
		if settlement.Status() == SettlementStatusUnknown {
			return errUnknownSettlement
		}
	}
	return engine.finalize()
}

func (engine *preparedSpikeEngine) finalize() error {
	prepared := engine.snapshot.Prepared
	if prepared == nil {
		return errors.New("no prepared Step")
	}
	for _, effect := range prepared.Effects {
		if effect.Settlement == nil || effect.Settlement.Status() == SettlementStatusUnknown {
			return errors.New("prepared Effect is not definitely settled")
		}
		engine.enqueue(mustSignalFromSettlement(effect.ID, *effect.Settlement, uint64(len(engine.snapshot.Signals)+1)))
	}
	engine.snapshot.LastStable = prepared.CandidateState
	engine.snapshot.Cursor = prepared.ConsumeThrough
	transition := prepared.Transition
	engine.snapshot.Prepared = nil
	engine.snapshot.Step++
	engine.acknowledged = false
	execution, err := engine.deployment.definition.Restore(engine.snapshot.LastStable)
	if err != nil {
		return err
	}
	engine.execution = execution
	switch transition.Kind() {
	case TransitionKindContinue:
		engine.snapshot.Status = StatusRunning
	case TransitionKindComplete:
		output, _ := transition.Output()
		engine.output = output
		engine.snapshot.Status = StatusCompleted
	default:
		return errors.New("prepared spike does not implement this Transition kind")
	}
	return nil
}

func (engine *preparedSpikeEngine) capture() ([]byte, error) {
	return json.Marshal(engine.snapshot)
}

func digestExecutionState(state ExecutionState) (Digest, error) {
	data, err := json.Marshal(state)
	if err != nil {
		return Digest{}, err
	}
	return digestBytes(data), nil
}

func preparedEffectID(processID ProcessID, step uint64, index int) EffectID {
	return mustEffectID(processID.String() + ":step:" + strconv.FormatUint(step, 10) + ":effect:" + strconv.Itoa(index))
}

func mustSignalFromSettlement(effectID EffectID, settlement Settlement, sequence uint64) Signal {
	return mustSignalWithoutTest("signal:"+effectID.String(), WaitID{}, time.Unix(int64(sequence), 0), settlement.Payload())
}

func mustSignalWithoutTest(id string, waitID WaitID, receivedAt time.Time, payload json.RawMessage) Signal {
	signalID, err := ParseSignalID(id)
	if err != nil {
		panic(err)
	}
	signal, err := newSignal(signalID, waitID, receivedAt, payload)
	if err != nil {
		panic(err)
	}
	return signal
}

func decodeJSONInto(data []byte, target any) error {
	decoder := json.NewDecoder(bytesReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}

func bytesReader(data []byte) *bytes.Reader { return bytes.NewReader(data) }

func TestPreparedStepRequiresDurableAcknowledgmentBeforeDispatch(t *testing.T) {
	definition := newPreparedSpikeDefinition(t)
	deployment := newPreparedSpikeDeployment(t, definition, "implementation:v1", "configuration:v1")
	engine := newPreparedSpikeEngine(t, deployment, "work")
	startPayload, _ := json.Marshal(preparedSpikeSignal{Kind: "start"})
	engine.enqueue(mustSignal(t, "signal:start", WaitID{}, time.Unix(1, 0), startPayload))
	if err := engine.prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	dispatcher := &preparedSpikeDispatcher{}
	if err := engine.dispatchPrepared(dispatcher); !errors.Is(err, errPreparedAcknowledgment) {
		t.Fatalf("dispatch before acknowledgment error = %v", err)
	}
	if dispatcher.calls != 0 || engine.snapshot.Cursor != 0 || engine.snapshot.LastStable.Kind() != "prepared.spike" {
		t.Fatalf("pre-ack state changed: calls=%d cursor=%d", dispatcher.calls, engine.snapshot.Cursor)
	}

	var durableSnapshot []byte
	if err := engine.acknowledge(func(snapshot []byte) error {
		if dispatcher.calls != 0 {
			t.Fatal("Effect dispatched before durable acknowledgment")
		}
		durableSnapshot = append([]byte(nil), snapshot...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	restored, err := restorePreparedSpikeEngine(durableSnapshot, deployment)
	if err != nil {
		t.Fatal(err)
	}
	if err := restored.dispatchPrepared(dispatcher); err != nil {
		t.Fatal(err)
	}
	if dispatcher.calls != 1 || restored.snapshot.Cursor != 1 || restored.snapshot.Prepared != nil {
		t.Fatalf("finalize state: calls=%d cursor=%d prepared=%v", dispatcher.calls, restored.snapshot.Cursor, restored.snapshot.Prepared)
	}

	if err := restored.prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := restored.finalize(); err != nil {
		t.Fatal(err)
	}
	if restored.snapshot.Status != StatusCompleted {
		t.Fatalf("Process status = %s, want completed", restored.snapshot.Status)
	}
	output, err := DecodeOutput[preparedSpikeOutput](restored.output)
	if err != nil || output.Value != "work:done" {
		t.Fatalf("Process Output = %+v, %v", output, err)
	}
}

func TestPreparedStepFailureDiscardsInstanceWithoutConsumingSignal(t *testing.T) {
	definition := newPreparedSpikeDefinition(t)
	deployment := newPreparedSpikeDeployment(t, definition, "implementation:v1", "configuration:v1")
	engine := newPreparedSpikeEngine(t, deployment, "work")
	failPayload, _ := json.Marshal(preparedSpikeSignal{Kind: "fail"})
	engine.enqueue(mustSignal(t, "signal:fail", WaitID{}, time.Unix(1, 0), failPayload))
	if err := engine.prepare(context.Background()); err == nil {
		t.Fatal("prepare unexpectedly accepted failing Step")
	}
	if engine.snapshot.Cursor != 0 || engine.snapshot.Prepared != nil {
		t.Fatalf("failed Step consumed state: cursor=%d prepared=%v", engine.snapshot.Cursor, engine.snapshot.Prepared)
	}
	state, err := engine.execution.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	stateJSON, _ := json.Marshal(state)
	lastStableJSON, _ := json.Marshal(engine.snapshot.LastStable)
	if string(stateJSON) != string(lastStableJSON) {
		t.Fatalf("failed Execution instance was not rebuilt from last stable state")
	}
}

func TestPreparedSnapshotRejectsDifferentDeploymentAndDoesNotReplayUnknown(t *testing.T) {
	definition := newPreparedSpikeDefinition(t)
	deployment := newPreparedSpikeDeployment(t, definition, "implementation:v1", "configuration:v1")
	engine := newPreparedSpikeEngine(t, deployment, "work")
	startPayload, _ := json.Marshal(preparedSpikeSignal{Kind: "start"})
	engine.enqueue(mustSignal(t, "signal:start", WaitID{}, time.Unix(1, 0), startPayload))
	if err := engine.prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := engine.acknowledge(func([]byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	dispatcher := &preparedSpikeDispatcher{unknown: true}
	if err := engine.dispatchPrepared(dispatcher); !errors.Is(err, errUnknownSettlement) {
		t.Fatalf("unknown dispatch error = %v", err)
	}
	data, err := engine.capture()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := restorePreparedSpikeEngine(data, deployment)
	if err != nil {
		t.Fatal(err)
	}
	if err := restored.dispatchPrepared(dispatcher); !errors.Is(err, errUnknownSettlement) {
		t.Fatalf("restored unknown dispatch error = %v", err)
	}
	if dispatcher.calls != 1 {
		t.Fatalf("unknown Effect was implicitly replayed %d times", dispatcher.calls)
	}

	different := newPreparedSpikeDeployment(t, definition, "implementation:v2", "configuration:v1")
	if _, err := restorePreparedSpikeEngine(data, different); err == nil {
		t.Fatal("snapshot restored against a different exact Deployment")
	}
}
