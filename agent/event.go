package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const maxEventBytes = 1 << 20

const (
	// EventProcessStarted reports initial Process execution.
	EventProcessStarted = "agent.process.started"
	// EventProcessRestored reports execution resumed from a Snapshot.
	EventProcessRestored = "agent.process.restored"
	// EventProcessPaused reports a committed scheduling pause.
	EventProcessPaused = "agent.process.paused"
	// EventProcessResumed reports committed scheduling resumption.
	EventProcessResumed = "agent.process.resumed"
	// EventProcessFinished reports one immutable terminal outcome.
	EventProcessFinished = "agent.process.finished"
	// EventSignalAccepted reports one newly accepted Signal.
	EventSignalAccepted = "agent.signal.accepted"
	// EventStepStarted reports an Execution.Step call about to begin.
	EventStepStarted = "agent.step.started"
	// EventStepFinished reports an Execution.Step return or failure.
	EventStepFinished = "agent.step.finished"
	// EventStepPrepared reports validated candidate Step state and fixed Effects.
	EventStepPrepared = "agent.step.prepared"
	// EventStepCommitted reports authoritative Step state publication.
	EventStepCommitted = "agent.step.committed"
	// EventEffectStarted reports a Framework or Dispatcher Effect attempt.
	EventEffectStarted = "agent.effect.started"
	// EventEffectFinished reports a definite or unknown attempt settlement.
	EventEffectFinished = "agent.effect.finished"
	// EventDeltaDropped reports best-effort increments lost to backpressure.
	EventDeltaDropped = "agent.delta.dropped"
)

var ErrInvalidEvent = errors.New("agent: invalid event")

// EventPhase distinguishes an attempted external operation from a fact that the
// Engine has committed into authoritative Process state.
type EventPhase string

const (
	// EventPhaseInvalid is the invalid zero value.
	EventPhaseInvalid EventPhase = ""
	// EventPhaseAttempt identifies work observed before authoritative commit.
	EventPhaseAttempt EventPhase = "attempt"
	// EventPhaseCommitted identifies a fact published after authoritative commit.
	EventPhaseCommitted EventPhase = "committed"
)

func (e EventPhase) Valid() bool {
	return e == EventPhaseAttempt || e == EventPhaseCommitted
}

func (e EventPhase) String() string {
	if !e.Valid() {
		return invalidEnumName
	}
	return string(e)
}

// Event is an immutable, ordered fact published by the Framework. Observers may
// project or instrument it, but observer failure never changes Process state.
// Payload is descriptive data, never a Signal or state mutation command.
type Event struct {
	processSequence uint64
	processID       ProcessID
	deploymentRef   DeploymentRef
	relation        ProcessRelation
	stepSequence    uint64
	effectID        EffectID
	name            string
	phase           EventPhase
	occurredAt      time.Time
	payload         json.RawMessage
}

func newEvent(
	processSequence uint64,
	processID ProcessID,
	deploymentRef DeploymentRef,
	relation ProcessRelation,
	stepSequence uint64,
	effectID EffectID,
	name string,
	phase EventPhase,
	occurredAt time.Time,
	payload json.RawMessage,
) (Event, error) {
	if processSequence == 0 {
		return Event{}, fmt.Errorf("%w: Process sequence must be greater than zero", ErrInvalidEvent)
	}
	if !processID.Valid() {
		return Event{}, fmt.Errorf("%w: process ID: %w", ErrInvalidEvent, ErrInvalidIdentity)
	}
	if !deploymentRef.Valid() {
		return Event{}, fmt.Errorf("%w: deployment: %w", ErrInvalidEvent, ErrInvalidDeploymentRef)
	}
	if !relation.Valid() || relation.ProcessID() != processID {
		return Event{}, fmt.Errorf("%w: relation: %w", ErrInvalidEvent, ErrInvalidProcessRelation)
	}
	if !validQualifiedName(name) {
		return Event{}, fmt.Errorf("%w: name must be a lowercase qualified name", ErrInvalidEvent)
	}
	if !phase.Valid() {
		return Event{}, fmt.Errorf("%w: phase is required", ErrInvalidEvent)
	}
	if occurredAt.IsZero() {
		return Event{}, fmt.Errorf("%w: occurrence time is required", ErrInvalidEvent)
	}
	normalized, err := wireJSON.normalize(payload, maxEventBytes)
	if err != nil {
		return Event{}, fmt.Errorf("%w: payload: %w", ErrInvalidEvent, err)
	}
	return Event{
		processSequence: processSequence,
		processID:       processID,
		deploymentRef:   deploymentRef,
		relation:        relation,
		stepSequence:    stepSequence,
		effectID:        effectID,
		name:            name,
		phase:           phase,
		occurredAt:      occurredAt.Round(0).UTC(),
		payload:         normalized,
	}, nil
}

// ProcessSequence returns the Process-local publication order.
func (e Event) ProcessSequence() uint64 { return e.processSequence }

// ProcessID returns the Process whose fact is described.
func (e Event) ProcessID() ProcessID { return e.processID }

// DeploymentRef returns the exact execution binding that emitted the fact.
func (e Event) DeploymentRef() DeploymentRef { return e.deploymentRef }

// Relation returns the Process tree location that emitted the fact.
func (e Event) Relation() ProcessRelation { return e.relation }

// StepSequence returns the one-based Step sequence and true, or zero and false
// for a Process fact outside a Step.
func (e Event) StepSequence() (uint64, bool) {
	return e.stepSequence, e.stepSequence > 0
}

// EffectID returns the related Effect identity and true when this is an Effect
// fact.
func (e Event) EffectID() (EffectID, bool) { return e.effectID, e.effectID.Valid() }

// Name returns the stable Framework fact name.
func (e Event) Name() string { return e.name }

// Phase returns whether the fact describes an attempt or committed state.
func (e Event) Phase() EventPhase { return e.phase }

// OccurredAt returns when the fact occurred.
func (e Event) OccurredAt() time.Time { return e.occurredAt }

// Payload returns an independently owned descriptive payload.
func (e Event) Payload() json.RawMessage { return bytes.Clone(e.payload) }

func (e Event) Valid() bool {
	return e.processSequence > 0 && e.processID.Valid() && e.deploymentRef.Valid() &&
		e.relation.Valid() && e.relation.ProcessID() == e.processID && validQualifiedName(e.name) &&
		e.phase.Valid() &&
		!e.occurredAt.IsZero() && len(e.payload) > 0
}

func (e Event) MarshalJSON() ([]byte, error) {
	if !e.Valid() {
		return nil, ErrInvalidEvent
	}
	wire := eventWire{
		ProcessSequence: e.processSequence,
		ProcessID:       e.processID,
		DeploymentRef:   e.deploymentRef,
		Relation:        e.relation.wire(),
		StepSequence:    e.stepSequence,
		Name:            e.name,
		Phase:           e.phase,
		OccurredAt:      e.occurredAt,
		Payload:         e.payload,
	}
	if e.effectID.Valid() {
		wire.EffectID = &e.effectID
	}
	return json.Marshal(wire)
}

func (e *Event) UnmarshalJSON(data []byte) error {
	if e == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidEvent)
	}
	wire, err := wireJSON.decode[eventWire](data)
	if err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidEvent, err)
	}
	var effectID EffectID
	if wire.EffectID != nil {
		effectID = *wire.EffectID
	}
	relation, err := processRelationFromWire(wire.ProcessID, wire.Relation)
	if err != nil {
		return fmt.Errorf("%w: relation: %w", ErrInvalidEvent, err)
	}
	value, err := newEvent(
		wire.ProcessSequence, wire.ProcessID, wire.DeploymentRef, relation, wire.StepSequence,
		effectID, wire.Name, wire.Phase, wire.OccurredAt, wire.Payload,
	)
	if err != nil {
		return err
	}
	*e = value
	return nil
}

type eventWire struct {
	ProcessSequence uint64              `json:"process_sequence"`
	ProcessID       ProcessID           `json:"process_id"`
	DeploymentRef   DeploymentRef       `json:"deployment_ref"`
	Relation        processRelationWire `json:"relation"`
	StepSequence    uint64              `json:"step_sequence,omitempty"`
	EffectID        *EffectID           `json:"effect_id,omitempty"`
	Name            string              `json:"name"`
	Phase           EventPhase          `json:"phase"`
	OccurredAt      time.Time           `json:"occurred_at"`
	Payload         json.RawMessage     `json:"payload"`
}
