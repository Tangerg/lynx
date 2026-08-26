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
func (c ChildRunStartReservation) Validate() error {
	if err := c.validateIdentity(); err != nil {
		return err
	}
	if err := c.Member.Validate(); err != nil {
		return fmt.Errorf("runs: child Run start reservation member: %w", err)
	}
	if !c.Member.Child() || c.Member.SpawnCallID == "" {
		return errors.New("runs: child Run start reservation requires a causal child member")
	}
	if err := c.Binding.Validate(); err != nil {
		return fmt.Errorf("runs: child Run start reservation binding: %w", err)
	}
	if c.Binding.MemberID != c.Member.MemberID {
		return errors.New("runs: child Run start reservation member differs from its binding")
	}
	if c.Binding.ParentRunID == c.Binding.RunID ||
		c.Binding.RunID == c.RootRunID {
		return errors.New("runs: child Run start reservation has contradictory Run identity")
	}
	if c.StartedAt.IsZero() {
		return errors.New("runs: child Run start reservation has no executor start time")
	}
	return nil
}

func (c ChildRunStartReservation) validateIdentity() error {
	if err := validateRequiredIdentity("session ID", c.SessionID); err != nil {
		return fmt.Errorf("runs: child Run start reservation: %w", err)
	}
	if err := validateRequiredIdentity("executor ID", c.ExecutorID); err != nil {
		return fmt.Errorf("runs: child Run start reservation: %w", err)
	}
	if err := validateRequiredIdentity("segment ID", c.SegmentID); err != nil {
		return fmt.Errorf("runs: child Run start reservation: %w", err)
	}
	if err := validateRequiredIdentity("spawning Item ID", c.SpawnedByItemID); err != nil {
		return fmt.Errorf("runs: child Run start reservation: %w", err)
	}
	if err := validateRequiredIdentity("root Run ID", c.RootRunID); err != nil {
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

func (c ChildRunReservationRequest) validate() error {
	if c.exchange == nil {
		return errors.New("runs: child Run reservation request has no receipt")
	}
	if c.StartedAt.IsZero() {
		return errors.New("runs: child Run reservation request has no executor start time")
	}
	return nil
}

func (c ChildRunReservationRequest) claim() bool { return c.exchange.claim() }

func (c ChildRunReservationRequest) complete(binding ChildRunBinding, err error) error {
	if c.exchange == nil {
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
	return c.exchange.complete(binding, err)
}

// Await returns the exact product binding after the Application durably stores
// the invisible reservation.
func (c ChildRunReservationReceipt) Await(
	ctx context.Context,
) (ChildRunBinding, error) {
	return c.exchange.await(ctx)
}

// ChildRunStartOutcome identifies the conclusive executor initialization
// result applied to one durable reservation. The zero value is invalid.
type ChildRunStartOutcome string

const (
	childRunStartOutcomeInvalid ChildRunStartOutcome = ""
	ChildRunStarted             ChildRunStartOutcome = "started"
	ChildRunStartAborted        ChildRunStartOutcome = "aborted"
)

// Valid reports whether c is one conclusive child initialization fact.
func (c ChildRunStartOutcome) Valid() bool {
	return c == ChildRunStarted || c == ChildRunStartAborted
}

// String returns the durable child-start conclusion name.
func (c ChildRunStartOutcome) String() string {
	if !c.Valid() {
		return "invalid"
	}
	return string(c)
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

func (c ChildRunStartOutcomeRequest) validate() error {
	if c.exchange == nil {
		return errors.New("runs: child Run start outcome request has no receipt")
	}
	if err := c.Binding.Validate(); err != nil {
		return err
	}
	if !c.Outcome.Valid() {
		return errors.New("runs: child Run start outcome is invalid")
	}
	return nil
}

func (c ChildRunStartOutcomeRequest) claim() bool { return c.exchange.claim() }

func (c ChildRunStartOutcomeRequest) complete(err error) error {
	if c.exchange == nil {
		return errors.New("runs: complete child Run start outcome without a receipt")
	}
	return c.exchange.complete(struct{}{}, err)
}

// Await returns after the Application has durably published or discarded the
// reserved child start.
func (c ChildRunStartOutcomeReceipt) Await(ctx context.Context) error {
	_, err := c.exchange.await(ctx)
	return err
}
