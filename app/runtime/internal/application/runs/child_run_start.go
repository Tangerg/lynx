package runs

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ChildRunStartReservation is the durable, non-public identity allocated for
// one admitted executor child before that child has conclusively initialized.
// It is not a Run and must never be returned by Run projections. The exact
// value is consumed when the child either enters Running or aborts.
type ChildRunStartReservation struct {
	SessionID       string
	ExecutorID      string
	Member          ExecutorMember
	Binding         ChildRunBinding
	SegmentID       string
	SpawnedByItemID string
	RootRunID       string
	StartedAt       time.Time
}

// Validate proves that the reservation binds one child executor identity to
// one future product child without copying executor topology into the Run.
func (reservation ChildRunStartReservation) Validate() error {
	if err := reservation.validateIdentity(); err != nil {
		return err
	}
	if err := reservation.Member.Validate(); err != nil {
		return fmt.Errorf("runs: child Run start reservation member: %w", err)
	}
	if !reservation.Member.Child() || reservation.Member.SpawnCallID == "" {
		return errors.New("runs: child Run start reservation requires a causal child member")
	}
	if err := reservation.Binding.Validate(); err != nil {
		return fmt.Errorf("runs: child Run start reservation binding: %w", err)
	}
	if reservation.Binding.MemberID != reservation.Member.MemberID {
		return errors.New("runs: child Run start reservation member differs from its binding")
	}
	if reservation.Binding.ParentRunID == reservation.Binding.RunID ||
		reservation.Binding.RunID == reservation.RootRunID {
		return errors.New("runs: child Run start reservation has contradictory Run identity")
	}
	if reservation.StartedAt.IsZero() {
		return errors.New("runs: child Run start reservation has no executor start time")
	}
	return nil
}

func (reservation ChildRunStartReservation) validateIdentity() error {
	if err := validateRequiredIdentity("session ID", reservation.SessionID); err != nil {
		return fmt.Errorf("runs: child Run start reservation: %w", err)
	}
	if err := validateRequiredIdentity("executor ID", reservation.ExecutorID); err != nil {
		return fmt.Errorf("runs: child Run start reservation: %w", err)
	}
	if err := validateRequiredIdentity("segment ID", reservation.SegmentID); err != nil {
		return fmt.Errorf("runs: child Run start reservation: %w", err)
	}
	if err := validateRequiredIdentity("spawning Item ID", reservation.SpawnedByItemID); err != nil {
		return fmt.Errorf("runs: child Run start reservation: %w", err)
	}
	if err := validateRequiredIdentity("root Run ID", reservation.RootRunID); err != nil {
		return fmt.Errorf("runs: child Run start reservation: %w", err)
	}
	return nil
}

// ChildRunStartCommitter owns the three durable transitions of an invisible
// child start reservation. CommitStarted atomically concludes the exact
// reservation with the child Run opening; Abort concludes it without publishing
// a Run. Implementations must treat an exact repeated request idempotently and
// reject a contradictory conclusion.
type ChildRunStartCommitter interface {
	ReserveChildRunStart(ctx context.Context, reservation ChildRunStartReservation) error
	CommitStartedChildRun(
		ctx context.Context,
		reservation ChildRunStartReservation,
		opening OpeningCommit,
	) error
	AbortChildRunStart(ctx context.Context, reservation ChildRunStartReservation) error
}

// ChildRunReservationRequest asks the Run pump to durably reserve one child
// Run identity before the corresponding executor child initializes. The event
// envelope carries the opaque child/parent/call causality; StartedAt is the
// exact stable value supplied by the executor admission contract.
type ChildRunReservationRequest struct {
	executorPayloadBase
	StartedAt time.Time
	exchange  *executorRequest[ChildRunBinding]
}

// ChildRunReservationReceipt is the executor's read-only side of one
// child reservation request.
type ChildRunReservationReceipt struct {
	exchange *executorRequest[ChildRunBinding]
}

// NewChildRunReservationRequest creates one single-use reservation handshake.
func NewChildRunReservationRequest(
	startedAt time.Time,
) (ChildRunReservationRequest, ChildRunReservationReceipt) {
	exchange := newExecutorRequest[ChildRunBinding]()
	return ChildRunReservationRequest{StartedAt: startedAt, exchange: exchange},
		ChildRunReservationReceipt{exchange: exchange}
}

func (request ChildRunReservationRequest) validate() error {
	if request.exchange == nil {
		return errors.New("runs: child Run reservation request has no receipt")
	}
	if request.StartedAt.IsZero() {
		return errors.New("runs: child Run reservation request has no executor start time")
	}
	return nil
}

func (request ChildRunReservationRequest) claim() bool { return request.exchange.claim() }

func (request ChildRunReservationRequest) complete(binding ChildRunBinding, err error) error {
	if request.exchange == nil {
		return errors.New("runs: complete child Run reservation without a receipt")
	}
	if err == nil {
		if validationErr := binding.Validate(); validationErr != nil {
			err = validationErr
		}
	} else if binding != (ChildRunBinding{}) {
		err = errors.Join(err, errors.New("runs: failed child Run reservation returned a binding"))
		binding = ChildRunBinding{}
	}
	return request.exchange.complete(binding, err)
}

// Await returns the exact product binding after the Application durably stores
// the invisible reservation.
func (receipt ChildRunReservationReceipt) Await(
	ctx context.Context,
) (ChildRunBinding, error) {
	return receipt.exchange.await(ctx)
}

// ChildRunStartOutcome identifies the conclusive executor initialization
// result applied to one durable reservation. The zero value is invalid.
type ChildRunStartOutcome string

const (
	childRunStartOutcomeInvalid ChildRunStartOutcome = ""
	ChildRunStarted             ChildRunStartOutcome = "started"
	ChildRunStartAborted        ChildRunStartOutcome = "aborted"
)

// Valid reports whether outcome is one conclusive child initialization fact.
func (outcome ChildRunStartOutcome) Valid() bool {
	return outcome == ChildRunStarted || outcome == ChildRunStartAborted
}

// String returns the durable child-start conclusion name.
func (outcome ChildRunStartOutcome) String() string {
	if !outcome.Valid() {
		return "invalid"
	}
	return string(outcome)
}

// ChildRunStartOutcomeRequest asks the Run pump to consume the exact
// reservation after executor initialization concludes. Started publishes the
// child Run atomically; aborted leaves no public Run.
type ChildRunStartOutcomeRequest struct {
	executorPayloadBase
	Binding  ChildRunBinding
	Outcome  ChildRunStartOutcome
	exchange *executorRequest[struct{}]
}

// ChildRunStartOutcomeReceipt is the executor's read-only side of the
// conclusive child start transaction.
type ChildRunStartOutcomeReceipt struct {
	exchange *executorRequest[struct{}]
}

// NewChildRunStartOutcomeRequest creates one single-use outcome handshake.
func NewChildRunStartOutcomeRequest(
	binding ChildRunBinding,
	outcome ChildRunStartOutcome,
) (ChildRunStartOutcomeRequest, ChildRunStartOutcomeReceipt) {
	exchange := newExecutorRequest[struct{}]()
	return ChildRunStartOutcomeRequest{Binding: binding, Outcome: outcome, exchange: exchange},
		ChildRunStartOutcomeReceipt{exchange: exchange}
}

func (request ChildRunStartOutcomeRequest) validate() error {
	if request.exchange == nil {
		return errors.New("runs: child Run start outcome request has no receipt")
	}
	if err := request.Binding.Validate(); err != nil {
		return err
	}
	if !request.Outcome.Valid() {
		return errors.New("runs: child Run start outcome is invalid")
	}
	return nil
}

func (request ChildRunStartOutcomeRequest) claim() bool { return request.exchange.claim() }

func (request ChildRunStartOutcomeRequest) complete(err error) error {
	if request.exchange == nil {
		return errors.New("runs: complete child Run start outcome without a receipt")
	}
	return request.exchange.complete(struct{}{}, err)
}

// Await returns after the Application has durably published or discarded the
// reserved child start.
func (receipt ChildRunStartOutcomeReceipt) Await(ctx context.Context) error {
	_, err := receipt.exchange.await(ctx)
	return err
}
