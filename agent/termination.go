package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const maxTerminationReasonBytes = 4096

var errInvalidTermination = errors.New("agent: invalid termination")

// deadlineOwner identifies the lifecycle boundary whose reached deadline is
// recorded by the Engine.
type deadlineOwner string

const (
	deadlineOwnerInvalid deadlineOwner = ""
	deadlineOwnerProcess deadlineOwner = "process"
	deadlineOwnerParent  deadlineOwner = "parent"
	deadlineOwnerHost    deadlineOwner = "host"
)

func (owner deadlineOwner) valid() bool {
	return owner == deadlineOwnerProcess || owner == deadlineOwnerParent || owner == deadlineOwnerHost
}

func (owner deadlineOwner) String() string {
	if !owner.valid() {
		return "invalid"
	}
	return string(owner)
}

// cancellationOwner identifies a non-deadline cancellation source.
type cancellationOwner string

const (
	cancellationOwnerInvalid cancellationOwner = ""
	cancellationOwnerParent  cancellationOwner = "parent"
	cancellationOwnerHost    cancellationOwner = "host"
)

func (owner cancellationOwner) valid() bool {
	return owner == cancellationOwnerParent || owner == cancellationOwnerHost
}

func (owner cancellationOwner) String() string {
	if !owner.valid() {
		return "invalid"
	}
	return string(owner)
}

// killIntent records an explicit Engine kill request.
type killIntent struct{ reason string }

func newKillIntent(reason string) (killIntent, error) {
	if err := validateTerminationReason(reason); err != nil {
		return killIntent{}, err
	}
	return killIntent{reason: reason}, nil
}

func (intent killIntent) valid() bool { return intent.reason != "" }

// deadlineIntent records that a specific Process lifecycle deadline was
// reached. A local Effect timeout remains a settlement Signal unless promoted
// to a Process termination before constructing these facts.
type deadlineIntent struct {
	owner  deadlineOwner
	reason string
}

func newDeadlineIntent(owner deadlineOwner, reason string) (deadlineIntent, error) {
	if !owner.valid() {
		return deadlineIntent{}, fmt.Errorf("%w: invalid deadline owner", errInvalidTermination)
	}
	if err := validateTerminationReason(reason); err != nil {
		return deadlineIntent{}, err
	}
	return deadlineIntent{owner: owner, reason: reason}, nil
}

func (intent deadlineIntent) valid() bool {
	return intent.owner.valid() && intent.reason != ""
}

// cancellationIntent records a non-deadline cancellation from a parent Process
// or Host context.
type cancellationIntent struct {
	owner  cancellationOwner
	reason string
}

func newCancellationIntent(owner cancellationOwner, reason string) (cancellationIntent, error) {
	if !owner.valid() {
		return cancellationIntent{}, fmt.Errorf("%w: invalid cancellation owner", errInvalidTermination)
	}
	if err := validateTerminationReason(reason); err != nil {
		return cancellationIntent{}, err
	}
	return cancellationIntent{owner: owner, reason: reason}, nil
}

func (intent cancellationIntent) valid() bool {
	return intent.owner.valid() && intent.reason != ""
}

// stepOutcomeKind describes the valid terminal result of a Step. A zero outcome
// is allowed only when control facts independently terminate a Process.
type stepOutcomeKind uint8

const (
	stepOutcomeNone stepOutcomeKind = iota
	stepOutcomeCompleted
	stepOutcomeFailed
)

// stepOutcome carries either legal completion or a classified failure.
type stepOutcome struct {
	kind    stepOutcomeKind
	failure Failure
}

func completedOutcome() stepOutcome { return stepOutcome{kind: stepOutcomeCompleted} }

func failedOutcome(failure Failure) (stepOutcome, error) {
	if !failure.Valid() {
		return stepOutcome{}, fmt.Errorf("%w: failure: %w", errInvalidTermination, ErrInvalidFailure)
	}
	return stepOutcome{kind: stepOutcomeFailed, failure: failure}, nil
}

func (outcome stepOutcome) valid() bool {
	return outcome.kind == stepOutcomeNone && !outcome.failure.Valid() ||
		outcome.kind == stepOutcomeCompleted && !outcome.failure.Valid() ||
		outcome.kind == stepOutcomeFailed && outcome.failure.Valid()
}

// terminationFacts are the independently recorded facts used by the Engine's
// terminal priority matrix. Presence is expressed by validated value objects,
// not by interpreting an error such as context.Canceled.
type terminationFacts struct {
	kill         killIntent
	deadline     deadlineIntent
	cancellation cancellationIntent
	outcome      stepOutcome
}

// TerminationCause is the stable reason category of a terminal Process.
type TerminationCause string

const (
	// TerminationCauseInvalid is the invalid zero value.
	TerminationCauseInvalid TerminationCause = ""
	// TerminationCauseCompletion identifies successful semantic completion.
	TerminationCauseCompletion TerminationCause = "completion"
	// TerminationCauseEngineKill identifies an explicit Engine kill.
	TerminationCauseEngineKill TerminationCause = "engine_kill"
	// TerminationCauseProcessDeadline identifies the Process's own deadline.
	TerminationCauseProcessDeadline TerminationCause = "process_deadline"
	// TerminationCauseParentDeadline identifies deadline propagation from a parent.
	TerminationCauseParentDeadline TerminationCause = "parent_deadline"
	// TerminationCauseHostDeadline identifies expiry of the Host context.
	TerminationCauseHostDeadline TerminationCause = "host_deadline"
	// TerminationCauseParentCancellation identifies cancellation by a parent Process.
	TerminationCauseParentCancellation TerminationCause = "parent_cancellation"
	// TerminationCauseHostCancellation identifies cancellation by the Host context.
	TerminationCauseHostCancellation TerminationCause = "host_cancellation"
	// TerminationCauseExecutionFailure identifies an ordinary Strategy failure.
	TerminationCauseExecutionFailure TerminationCause = "execution_failure"
	// TerminationCauseContractFailure identifies a contract violation.
	TerminationCauseContractFailure TerminationCause = "contract_failure"
	// TerminationCauseExternalFailure identifies failed external infrastructure.
	TerminationCauseExternalFailure TerminationCause = "external_failure"
	// TerminationCausePanic identifies a recovered execution-boundary panic.
	TerminationCausePanic TerminationCause = "panic"
)

// Valid reports whether cause is a terminal Process category.
func (cause TerminationCause) Valid() bool {
	switch cause {
	case TerminationCauseCompletion, TerminationCauseEngineKill,
		TerminationCauseProcessDeadline, TerminationCauseParentDeadline,
		TerminationCauseHostDeadline, TerminationCauseParentCancellation,
		TerminationCauseHostCancellation, TerminationCauseExecutionFailure,
		TerminationCauseContractFailure, TerminationCauseExternalFailure,
		TerminationCausePanic:
		return true
	default:
		return false
	}
}

// String returns the stable termination-cause name.
func (cause TerminationCause) String() string {
	if !cause.Valid() {
		return "invalid"
	}
	return string(cause)
}

// Termination is the immutable result of applying the terminal priority matrix.
type Termination struct {
	status  Status
	cause   TerminationCause
	reason  string
	failure Failure
}

// resolveTermination applies Engine kill, deadline, cancellation, and Step
// outcome facts in that priority order. It never infers intent from an error.
func resolveTermination(facts terminationFacts) (Termination, error) {
	if !facts.outcome.valid() {
		return Termination{}, fmt.Errorf("%w: invalid Step outcome", errInvalidTermination)
	}
	if facts.kill.valid() {
		return Termination{status: StatusKilled, cause: TerminationCauseEngineKill, reason: facts.kill.reason}, nil
	}
	if facts.deadline.valid() {
		return terminationForDeadline(facts.deadline), nil
	}
	if facts.cancellation.valid() {
		return terminationForCancellation(facts.cancellation), nil
	}
	switch facts.outcome.kind {
	case stepOutcomeCompleted:
		return Termination{status: StatusCompleted, cause: TerminationCauseCompletion}, nil
	case stepOutcomeFailed:
		return terminationForFailure(facts.outcome.failure), nil
	default:
		return Termination{}, fmt.Errorf("%w: no terminal fact was recorded", errInvalidTermination)
	}
}

func terminationForDeadline(intent deadlineIntent) Termination {
	cause := TerminationCauseProcessDeadline
	switch intent.owner {
	case deadlineOwnerParent:
		cause = TerminationCauseParentDeadline
	case deadlineOwnerHost:
		cause = TerminationCauseHostDeadline
	}
	return Termination{status: StatusTimedOut, cause: cause, reason: intent.reason}
}

func terminationForCancellation(intent cancellationIntent) Termination {
	cause := TerminationCauseParentCancellation
	if intent.owner == cancellationOwnerHost {
		cause = TerminationCauseHostCancellation
	}
	return Termination{status: StatusCanceled, cause: cause, reason: intent.reason}
}

func terminationForFailure(failure Failure) Termination {
	cause := TerminationCauseExecutionFailure
	switch failure.Kind() {
	case FailureKindContract:
		cause = TerminationCauseContractFailure
	case FailureKindExternal:
		cause = TerminationCauseExternalFailure
	case FailureKindPanic:
		cause = TerminationCausePanic
	}
	return Termination{status: StatusFailed, cause: cause, reason: failure.Message(), failure: failure}
}

func validateTerminationReason(reason string) error {
	if reason == "" || strings.TrimSpace(reason) != reason || len(reason) > maxTerminationReasonBytes {
		return fmt.Errorf("%w: reason must be non-empty, trimmed, and at most %d bytes", errInvalidTermination, maxTerminationReasonBytes)
	}
	return nil
}

// Status returns the resolved terminal Process status.
func (termination Termination) Status() Status { return termination.status }

// Cause returns the stable machine-readable terminal category.
func (termination Termination) Cause() TerminationCause { return termination.cause }

// Reason returns a bounded diagnostic reason. Completion has an empty reason.
func (termination Termination) Reason() string { return termination.reason }

// Failure returns the classified failure for StatusFailed.
func (termination Termination) Failure() (Failure, bool) {
	return termination.failure, termination.status == StatusFailed
}

// Valid reports whether the resolved status, cause, and optional Failure agree.
func (termination Termination) Valid() bool {
	if !termination.status.Terminal() || !termination.cause.Valid() {
		return false
	}
	switch termination.status {
	case StatusCompleted:
		return termination.cause == TerminationCauseCompletion && termination.reason == "" && !termination.failure.Valid()
	case StatusFailed:
		if !termination.failure.Valid() || termination.reason != termination.failure.Message() {
			return false
		}
		return termination.cause == TerminationCauseExecutionFailure ||
			termination.cause == TerminationCauseContractFailure ||
			termination.cause == TerminationCauseExternalFailure ||
			termination.cause == TerminationCausePanic
	case StatusCanceled:
		return (termination.cause == TerminationCauseParentCancellation || termination.cause == TerminationCauseHostCancellation) &&
			termination.reason != "" && !termination.failure.Valid()
	case StatusTimedOut:
		return (termination.cause == TerminationCauseProcessDeadline || termination.cause == TerminationCauseParentDeadline || termination.cause == TerminationCauseHostDeadline) &&
			termination.reason != "" && !termination.failure.Valid()
	case StatusKilled:
		return termination.cause == TerminationCauseEngineKill && termination.reason != "" && !termination.failure.Valid()
	default:
		return false
	}
}

// MarshalJSON returns the validated immutable terminal fact.
func (termination Termination) MarshalJSON() ([]byte, error) {
	if !termination.Valid() {
		return nil, errInvalidTermination
	}
	wire := terminationWire{
		Status: termination.status,
		Cause:  termination.cause,
		Reason: termination.reason,
	}
	if termination.failure.Valid() {
		wire.Failure = &termination.failure
	}
	return json.Marshal(wire)
}

// UnmarshalJSON replaces termination with a strictly decoded terminal fact.
func (termination *Termination) UnmarshalJSON(data []byte) error {
	if termination == nil {
		return fmt.Errorf("%w: nil receiver", errInvalidTermination)
	}
	var wire terminationWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return fmt.Errorf("%w: decode: %w", errInvalidTermination, err)
	}
	if err := wireJSON.requireEOF(decoder); err != nil {
		return fmt.Errorf("%w: %w", errInvalidTermination, err)
	}
	value := Termination{status: wire.Status, cause: wire.Cause, reason: wire.Reason}
	if wire.Failure != nil {
		value.failure = *wire.Failure
	}
	if !value.Valid() {
		return errInvalidTermination
	}
	*termination = value
	return nil
}

type terminationWire struct {
	Status  Status           `json:"status"`
	Cause   TerminationCause `json:"cause"`
	Reason  string           `json:"reason,omitempty"`
	Failure *Failure         `json:"failure,omitempty"`
}
