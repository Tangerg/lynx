package agent

import (
	"context"
	"fmt"
)

// ProcessStartOutcomeStatus identifies the conclusive result of one accepted
// Process admission. The zero value is invalid.
type ProcessStartOutcomeStatus uint8

const (
	// ProcessStartOutcomeStatusInvalid is the invalid zero value.
	ProcessStartOutcomeStatusInvalid ProcessStartOutcomeStatus = iota
	// ProcessStartOutcomeStatusStarted means the prospective Process completed
	// initialization and is ready for Engine publication.
	ProcessStartOutcomeStatusStarted
	// ProcessStartOutcomeStatusAborted means initialization failed and no
	// Process will be published for the accepted admission.
	ProcessStartOutcomeStatusAborted
)

// String returns the stable Process-start outcome status name.
func (status ProcessStartOutcomeStatus) String() string {
	switch status {
	case ProcessStartOutcomeStatusStarted:
		return "started"
	case ProcessStartOutcomeStatusAborted:
		return "aborted"
	default:
		return "invalid"
	}
}

// ProcessStartOutcome is the immutable conclusive Framework result for one
// accepted ProcessAdmission. A started outcome is acknowledged immediately
// before Engine publication; an aborted outcome guarantees no publication.
type ProcessStartOutcome struct {
	admission ProcessAdmission
	status    ProcessStartOutcomeStatus
	failure   Failure
}

// Admission returns the exact accepted admission concluded by this outcome.
func (outcome ProcessStartOutcome) Admission() ProcessAdmission { return outcome.admission }

// Status returns the conclusive started or aborted initialization result.
func (outcome ProcessStartOutcome) Status() ProcessStartOutcomeStatus { return outcome.status }

// Failure returns the stable initialization failure for an aborted outcome.
func (outcome ProcessStartOutcome) Failure() (Failure, bool) {
	return outcome.failure, outcome.status == ProcessStartOutcomeStatusAborted
}

// Valid reports whether the outcome conclusively matches one accepted
// admission. Only the Engine constructs outcome values.
func (outcome ProcessStartOutcome) Valid() bool {
	if !outcome.admission.Valid() {
		return false
	}
	switch outcome.status {
	case ProcessStartOutcomeStatusStarted:
		return !outcome.failure.Valid()
	case ProcessStartOutcomeStatusAborted:
		return outcome.failure.Valid()
	default:
		return false
	}
}

// ProcessStartOutcomeAcknowledger is the optional synchronous boundary that
// accepts exactly one conclusive result for every accepted admission.
// Implementations may be called concurrently for different Processes, must be
// idempotent for the same admission identity, must return in bounded time, and
// must not re-enter the Engine or any Process. Restore does not produce outcomes.
//
// Returning nil accepts the outcome. Rejecting a started outcome prevents
// publication; rejecting an aborted outcome cannot create a Process. The
// Framework owns no persistence, transaction, charging, or product semantics
// behind this neutral lifecycle handshake.
type ProcessStartOutcomeAcknowledger interface {
	AcknowledgeProcessStartOutcome(ctx context.Context, outcome ProcessStartOutcome) error
}

// ProcessStartOutcomeAcknowledgerFunc adapts a function to
// ProcessStartOutcomeAcknowledger.
type ProcessStartOutcomeAcknowledgerFunc func(
	ctx context.Context,
	outcome ProcessStartOutcome,
) error

// AcknowledgeProcessStartOutcome invokes acknowledger.
func (acknowledger ProcessStartOutcomeAcknowledgerFunc) AcknowledgeProcessStartOutcome(
	ctx context.Context,
	outcome ProcessStartOutcome,
) error {
	return acknowledger(ctx, outcome)
}

func startedProcessOutcome(admission ProcessAdmission) ProcessStartOutcome {
	return ProcessStartOutcome{admission: admission, status: ProcessStartOutcomeStatusStarted}
}

func abortedProcessOutcome(admission ProcessAdmission, failure Failure) ProcessStartOutcome {
	return ProcessStartOutcome{
		admission: admission,
		status:    ProcessStartOutcomeStatusAborted,
		failure:   failure,
	}
}

func acknowledgeProcessStartOutcome(
	acknowledger ProcessStartOutcomeAcknowledger,
	ctx context.Context,
	outcome ProcessStartOutcome,
) (err error) {
	if acknowledger == nil {
		return nil
	}
	if !outcome.Valid() {
		return fmt.Errorf("invalid Process start outcome")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("process-start outcome acknowledger panicked: %v", recovered)
		}
	}()
	return acknowledger.AcknowledgeProcessStartOutcome(
		context.WithoutCancel(contextOrBackground(ctx)), outcome,
	)
}
