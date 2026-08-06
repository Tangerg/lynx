package agent2

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
)

var ErrInvalidChildWait = errors.New("agent: invalid child wait")

// ChildWaitCondition identifies when a set of child Processes releases its
// parent. It describes completion count only; it does not imply cancellation
// of unfinished children or reinterpret child terminal statuses.
type ChildWaitCondition struct {
	kind   childWaitKind
	quorum uint32
}

type childWaitKind string

const (
	childWaitAll    childWaitKind = "all"
	childWaitAny    childWaitKind = "any"
	childWaitQuorum childWaitKind = "quorum"
)

// AllChildren waits until every named child is terminal.
func AllChildren() ChildWaitCondition { return ChildWaitCondition{kind: childWaitAll} }

// AnyChild waits until at least one named child is terminal.
func AnyChild() ChildWaitCondition { return ChildWaitCondition{kind: childWaitAny} }

// ChildQuorum waits until count named children are terminal.
func ChildQuorum(count uint32) (ChildWaitCondition, error) {
	condition := ChildWaitCondition{kind: childWaitQuorum, quorum: count}
	if count == 0 {
		return ChildWaitCondition{}, ErrInvalidChildWait
	}
	return condition, nil
}

// Valid reports whether condition is one supported completion predicate.
func (condition ChildWaitCondition) Valid() bool {
	return condition.kind == childWaitAll && condition.quorum == 0 ||
		condition.kind == childWaitAny && condition.quorum == 0 ||
		condition.kind == childWaitQuorum && condition.quorum > 0
}

func (condition ChildWaitCondition) required(total int) (uint32, error) {
	if !condition.Valid() || total <= 0 || uint64(total) > uint64(^uint32(0)) {
		return 0, ErrInvalidChildWait
	}
	switch condition.kind {
	case childWaitAll:
		return uint32(total), nil
	case childWaitAny:
		return 1, nil
	case childWaitQuorum:
		if condition.quorum > uint32(total) {
			return 0, ErrInvalidChildWait
		}
		return condition.quorum, nil
	default:
		return 0, ErrInvalidChildWait
	}
}

// ChildWaitSpec names one stable logical wait, its direct children in result
// order, and the completion predicate.
type ChildWaitSpec struct {
	Key       WaitKey
	Children  []ProcessID
	Condition ChildWaitCondition
}

// Valid reports whether the wait has a usable key, unique child identities,
// and a condition satisfiable by the declared child count.
func (spec ChildWaitSpec) Valid() bool {
	if !spec.Key.Valid() {
		return false
	}
	if _, err := spec.Condition.required(len(spec.Children)); err != nil {
		return false
	}
	seen := make(map[ProcessID]struct{}, len(spec.Children))
	for _, childID := range spec.Children {
		if !childID.Valid() {
			return false
		}
		if _, duplicate := seen[childID]; duplicate {
			return false
		}
		seen[childID] = struct{}{}
	}
	return true
}

// WaitForChildren creates a Framework Effect that opens an Engine-owned wait
// over direct children. Dispatch returns immediately with a WaitID; child work
// never blocks Execution.Step or holds a prepared Step open.
func WaitForChildren(spec ChildWaitSpec) (Effect, error) {
	if !spec.Valid() {
		return Effect{}, ErrInvalidChildWait
	}
	payload, err := json.Marshal(childWaitEffectWire{
		Operation: frameworkEffectWaitChildren, SchemaVersion: frameworkEffectSchemaVersion,
		Spec: childWaitSpecWireFromValue(spec),
	})
	if err != nil {
		return Effect{}, fmt.Errorf("%w: encode request: %v", ErrInvalidChildWait, err)
	}
	return newEffect(EffectTargetFramework, payload)
}

// ChildWaitOpened is the definite acknowledgement that the Engine registered
// a child wait and minted its WaitID.
type ChildWaitOpened struct {
	waitID WaitID
	spec   ChildWaitSpec
}

// WaitID returns the Engine-minted wait identity to store in Execution state.
func (opened ChildWaitOpened) WaitID() WaitID { return opened.waitID }

// Valid reports whether the acknowledgement contains one complete wait.
func (opened ChildWaitOpened) Valid() bool { return opened.waitID.Valid() && opened.spec.Valid() }

// ParseChildWaitOpened decodes the settlement Signal produced by
// WaitForChildren and verifies its Engine-attached WaitID.
func ParseChildWaitOpened(signal Signal) (ChildWaitOpened, error) {
	waitID, addressed := signal.WaitID()
	if !signal.Valid() || !addressed {
		return ChildWaitOpened{}, ErrInvalidChildWait
	}
	var wire childWaitOpenedWire
	if err := decodeStrictJSON(signal.Payload(), &wire); err != nil {
		return ChildWaitOpened{}, fmt.Errorf("%w: decode opened Signal: %v", ErrInvalidChildWait, err)
	}
	spec, err := wire.Spec.value()
	if err != nil || wire.SchemaVersion != childProtocolSchemaVersion ||
		wire.Operation != childSignalWaitOpened {
		return ChildWaitOpened{}, ErrInvalidChildWait
	}
	opened := ChildWaitOpened{waitID: waitID, spec: spec}
	if !opened.Valid() {
		return ChildWaitOpened{}, ErrInvalidChildWait
	}
	return opened, nil
}

// ChildOutcome pairs a parent's logical ChildKey with one immutable terminal
// Process Result.
type ChildOutcome struct {
	key    ChildKey
	result Result
}

// Key returns the parent-scoped logical child identity.
func (outcome ChildOutcome) Key() ChildKey { return outcome.key }

// Result returns the child's immutable terminal result.
func (outcome ChildOutcome) Result() Result { return outcome.result }

// Valid reports whether both logical identity and terminal result are complete.
func (outcome ChildOutcome) Valid() bool { return outcome.key.Valid() && outcome.result.Valid() }

// ChildrenCompleted is one condition-satisfying, request-ordered child result
// set. For any or quorum it includes every child already terminal at the atomic
// satisfaction check, without cancelling or omitting based on status.
type ChildrenCompleted struct {
	waitID   WaitID
	key      WaitKey
	outcomes []ChildOutcome
}

// WaitID returns the addressed wait identity.
func (completed ChildrenCompleted) WaitID() WaitID { return completed.waitID }

// Key returns the logical wait key declared by the Execution.
func (completed ChildrenCompleted) Key() WaitKey { return completed.key }

// Outcomes returns terminal children in the original ChildWaitSpec order.
func (completed ChildrenCompleted) Outcomes() []ChildOutcome {
	return slices.Clone(completed.outcomes)
}

// Valid reports whether the completion contains at least one ordered result.
func (completed ChildrenCompleted) Valid() bool {
	if !completed.waitID.Valid() || !completed.key.Valid() || len(completed.outcomes) == 0 {
		return false
	}
	seen := make(map[ProcessID]struct{}, len(completed.outcomes))
	for _, outcome := range completed.outcomes {
		if !outcome.Valid() {
			return false
		}
		if _, duplicate := seen[outcome.result.ProcessID()]; duplicate {
			return false
		}
		seen[outcome.result.ProcessID()] = struct{}{}
	}
	return true
}

// ParseChildrenCompleted decodes an Engine-generated, WaitID-addressed child
// completion Signal.
func ParseChildrenCompleted(signal Signal) (ChildrenCompleted, error) {
	waitID, addressed := signal.WaitID()
	if !signal.Valid() || !addressed {
		return ChildrenCompleted{}, ErrInvalidChildWait
	}
	var wire childrenCompletedWire
	if err := decodeStrictJSON(signal.Payload(), &wire); err != nil {
		return ChildrenCompleted{}, fmt.Errorf("%w: decode completion Signal: %v", ErrInvalidChildWait, err)
	}
	if wire.SchemaVersion != childProtocolSchemaVersion ||
		wire.Operation != childSignalChildrenCompleted || !wire.Key.Valid() || len(wire.Outcomes) == 0 {
		return ChildrenCompleted{}, ErrInvalidChildWait
	}
	completed := ChildrenCompleted{waitID: waitID, key: wire.Key}
	for _, encoded := range wire.Outcomes {
		outcome, err := encoded.value()
		if err != nil {
			return ChildrenCompleted{}, err
		}
		completed.outcomes = append(completed.outcomes, outcome)
	}
	if !completed.Valid() {
		return ChildrenCompleted{}, ErrInvalidChildWait
	}
	return completed, nil
}

const (
	childSignalWaitOpened        = "child_wait_opened"
	childSignalChildrenCompleted = "children_completed"
)

type childWaitConditionWire struct {
	Kind   childWaitKind `json:"kind"`
	Quorum uint32        `json:"quorum,omitempty"`
}

type childWaitSpecWire struct {
	Key       WaitKey                `json:"key"`
	Children  []ProcessID            `json:"children"`
	Condition childWaitConditionWire `json:"condition"`
}

type childWaitEffectWire struct {
	Operation     string            `json:"operation"`
	SchemaVersion uint16            `json:"schema_version"`
	Spec          childWaitSpecWire `json:"spec"`
}

type childWaitOpenedWire struct {
	SchemaVersion uint16            `json:"schema_version"`
	Operation     string            `json:"operation"`
	Spec          childWaitSpecWire `json:"spec"`
}

type childrenCompletedWire struct {
	SchemaVersion uint16             `json:"schema_version"`
	Operation     string             `json:"operation"`
	Key           WaitKey            `json:"key"`
	Outcomes      []childOutcomeWire `json:"outcomes"`
}

type childOutcomeWire struct {
	Key    ChildKey   `json:"key"`
	Result resultWire `json:"result"`
}

type resultWire struct {
	ProcessID   ProcessID   `json:"process_id"`
	StartedAt   time.Time   `json:"started_at"`
	FinishedAt  time.Time   `json:"finished_at"`
	Output      *Output     `json:"output,omitempty"`
	Termination Termination `json:"termination"`
	Usage       Usage       `json:"usage"`
}

func childWaitSpecWireFromValue(spec ChildWaitSpec) childWaitSpecWire {
	return childWaitSpecWire{
		Key: spec.Key, Children: slices.Clone(spec.Children),
		Condition: childWaitConditionWire{Kind: spec.Condition.kind, Quorum: spec.Condition.quorum},
	}
}

func (wire childWaitSpecWire) value() (ChildWaitSpec, error) {
	spec := ChildWaitSpec{
		Key: wire.Key, Children: slices.Clone(wire.Children),
		Condition: ChildWaitCondition{kind: wire.Condition.Kind, quorum: wire.Condition.Quorum},
	}
	if !spec.Valid() {
		return ChildWaitSpec{}, ErrInvalidChildWait
	}
	return spec, nil
}

func cloneChildWaitSpec(spec ChildWaitSpec) ChildWaitSpec {
	cloned := spec
	cloned.Children = slices.Clone(spec.Children)
	return cloned
}

func decodeChildWaitEffect(payload json.RawMessage) (ChildWaitSpec, error) {
	var wire childWaitEffectWire
	if err := decodeStrictJSON(payload, &wire); err != nil {
		return ChildWaitSpec{}, fmt.Errorf("%w: decode request: %v", ErrInvalidChildWait, err)
	}
	if wire.Operation != frameworkEffectWaitChildren || wire.SchemaVersion != frameworkEffectSchemaVersion {
		return ChildWaitSpec{}, ErrInvalidChildWait
	}
	return wire.Spec.value()
}

func encodeChildWaitOpened(spec ChildWaitSpec) (json.RawMessage, error) {
	if !spec.Valid() {
		return nil, ErrInvalidChildWait
	}
	return json.Marshal(childWaitOpenedWire{
		SchemaVersion: childProtocolSchemaVersion,
		Operation:     childSignalWaitOpened,
		Spec:          childWaitSpecWireFromValue(spec),
	})
}

func (wire childOutcomeWire) value() (ChildOutcome, error) {
	result, err := wire.Result.value()
	if err != nil {
		return ChildOutcome{}, err
	}
	outcome := ChildOutcome{key: wire.Key, result: result}
	if !outcome.Valid() {
		return ChildOutcome{}, ErrInvalidChildWait
	}
	return outcome, nil
}

func resultWireFromValue(result Result) resultWire {
	wire := resultWire{
		ProcessID: result.processID, StartedAt: result.startedAt, FinishedAt: result.finishedAt,
		Termination: result.termination, Usage: result.usage,
	}
	if result.output.Valid() {
		output := result.output
		wire.Output = &output
	}
	return wire
}

func (wire resultWire) value() (Result, error) {
	var output Output
	if wire.Output != nil {
		output = *wire.Output
	}
	result := Result{
		processID: wire.ProcessID, startedAt: wire.StartedAt, finishedAt: wire.FinishedAt,
		output: output, termination: wire.Termination, usage: wire.Usage,
	}
	if !result.Valid() {
		return Result{}, ErrInvalidChildWait
	}
	return result, nil
}

func encodeChildrenCompleted(
	waitID WaitID,
	key WaitKey,
	outcomes []ChildOutcome,
) (Signal, error) {
	completed := ChildrenCompleted{waitID: waitID, key: key, outcomes: slices.Clone(outcomes)}
	if !completed.Valid() {
		return Signal{}, ErrInvalidChildWait
	}
	wire := childrenCompletedWire{
		SchemaVersion: childProtocolSchemaVersion,
		Operation:     childSignalChildrenCompleted,
		Key:           key,
		Outcomes:      make([]childOutcomeWire, len(outcomes)),
	}
	for index, outcome := range outcomes {
		wire.Outcomes[index] = childOutcomeWire{
			Key: outcome.key, Result: resultWireFromValue(outcome.result),
		}
	}
	payload, err := json.Marshal(wire)
	if err != nil {
		return Signal{}, err
	}
	return newSignal(deriveChildCompletionSignalID(waitID), waitID, time.Now(), payload)
}

func deriveChildCompletionSignalID(waitID WaitID) SignalID {
	digest := digestBytes([]byte("children-completed\x00" + waitID.String()))
	id, err := ParseSignalID("signal:" + digest.String()[len("sha256:"):])
	if err != nil {
		panic(err)
	}
	return id
}
