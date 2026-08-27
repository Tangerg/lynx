package agent

import (
	"context"
	"fmt"
)

// ProcessStartOutcomeStatus identifies the conclusive result of one accepted
// Process admission. The zero value is invalid.
type ProcessStartOutcomeStatus string

const (
	// ProcessStartOutcomeStatusInvalid is the invalid zero value.
	ProcessStartOutcomeStatusInvalid ProcessStartOutcomeStatus = ""
	// ProcessStartOutcomeStatusStarted means the prospective Process completed
	// initialization and is ready for Engine publication.
	ProcessStartOutcomeStatusStarted ProcessStartOutcomeStatus = "started"
	// ProcessStartOutcomeStatusAborted means initialization failed and no
	// Process will be published for the accepted admission.
	ProcessStartOutcomeStatusAborted ProcessStartOutcomeStatus = "aborted"
)

func (p ProcessStartOutcomeStatus) Valid() bool {
	return p == ProcessStartOutcomeStatusStarted || p == ProcessStartOutcomeStatusAborted
}

func (p ProcessStartOutcomeStatus) String() string {
	if !p.Valid() {
		return invalidEnumName
	}
	return string(p)
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
func (p ProcessStartOutcome) Admission() ProcessAdmission { return p.admission }

// Status returns the conclusive started or aborted initialization result.
func (p ProcessStartOutcome) Status() ProcessStartOutcomeStatus { return p.status }

// Failure returns the stable initialization failure for an aborted outcome.
func (p ProcessStartOutcome) Failure() (Failure, bool) {
	return p.failure, p.status == ProcessStartOutcomeStatusAborted
}

func (p ProcessStartOutcome) Valid() bool {
	if !p.admission.Valid() {
		return false
	}
	switch p.status {
	case ProcessStartOutcomeStatusStarted:
		return !p.failure.Valid()
	case ProcessStartOutcomeStatusAborted:
		return p.failure.Valid()
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
	// AcknowledgeProcessStartOutcome synchronously closes one previously accepted
	// admission as started or aborted. Returning an error for started prevents
	// Process publication; an aborted Process is never published regardless of
	// acknowledgment outcome. Implementations must be bounded, concurrency-safe,
	// idempotent by admission identity, and must not re-enter Engine or Process.
	AcknowledgeProcessStartOutcome(ctx context.Context, outcome ProcessStartOutcome) error
}

type ProcessStartOutcomeAcknowledgerFunc func(
	ctx context.Context,
	outcome ProcessStartOutcome,
) error

func (p ProcessStartOutcomeAcknowledgerFunc) AcknowledgeProcessStartOutcome(
	ctx context.Context,
	outcome ProcessStartOutcome,
) error {
	return p(ctx, outcome)
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
	ctx context.Context,
	acknowledger ProcessStartOutcomeAcknowledger,
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
