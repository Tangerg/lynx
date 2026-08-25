package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const maxDeltaBytes = 1 << 20

// ErrInvalidDelta reports malformed best-effort observation data.
var ErrInvalidDelta = errors.New("agent: invalid delta")

// Delta is a bounded, best-effort stream increment from one Effect attempt.
// EffectSequence preserves producer order. Delta is never replayed from a
// snapshot and never contributes to the authoritative final Output.
type Delta struct {
	processID      ProcessID
	effectID       EffectID
	effectSequence uint64
	emittedAt      time.Time
	payload        json.RawMessage
}

func newDelta(processID ProcessID, effectID EffectID, effectSequence uint64, emittedAt time.Time, payload json.RawMessage) (Delta, error) {
	if !processID.Valid() || !effectID.Valid() {
		return Delta{}, fmt.Errorf("%w: process ID and effect ID are required", ErrInvalidDelta)
	}
	if effectSequence == 0 {
		return Delta{}, fmt.Errorf("%w: Effect sequence must be greater than zero", ErrInvalidDelta)
	}
	if emittedAt.IsZero() {
		return Delta{}, fmt.Errorf("%w: emission time is required", ErrInvalidDelta)
	}
	normalized, err := wireJSON.normalize(payload, maxDeltaBytes)
	if err != nil {
		return Delta{}, fmt.Errorf("%w: payload: %w", ErrInvalidDelta, err)
	}
	return Delta{
		processID:      processID,
		effectID:       effectID,
		effectSequence: effectSequence,
		emittedAt:      emittedAt.Round(0).UTC(),
		payload:        normalized,
	}, nil
}

// ProcessID returns the Process that owns the Effect attempt.
func (d Delta) ProcessID() ProcessID { return d.processID }

// EffectID returns the Effect attempt that emitted the increment.
func (d Delta) EffectID() EffectID { return d.effectID }

// EffectSequence returns the one-based producer order within the Effect attempt.
func (d Delta) EffectSequence() uint64 { return d.effectSequence }

// EmittedAt returns when the producer emitted the increment.
func (d Delta) EmittedAt() time.Time { return d.emittedAt }

// Payload returns an independently owned Strategy-defined increment.
func (d Delta) Payload() json.RawMessage { return bytes.Clone(d.payload) }

// Valid reports whether the Delta has a complete immutable envelope.
func (d Delta) Valid() bool {
	return d.processID.Valid() && d.effectID.Valid() && d.effectSequence > 0 &&
		!d.emittedAt.IsZero() && len(d.payload) > 0
}

// MarshalJSON returns the validated Delta wire representation.
func (d Delta) MarshalJSON() ([]byte, error) {
	if !d.Valid() {
		return nil, ErrInvalidDelta
	}
	return json.Marshal(deltaWire{
		ProcessID:      d.processID,
		EffectID:       d.effectID,
		EffectSequence: d.effectSequence,
		EmittedAt:      d.emittedAt,
		Payload:        d.payload,
	})
}

// UnmarshalJSON replaces d with a strictly decoded Delta.
func (d *Delta) UnmarshalJSON(data []byte) error {
	if d == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidDelta)
	}
	var wire deltaWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidDelta, err)
	}
	if err := wireJSON.requireEOF(decoder); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidDelta, err)
	}
	value, err := newDelta(wire.ProcessID, wire.EffectID, wire.EffectSequence, wire.EmittedAt, wire.Payload)
	if err != nil {
		return err
	}
	*d = value
	return nil
}

type deltaWire struct {
	ProcessID      ProcessID       `json:"process_id"`
	EffectID       EffectID        `json:"effect_id"`
	EffectSequence uint64          `json:"effect_sequence"`
	EmittedAt      time.Time       `json:"emitted_at"`
	Payload        json.RawMessage `json:"payload"`
}
