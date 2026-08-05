package agent2

import (
	"errors"
	"fmt"
	"strings"
)

const maxTerminationReasonBytes = 4096

var errInvalidTermination = errors.New("agent: invalid termination")

// deadlineOwner identifies the lifecycle boundary whose reached deadline is
// recorded by the Engine.
type deadlineOwner uint8

const (
	deadlineOwnerInvalid deadlineOwner = iota
	deadlineOwnerProcess
	deadlineOwnerParent
	deadlineOwnerHost
)

func (owner deadlineOwner) String() string {
	switch owner {
	case deadlineOwnerProcess:
		return "process"
	case deadlineOwnerParent:
		return "parent"
	case deadlineOwnerHost:
		return "host"
	default:
		return "invalid"
	}
}

// cancellationOwner identifies a non-deadline cancellation source.
type cancellationOwner uint8

const (
	cancellationOwnerInvalid cancellationOwner = iota
	cancellationOwnerParent
	cancellationOwnerHost
)

func (owner cancellationOwner) String() string {
	switch owner {
	case cancellationOwnerParent:
		return "parent"
	case cancellationOwnerHost:
		return "host"
	default:
		return "invalid"
	}
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
	if owner < deadlineOwnerProcess || owner > deadlineOwnerHost {
		return deadlineIntent{}, fmt.Errorf("%w: invalid deadline owner", errInvalidTermination)
	}
	if err := validateTerminationReason(reason); err != nil {
		return deadlineIntent{}, err
	}
	return deadlineIntent{owner: owner, reason: reason}, nil
}

func (intent deadlineIntent) valid() bool {
	return intent.owner >= deadlineOwnerProcess && intent.owner <= deadlineOwnerHost && intent.reason != ""
}

// cancellationIntent records a non-deadline cancellation from a parent Process
// or Host context.
type cancellationIntent struct {
	owner  cancellationOwner
	reason string
}

func newCancellationIntent(owner cancellationOwner, reason string) (cancellationIntent, error) {
	if owner < cancellationOwnerParent || owner > cancellationOwnerHost {
		return cancellationIntent{}, fmt.Errorf("%w: invalid cancellation owner", errInvalidTermination)
	}
	if err := validateTerminationReason(reason); err != nil {
		return cancellationIntent{}, err
	}
	return cancellationIntent{owner: owner, reason: reason}, nil
}

func (intent cancellationIntent) valid() bool {
	return intent.owner >= cancellationOwnerParent && intent.owner <= cancellationOwnerHost && intent.reason != ""
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
type TerminationCause uint8

const (
	TerminationCauseInvalid TerminationCause = iota
	TerminationCauseCompletion
	TerminationCauseEngineKill
	TerminationCauseProcessDeadline
	TerminationCauseParentDeadline
	TerminationCauseHostDeadline
	TerminationCauseParentCancellation
	TerminationCauseHostCancellation
	TerminationCauseExecutionFailure
	TerminationCauseContractFailure
	TerminationCauseExternalFailure
	TerminationCausePanic
)

func (cause TerminationCause) String() string {
	switch cause {
	case TerminationCauseCompletion:
		return "completion"
	case TerminationCauseEngineKill:
		return "engine_kill"
	case TerminationCauseProcessDeadline:
		return "process_deadline"
	case TerminationCauseParentDeadline:
		return "parent_deadline"
	case TerminationCauseHostDeadline:
		return "host_deadline"
	case TerminationCauseParentCancellation:
		return "parent_cancellation"
	case TerminationCauseHostCancellation:
		return "host_cancellation"
	case TerminationCauseExecutionFailure:
		return "execution_failure"
	case TerminationCauseContractFailure:
		return "contract_failure"
	case TerminationCauseExternalFailure:
		return "external_failure"
	case TerminationCausePanic:
		return "panic"
	default:
		return "invalid"
	}
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
	return Termination{status: StatusCancelled, cause: cause, reason: intent.reason}
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
	if !termination.status.Terminal() || termination.cause == TerminationCauseInvalid {
		return false
	}
	if termination.status == StatusFailed {
		return termination.failure.Valid()
	}
	return !termination.failure.Valid()
}
