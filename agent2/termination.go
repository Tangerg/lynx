package agent2

import (
	"errors"
	"fmt"
	"strings"
)

const maxTerminationReasonBytes = 4096

var ErrInvalidTermination = errors.New("agent: invalid termination")

// DeadlineOwner identifies the lifecycle boundary whose reached deadline is
// recorded by the Engine.
type DeadlineOwner uint8

const (
	DeadlineOwnerInvalid DeadlineOwner = iota
	DeadlineOwnerProcess
	DeadlineOwnerParent
	DeadlineOwnerHost
)

func (owner DeadlineOwner) String() string {
	switch owner {
	case DeadlineOwnerProcess:
		return "process"
	case DeadlineOwnerParent:
		return "parent"
	case DeadlineOwnerHost:
		return "host"
	default:
		return "invalid"
	}
}

// CancellationOwner identifies a non-deadline cancellation source.
type CancellationOwner uint8

const (
	CancellationOwnerInvalid CancellationOwner = iota
	CancellationOwnerParent
	CancellationOwnerHost
)

func (owner CancellationOwner) String() string {
	switch owner {
	case CancellationOwnerParent:
		return "parent"
	case CancellationOwnerHost:
		return "host"
	default:
		return "invalid"
	}
}

// KillIntent records an explicit Engine kill request.
type KillIntent struct{ reason string }

// NewKillIntent validates an explicit Engine kill reason.
func NewKillIntent(reason string) (KillIntent, error) {
	if err := validateTerminationReason(reason); err != nil {
		return KillIntent{}, err
	}
	return KillIntent{reason: reason}, nil
}

func (intent KillIntent) valid() bool { return intent.reason != "" }

// DeadlineIntent records that a specific Process lifecycle deadline was
// reached. A local Effect timeout remains a settlement Signal unless promoted
// to a Process termination before constructing these facts.
type DeadlineIntent struct {
	owner  DeadlineOwner
	reason string
}

// NewDeadlineIntent validates a reached lifecycle deadline.
func NewDeadlineIntent(owner DeadlineOwner, reason string) (DeadlineIntent, error) {
	if owner < DeadlineOwnerProcess || owner > DeadlineOwnerHost {
		return DeadlineIntent{}, fmt.Errorf("%w: invalid deadline owner", ErrInvalidTermination)
	}
	if err := validateTerminationReason(reason); err != nil {
		return DeadlineIntent{}, err
	}
	return DeadlineIntent{owner: owner, reason: reason}, nil
}

func (intent DeadlineIntent) valid() bool {
	return intent.owner >= DeadlineOwnerProcess && intent.owner <= DeadlineOwnerHost && intent.reason != ""
}

// CancellationIntent records a non-deadline cancellation from a parent Process
// or Host context.
type CancellationIntent struct {
	owner  CancellationOwner
	reason string
}

// NewCancellationIntent validates a non-deadline cancellation.
func NewCancellationIntent(owner CancellationOwner, reason string) (CancellationIntent, error) {
	if owner < CancellationOwnerParent || owner > CancellationOwnerHost {
		return CancellationIntent{}, fmt.Errorf("%w: invalid cancellation owner", ErrInvalidTermination)
	}
	if err := validateTerminationReason(reason); err != nil {
		return CancellationIntent{}, err
	}
	return CancellationIntent{owner: owner, reason: reason}, nil
}

func (intent CancellationIntent) valid() bool {
	return intent.owner >= CancellationOwnerParent && intent.owner <= CancellationOwnerHost && intent.reason != ""
}

// stepOutcomeKind describes the valid terminal result of a Step. A zero outcome
// is allowed only when control facts independently terminate a Process.
type stepOutcomeKind uint8

const (
	stepOutcomeNone stepOutcomeKind = iota
	stepOutcomeCompleted
	stepOutcomeFailed
)

// StepOutcome carries either legal completion or a classified failure.
type StepOutcome struct {
	kind    stepOutcomeKind
	failure Failure
}

// CompletedOutcome records a legal completion candidate.
func CompletedOutcome() StepOutcome { return StepOutcome{kind: stepOutcomeCompleted} }

// FailedOutcome records a classified failed-execution candidate.
func FailedOutcome(failure Failure) (StepOutcome, error) {
	if !failure.Valid() {
		return StepOutcome{}, fmt.Errorf("%w: failure: %w", ErrInvalidTermination, ErrInvalidFailure)
	}
	return StepOutcome{kind: stepOutcomeFailed, failure: failure}, nil
}

func (outcome StepOutcome) valid() bool {
	return outcome.kind == stepOutcomeNone && !outcome.failure.Valid() ||
		outcome.kind == stepOutcomeCompleted && !outcome.failure.Valid() ||
		outcome.kind == stepOutcomeFailed && outcome.failure.Valid()
}

// TerminationFacts are the independently recorded facts used by the Engine's
// terminal priority matrix. Presence is expressed by validated value objects,
// not by interpreting an error such as context.Canceled.
type TerminationFacts struct {
	Kill         KillIntent
	Deadline     DeadlineIntent
	Cancellation CancellationIntent
	Outcome      StepOutcome
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

// ResolveTermination applies Engine kill, deadline, cancellation, and Step
// outcome facts in that priority order. It never infers intent from an error.
func ResolveTermination(facts TerminationFacts) (Termination, error) {
	if !facts.Outcome.valid() {
		return Termination{}, fmt.Errorf("%w: invalid Step outcome", ErrInvalidTermination)
	}
	if facts.Kill.valid() {
		return Termination{status: StatusKilled, cause: TerminationCauseEngineKill, reason: facts.Kill.reason}, nil
	}
	if facts.Deadline.valid() {
		return terminationForDeadline(facts.Deadline), nil
	}
	if facts.Cancellation.valid() {
		return terminationForCancellation(facts.Cancellation), nil
	}
	switch facts.Outcome.kind {
	case stepOutcomeCompleted:
		return Termination{status: StatusCompleted, cause: TerminationCauseCompletion}, nil
	case stepOutcomeFailed:
		return terminationForFailure(facts.Outcome.failure), nil
	default:
		return Termination{}, fmt.Errorf("%w: no terminal fact was recorded", ErrInvalidTermination)
	}
}

func terminationForDeadline(intent DeadlineIntent) Termination {
	cause := TerminationCauseProcessDeadline
	switch intent.owner {
	case DeadlineOwnerParent:
		cause = TerminationCauseParentDeadline
	case DeadlineOwnerHost:
		cause = TerminationCauseHostDeadline
	}
	return Termination{status: StatusTimedOut, cause: cause, reason: intent.reason}
}

func terminationForCancellation(intent CancellationIntent) Termination {
	cause := TerminationCauseParentCancellation
	if intent.owner == CancellationOwnerHost {
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
		return fmt.Errorf("%w: reason must be non-empty, trimmed, and at most %d bytes", ErrInvalidTermination, maxTerminationReasonBytes)
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
