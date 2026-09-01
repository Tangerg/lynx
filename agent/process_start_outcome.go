package agent

import (
	"context"
	"errors"
	"fmt"
	"time"
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
	admission          ProcessAdmission
	status             ProcessStartOutcomeStatus
	startedAt          time.Time
	failure            Failure
	previousTreeDigest Digest
	hasPreviousTree    bool
	treeSnapshot       TreeSnapshot
	hasTreeSnapshot    bool
}

// Admission returns the exact accepted admission concluded by this outcome.
func (p ProcessStartOutcome) Admission() ProcessAdmission { return p.admission }

// Status returns the conclusive started or aborted initialization result.
func (p ProcessStartOutcome) Status() ProcessStartOutcomeStatus { return p.status }

// StartedAt returns the authoritative UTC lifecycle time for a started
// Process. Aborted outcomes return false because no Process lifecycle began.
func (p ProcessStartOutcome) StartedAt() (time.Time, bool) {
	if p.status != ProcessStartOutcomeStatusStarted || p.startedAt.IsZero() {
		return time.Time{}, false
	}
	return p.startedAt, true
}

// Failure returns the stable initialization failure for an aborted outcome.
func (p ProcessStartOutcome) Failure() (Failure, bool) {
	return p.failure, p.status == ProcessStartOutcomeStatusAborted
}

// PreviousTreeDigest returns the authoritative head compared by a durable
// child outcome. Root and ephemeral outcomes return false.
func (p ProcessStartOutcome) PreviousTreeDigest() (Digest, bool) {
	return p.previousTreeDigest, p.hasPreviousTree
}

// TreeSnapshot returns the prospective complete tree installed atomically by
// a durable started or child-aborted outcome. Ephemeral and root-aborted
// outcomes return false.
func (p ProcessStartOutcome) TreeSnapshot() (TreeSnapshot, bool) {
	return p.treeSnapshot, p.hasTreeSnapshot
}

func (p ProcessStartOutcome) Valid() bool {
	if !p.admission.Valid() {
		return false
	}
	if p.hasPreviousTree != p.previousTreeDigest.Valid() ||
		p.hasTreeSnapshot != p.treeSnapshot.Valid() ||
		p.hasPreviousTree && !p.hasTreeSnapshot {
		return false
	}
	if p.hasTreeSnapshot && p.treeSnapshot.RootID() != p.admission.Relation().RootID() {
		return false
	}
	if p.hasPreviousTree && p.previousTreeDigest == p.treeSnapshot.Digest() {
		return false
	}
	if p.hasTreeSnapshot {
		if _, durable := p.treeSnapshot.IncarnationID(); !durable {
			return false
		}
		containsProcess := false
		for _, snapshot := range p.treeSnapshot.ProcessSnapshots() {
			if snapshot.ProcessID() == p.admission.Relation().ProcessID() {
				containsProcess = true
				break
			}
		}
		if containsProcess != (p.status == ProcessStartOutcomeStatusStarted) {
			return false
		}
	}
	switch p.status {
	case ProcessStartOutcomeStatusStarted:
		return !p.startedAt.IsZero() && p.startedAt.Location() == time.UTC && !p.failure.Valid()
	case ProcessStartOutcomeStatusAborted:
		return p.startedAt.IsZero() && p.failure.Valid() &&
			(!p.hasTreeSnapshot || p.hasPreviousTree)
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

// ProcessStartOutcomeAcknowledgerFunc adapts a plain function to the
// acknowledger interface. The function still owes the interface's guarantees —
// bounded, concurrency-safe, idempotent, and no re-entry into the Engine.
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

func startedProcessOutcome(admission ProcessAdmission, startedAt time.Time) ProcessStartOutcome {
	return ProcessStartOutcome{
		admission: admission, status: ProcessStartOutcomeStatusStarted,
		startedAt: startedAt.Round(0).UTC(),
	}
}

func startedProcessTreeOutcome(
	admission ProcessAdmission,
	startedAt time.Time,
	previousTreeDigest Digest,
	hasPreviousTree bool,
	treeSnapshot TreeSnapshot,
) ProcessStartOutcome {
	outcome := startedProcessOutcome(admission, startedAt)
	outcome.previousTreeDigest = previousTreeDigest
	outcome.hasPreviousTree = hasPreviousTree
	outcome.treeSnapshot = treeSnapshot
	outcome.hasTreeSnapshot = true
	return outcome
}

func abortedProcessOutcome(admission ProcessAdmission, failure Failure) ProcessStartOutcome {
	return ProcessStartOutcome{
		admission: admission,
		status:    ProcessStartOutcomeStatusAborted,
		failure:   failure,
	}
}

func abortedProcessTreeOutcome(
	admission ProcessAdmission,
	failure Failure,
	previousTreeDigest Digest,
	treeSnapshot TreeSnapshot,
) ProcessStartOutcome {
	outcome := abortedProcessOutcome(admission, failure)
	outcome.previousTreeDigest = previousTreeDigest
	outcome.hasPreviousTree = true
	outcome.treeSnapshot = treeSnapshot
	outcome.hasTreeSnapshot = true
	return outcome
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
		return errors.New("invalid Process start outcome")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("process-start outcome acknowledger panicked: %v", recovered)
		}
	}()
	return acknowledger.AcknowledgeProcessStartOutcome(
		context.WithoutCancel(requireContext(ctx)), outcome,
	)
}
