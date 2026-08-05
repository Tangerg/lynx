package agent2

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const maxEventBytes = 1 << 20

var ErrInvalidEvent = errors.New("agent: invalid event")

// EventPhase distinguishes an attempted external operation from a fact that the
// Engine has committed into authoritative Process state.
type EventPhase uint8

const (
	EventPhaseInvalid EventPhase = iota
	EventPhaseAttempt
	EventPhaseCommitted
)

func (phase EventPhase) String() string {
	switch phase {
	case EventPhaseAttempt:
		return "attempt"
	case EventPhaseCommitted:
		return "committed"
	default:
		return "invalid"
	}
}

func parseEventPhase(value string) (EventPhase, error) {
	switch value {
	case "attempt":
		return EventPhaseAttempt, nil
	case "committed":
		return EventPhaseCommitted, nil
	default:
		return EventPhaseInvalid, fmt.Errorf("%w: unknown phase %q", ErrInvalidEvent, value)
	}
}

// Event is an immutable, ordered fact published by the Framework. Observers may
// project or instrument it, but observer failure never changes Process state.
// Payload is descriptive data, never a Signal or state mutation command.
type Event struct {
	sequence   uint64
	processID  ProcessID
	step       uint64
	effectID   EffectID
	name       string
	phase      EventPhase
	occurredAt time.Time
	payload    json.RawMessage
}

func newEvent(sequence uint64, processID ProcessID, step uint64, effectID EffectID, name string, phase EventPhase, occurredAt time.Time, payload json.RawMessage) (Event, error) {
	if sequence == 0 {
		return Event{}, fmt.Errorf("%w: sequence must be greater than zero", ErrInvalidEvent)
	}
	if !processID.Valid() {
		return Event{}, fmt.Errorf("%w: process ID: %w", ErrInvalidEvent, ErrInvalidIdentity)
	}
	if !validQualifiedName(name) {
		return Event{}, fmt.Errorf("%w: name must be a lowercase qualified name", ErrInvalidEvent)
	}
	if phase != EventPhaseAttempt && phase != EventPhaseCommitted {
		return Event{}, fmt.Errorf("%w: phase is required", ErrInvalidEvent)
	}
	if occurredAt.IsZero() {
		return Event{}, fmt.Errorf("%w: occurrence time is required", ErrInvalidEvent)
	}
	normalized, err := normalizeJSON(payload, maxEventBytes)
	if err != nil {
		return Event{}, fmt.Errorf("%w: payload: %w", ErrInvalidEvent, err)
	}
	return Event{
		sequence:   sequence,
		processID:  processID,
		step:       step,
		effectID:   effectID,
		name:       name,
		phase:      phase,
		occurredAt: occurredAt.Round(0).UTC(),
		payload:    normalized,
	}, nil
}

// Sequence returns the Process-local publication order.
func (e Event) Sequence() uint64 { return e.sequence }

// ProcessID returns the Process whose fact is described.
func (e Event) ProcessID() ProcessID { return e.processID }

// StepSequence returns the one-based Step sequence and true, or zero and false
// for a Process fact outside a Step.
func (e Event) StepSequence() (uint64, bool) { return e.step, e.step > 0 }

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

// Valid reports whether the Event has a complete immutable envelope.
func (e Event) Valid() bool {
	return e.sequence > 0 && e.processID.Valid() && validQualifiedName(e.name) &&
		(e.phase == EventPhaseAttempt || e.phase == EventPhaseCommitted) &&
		!e.occurredAt.IsZero() && len(e.payload) > 0
}

func (e Event) MarshalJSON() ([]byte, error) {
	if !e.Valid() {
		return nil, ErrInvalidEvent
	}
	wire := eventWire{
		Sequence:   e.sequence,
		ProcessID:  e.processID,
		Step:       e.step,
		Name:       e.name,
		Phase:      e.phase.String(),
		OccurredAt: e.occurredAt,
		Payload:    e.payload,
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
	var wire eventWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidEvent, err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidEvent, err)
	}
	phase, err := parseEventPhase(wire.Phase)
	if err != nil {
		return err
	}
	var effectID EffectID
	if wire.EffectID != nil {
		effectID = *wire.EffectID
	}
	value, err := newEvent(wire.Sequence, wire.ProcessID, wire.Step, effectID, wire.Name, phase, wire.OccurredAt, wire.Payload)
	if err != nil {
		return err
	}
	*e = value
	return nil
}

type eventWire struct {
	Sequence   uint64          `json:"sequence"`
	ProcessID  ProcessID       `json:"process_id"`
	Step       uint64          `json:"step_sequence,omitempty"`
	EffectID   *EffectID       `json:"effect_id,omitempty"`
	Name       string          `json:"name"`
	Phase      string          `json:"phase"`
	OccurredAt time.Time       `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload"`
}
