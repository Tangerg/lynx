package agent2

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

const maxPauseReasonBytes = 4096

// ErrInvalidTransition reports a malformed or contradictory Step intent.
var ErrInvalidTransition = errors.New("agent: invalid transition")

// TransitionKind is the lifecycle intent produced by one bounded Step.
type TransitionKind uint8

const (
	// TransitionKindInvalid is the invalid zero value.
	TransitionKindInvalid TransitionKind = iota
	// TransitionKindContinue advances to another runnable Step.
	TransitionKindContinue
	// TransitionKindWait enters an Engine-minted wait.
	TransitionKindWait
	// TransitionKindPause enters an explicit scheduling pause.
	TransitionKindPause
	// TransitionKindComplete commits a validated semantic Output.
	TransitionKindComplete
	// TransitionKindFail commits a classified failure.
	TransitionKindFail
)

// String returns the stable Transition kind name.
func (kind TransitionKind) String() string {
	switch kind {
	case TransitionKindContinue:
		return "continue"
	case TransitionKindWait:
		return "wait"
	case TransitionKindPause:
		return "pause"
	case TransitionKindComplete:
		return "complete"
	case TransitionKindFail:
		return "fail"
	default:
		return "invalid"
	}
}

func parseTransitionKind(value string) (TransitionKind, error) {
	switch value {
	case "continue":
		return TransitionKindContinue, nil
	case "wait":
		return TransitionKindWait, nil
	case "pause":
		return TransitionKindPause, nil
	case "complete":
		return TransitionKindComplete, nil
	case "fail":
		return TransitionKindFail, nil
	default:
		return TransitionKindInvalid, fmt.Errorf("%w: unknown kind %q", ErrInvalidTransition, value)
	}
}

// Transition is an immutable candidate lifecycle intent. The Engine validates
// ConsumedSignals against the delivered Signal window, captures the candidate
// ExecutionState, and assigns EffectID values before committing anything.
type Transition struct {
	kind            TransitionKind
	consumedSignals uint32
	effects         []Effect
	waitID          WaitID
	reason          string
	output          Output
	failure         Failure
}

// Continue keeps the Process schedulable after consuming the stated Signal
// prefix. Effects are dispatched only after the Engine prepares the Step.
func Continue(consumedSignals uint32, effects ...Effect) (Transition, error) {
	owned, err := cloneEffects(effects)
	if err != nil {
		return Transition{}, err
	}
	return Transition{kind: TransitionKindContinue, consumedSignals: consumedSignals, effects: owned}, nil
}

// Wait moves the Process to Waiting for an Engine-minted WaitID already stored
// in the candidate ExecutionState.
func Wait(consumedSignals uint32, waitID WaitID) (Transition, error) {
	if !waitID.Valid() {
		return Transition{}, fmt.Errorf("%w: wait ID: %w", ErrInvalidTransition, ErrInvalidIdentity)
	}
	return Transition{kind: TransitionKindWait, consumedSignals: consumedSignals, waitID: waitID}, nil
}

// Pause requests an explicit scheduling pause with a bounded diagnostic reason.
func Pause(consumedSignals uint32, reason string) (Transition, error) {
	if reason == "" || strings.TrimSpace(reason) != reason || len(reason) > maxPauseReasonBytes {
		return Transition{}, fmt.Errorf("%w: pause reason must be non-empty, trimmed, and at most %d bytes", ErrInvalidTransition, maxPauseReasonBytes)
	}
	return Transition{kind: TransitionKindPause, consumedSignals: consumedSignals, reason: reason}, nil
}

// Complete supplies the final semantic Output. The Engine must validate it
// against the Definition Descriptor before committing Completed.
func Complete(consumedSignals uint32, output Output) (Transition, error) {
	if !output.Valid() {
		return Transition{}, fmt.Errorf("%w: output: %w", ErrInvalidTransition, ErrInvalidOutput)
	}
	return Transition{kind: TransitionKindComplete, consumedSignals: consumedSignals, output: output}, nil
}

// Fail supplies a stable Strategy-declared failure without making the
// Execution instance untrusted. A Step error follows the separate discard path.
func Fail(consumedSignals uint32, failure Failure) (Transition, error) {
	if !failure.Valid() {
		return Transition{}, fmt.Errorf("%w: failure: %w", ErrInvalidTransition, ErrInvalidFailure)
	}
	return Transition{kind: TransitionKindFail, consumedSignals: consumedSignals, failure: failure}, nil
}

// Kind returns the requested lifecycle intent.
func (t Transition) Kind() TransitionKind { return t.kind }

// ConsumedSignals returns the length of the delivered Signal prefix to commit.
func (t Transition) ConsumedSignals() uint32 { return t.consumedSignals }

// Effects returns independently owned operation intents in declaration order.
func (t Transition) Effects() []Effect { return cloneEffectsUnchecked(t.effects) }

// WaitID returns the wait target for a Wait transition.
func (t Transition) WaitID() (WaitID, bool) { return t.waitID, t.kind == TransitionKindWait }

// Reason returns the pause reason for a Pause transition.
func (t Transition) Reason() (string, bool) { return t.reason, t.kind == TransitionKindPause }

// Output returns the final result for a Complete transition.
func (t Transition) Output() (Output, bool) { return t.output, t.kind == TransitionKindComplete }

// Failure returns the terminal failure for a Fail transition.
func (t Transition) Failure() (Failure, bool) { return t.failure, t.kind == TransitionKindFail }

// Valid reports whether exactly the fields permitted by Kind are present.
func (t Transition) Valid() bool {
	switch t.kind {
	case TransitionKindContinue:
		return validEffects(t.effects) && !t.waitID.Valid() && t.reason == "" && !t.output.Valid() && !t.failure.Valid()
	case TransitionKindWait:
		return len(t.effects) == 0 && t.waitID.Valid() && t.reason == "" && !t.output.Valid() && !t.failure.Valid()
	case TransitionKindPause:
		return len(t.effects) == 0 && !t.waitID.Valid() && t.reason != "" && !t.output.Valid() && !t.failure.Valid()
	case TransitionKindComplete:
		return len(t.effects) == 0 && !t.waitID.Valid() && t.reason == "" && t.output.Valid() && !t.failure.Valid()
	case TransitionKindFail:
		return len(t.effects) == 0 && !t.waitID.Valid() && t.reason == "" && !t.output.Valid() && t.failure.Valid()
	default:
		return false
	}
}

func cloneEffects(effects []Effect) ([]Effect, error) {
	for _, effect := range effects {
		if !effect.Valid() {
			return nil, fmt.Errorf("%w: effect: %w", ErrInvalidTransition, ErrInvalidEffect)
		}
	}
	return cloneEffectsUnchecked(effects), nil
}

func cloneEffectsUnchecked(effects []Effect) []Effect {
	owned := slices.Clone(effects)
	for index := range owned {
		owned[index] = owned[index].clone()
	}
	return owned
}

func validEffects(effects []Effect) bool {
	for _, effect := range effects {
		if !effect.Valid() {
			return false
		}
	}
	return true
}

// MarshalJSON returns the validated immutable Step intent.
func (t Transition) MarshalJSON() ([]byte, error) {
	if !t.Valid() {
		return nil, ErrInvalidTransition
	}
	wire := transitionWire{Kind: t.kind.String(), ConsumedSignals: t.consumedSignals}
	switch t.kind {
	case TransitionKindContinue:
		wire.Effects = t.effects
	case TransitionKindWait:
		wire.WaitID = &t.waitID
	case TransitionKindPause:
		wire.Reason = t.reason
	case TransitionKindComplete:
		wire.Output = t.output.JSON()
	case TransitionKindFail:
		wire.Failure = &t.failure
	}
	return json.Marshal(wire)
}

// UnmarshalJSON replaces t with a strictly decoded Transition.
func (t *Transition) UnmarshalJSON(data []byte) error {
	if t == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidTransition)
	}
	var wire transitionWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidTransition, err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidTransition, err)
	}
	kind, err := parseTransitionKind(wire.Kind)
	if err != nil {
		return err
	}
	value, err := transitionFromWire(kind, wire)
	if err != nil {
		return err
	}
	*t = value
	return nil
}

func transitionFromWire(kind TransitionKind, wire transitionWire) (Transition, error) {
	switch kind {
	case TransitionKindContinue:
		if wire.WaitID != nil || wire.Reason != "" || len(wire.Output) > 0 || wire.Failure != nil {
			return Transition{}, fmt.Errorf("%w: continue contains fields owned by another transition kind", ErrInvalidTransition)
		}
		return Continue(wire.ConsumedSignals, wire.Effects...)
	case TransitionKindWait:
		if len(wire.Effects) > 0 || wire.WaitID == nil || wire.Reason != "" || len(wire.Output) > 0 || wire.Failure != nil {
			return Transition{}, fmt.Errorf("%w: wait has an invalid field set", ErrInvalidTransition)
		}
		return Wait(wire.ConsumedSignals, *wire.WaitID)
	case TransitionKindPause:
		if len(wire.Effects) > 0 || wire.WaitID != nil || len(wire.Output) > 0 || wire.Failure != nil {
			return Transition{}, fmt.Errorf("%w: pause has an invalid field set", ErrInvalidTransition)
		}
		return Pause(wire.ConsumedSignals, wire.Reason)
	case TransitionKindComplete:
		if len(wire.Effects) > 0 || wire.WaitID != nil || wire.Reason != "" || len(wire.Output) == 0 || wire.Failure != nil {
			return Transition{}, fmt.Errorf("%w: complete has an invalid field set", ErrInvalidTransition)
		}
		output, err := ParseOutput(wire.Output)
		if err != nil {
			return Transition{}, fmt.Errorf("%w: output: %w", ErrInvalidTransition, err)
		}
		return Complete(wire.ConsumedSignals, output)
	case TransitionKindFail:
		if len(wire.Effects) > 0 || wire.WaitID != nil || wire.Reason != "" || len(wire.Output) > 0 || wire.Failure == nil {
			return Transition{}, fmt.Errorf("%w: fail has an invalid field set", ErrInvalidTransition)
		}
		return Fail(wire.ConsumedSignals, *wire.Failure)
	default:
		return Transition{}, fmt.Errorf("%w: invalid kind", ErrInvalidTransition)
	}
}

type transitionWire struct {
	Kind            string          `json:"kind"`
	ConsumedSignals uint32          `json:"consumed_signals"`
	Effects         []Effect        `json:"effects,omitempty"`
	WaitID          *WaitID         `json:"wait_id,omitempty"`
	Reason          string          `json:"reason,omitempty"`
	Output          json.RawMessage `json:"output,omitempty"`
	Failure         *Failure        `json:"failure,omitempty"`
}
