package runs

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ChildOpeningRequest asks the Coordinator to durably admit one child Run
// before its executor process publishes ProcessCreated or starts executing.
//
// It is an internal control signal, not a reducible EngineEvent and never a
// journal or wire value. The constructor returns the request and the matching
// confirmation as separate capabilities: the executor may wait, while only the
// runs package can claim and complete the transaction.
type ChildOpeningRequest struct {
	executorPayloadBase
	StartedAt time.Time
	exchange  *childOpeningExchange
}

// ChildOpeningConfirmation is the executor's read-only side of one child
// opening transaction.
type ChildOpeningConfirmation struct {
	exchange *childOpeningExchange
}

// NewChildOpeningRequest creates one single-use child opening handshake.
func NewChildOpeningRequest(startedAt time.Time) (ChildOpeningRequest, ChildOpeningConfirmation) {
	exchange := &childOpeningExchange{result: make(chan error, 1)}
	return ChildOpeningRequest{StartedAt: startedAt, exchange: exchange},
		ChildOpeningConfirmation{exchange: exchange}
}

func (request ChildOpeningRequest) validate() error {
	if request.exchange == nil {
		return errors.New("runs: child opening request has no confirmation")
	}
	if request.StartedAt.IsZero() {
		return errors.New("runs: child opening request has no process start time")
	}
	return nil
}

// claim transfers cancellation ownership from the waiting executor to the
// Coordinator. false means the executor's context canceled before transaction
// processing began, so no durable opening may be attempted.
func (request ChildOpeningRequest) claim() bool {
	if request.exchange == nil {
		return false
	}
	return request.exchange.claim()
}

func (request ChildOpeningRequest) complete(err error) error {
	if request.exchange == nil {
		return errors.New("runs: complete child opening without a confirmation")
	}
	return request.exchange.complete(err)
}

// Await waits for the authoritative transaction result. Cancellation wins
// while the request is still queued. Once the Coordinator claims it, Await
// waits for the result even if ctx is canceled: abandoning an in-flight commit
// would let the executor remove a child whose durable Run may already exist.
func (confirmation ChildOpeningConfirmation) Await(ctx context.Context) error {
	if confirmation.exchange == nil {
		return errors.New("runs: await child opening without a confirmation")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return confirmation.exchange.await(ctx)
}

type childOpeningState uint8

const (
	childOpeningPending childOpeningState = iota
	childOpeningClaimed
	childOpeningCompleted
	childOpeningCanceled
)

type childOpeningExchange struct {
	mu     sync.Mutex
	state  childOpeningState
	result chan error
}

func (exchange *childOpeningExchange) claim() bool {
	exchange.mu.Lock()
	defer exchange.mu.Unlock()
	if exchange.state != childOpeningPending {
		return false
	}
	exchange.state = childOpeningClaimed
	return true
}

func (exchange *childOpeningExchange) complete(err error) error {
	exchange.mu.Lock()
	if exchange.state != childOpeningClaimed {
		state := exchange.state
		exchange.mu.Unlock()
		return fmt.Errorf(
			"runs: complete child opening confirmation in %s state; want claimed",
			state,
		)
	}
	exchange.state = childOpeningCompleted
	exchange.mu.Unlock()
	exchange.result <- err
	return nil
}

func (exchange *childOpeningExchange) await(ctx context.Context) error {
	select {
	case err := <-exchange.result:
		return err
	case <-ctx.Done():
	}

	exchange.mu.Lock()
	switch exchange.state {
	case childOpeningPending:
		exchange.state = childOpeningCanceled
		exchange.mu.Unlock()
		return ctx.Err()
	case childOpeningClaimed:
		exchange.mu.Unlock()
		return <-exchange.result
	case childOpeningCompleted:
		exchange.mu.Unlock()
		return <-exchange.result
	case childOpeningCanceled:
		exchange.mu.Unlock()
		return ctx.Err()
	default:
		exchange.mu.Unlock()
		return errors.New("runs: child opening confirmation has an invalid state")
	}
}

func (state childOpeningState) String() string {
	switch state {
	case childOpeningPending:
		return "pending"
	case childOpeningClaimed:
		return "claimed"
	case childOpeningCompleted:
		return "completed"
	case childOpeningCanceled:
		return "canceled"
	default:
		return "invalid"
	}
}
