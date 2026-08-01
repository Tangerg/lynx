package runs

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

// ChildRunBinding is the application identity assigned to one opaque executor
// child after its opening transaction commits. Product lifecycle observers use
// Run identity; executor process identity remains an adapter routing detail.
type ChildRunBinding struct {
	ProcessID   string
	RunID       string
	ParentRunID string
}

// Validate rejects incomplete or ambiguous child identity before it reaches a
// lifecycle observer.
func (binding ChildRunBinding) Validate() error {
	switch {
	case binding.ProcessID == "":
		return errors.New("runs: child Run binding has no executor process id")
	case strings.TrimSpace(binding.ProcessID) != binding.ProcessID:
		return fmt.Errorf("runs: child Run binding process id %q has surrounding whitespace", binding.ProcessID)
	case binding.RunID == "":
		return errors.New("runs: child Run binding has no Run id")
	case strings.TrimSpace(binding.RunID) != binding.RunID:
		return fmt.Errorf("runs: child Run binding Run id %q has surrounding whitespace", binding.RunID)
	case binding.ParentRunID == "":
		return errors.New("runs: child Run binding has no parent Run id")
	case strings.TrimSpace(binding.ParentRunID) != binding.ParentRunID:
		return fmt.Errorf("runs: child Run binding parent Run id %q has surrounding whitespace", binding.ParentRunID)
	case binding.RunID == binding.ParentRunID:
		return errors.New("runs: child Run binding refers to itself as parent")
	default:
		return nil
	}
}

// ValidateChildRunBindings proves that every restored child belongs to one
// connected App Run tree rooted at rootRunID. Executor topology is deliberately
// absent from this validation.
func ValidateChildRunBindings(rootRunID string, bindings []ChildRunBinding) error {
	if rootRunID == "" || strings.TrimSpace(rootRunID) != rootRunID {
		return errors.New("runs: restored child Run bindings require a root Run id")
	}
	byRun := make(map[string]ChildRunBinding, len(bindings))
	byProcess := make(map[string]string, len(bindings))
	for _, binding := range bindings {
		if err := binding.Validate(); err != nil {
			return err
		}
		if binding.RunID == rootRunID {
			return fmt.Errorf("runs: root Run %q cannot appear as a child binding", rootRunID)
		}
		if _, exists := byRun[binding.RunID]; exists {
			return fmt.Errorf("runs: duplicate child Run binding %q", binding.RunID)
		}
		if runID, exists := byProcess[binding.ProcessID]; exists {
			return fmt.Errorf(
				"runs: executor process %q is bound to child Runs %q and %q",
				binding.ProcessID,
				runID,
				binding.RunID,
			)
		}
		byRun[binding.RunID] = binding
		byProcess[binding.ProcessID] = binding.RunID
	}
	for _, binding := range bindings {
		seen := map[string]struct{}{binding.RunID: {}}
		parentRunID := binding.ParentRunID
		for parentRunID != rootRunID {
			parent, exists := byRun[parentRunID]
			if !exists {
				return fmt.Errorf(
					"runs: child Run %q refers to unknown parent Run %q",
					binding.RunID,
					parentRunID,
				)
			}
			if _, cyclic := seen[parentRunID]; cyclic {
				return fmt.Errorf("runs: child Run binding cycle reaches %q", parentRunID)
			}
			seen[parentRunID] = struct{}{}
			parentRunID = parent.ParentRunID
		}
	}
	return nil
}

// NewChildOpeningRequest creates one single-use child opening handshake.
func NewChildOpeningRequest(startedAt time.Time) (ChildOpeningRequest, ChildOpeningConfirmation) {
	exchange := &childOpeningExchange{result: make(chan childOpeningResult, 1)}
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

func (request ChildOpeningRequest) complete(binding ChildRunBinding, err error) error {
	if request.exchange == nil {
		return errors.New("runs: complete child opening without a confirmation")
	}
	var contractErr error
	if err == nil {
		if validationErr := binding.Validate(); validationErr != nil {
			contractErr = validationErr
			err = validationErr
		}
	} else if binding != (ChildRunBinding{}) {
		contractErr = errors.New("runs: failed child opening cannot return a Run binding")
		err = errors.Join(err, contractErr)
		binding = ChildRunBinding{}
	}
	if completionErr := request.exchange.complete(childOpeningResult{binding: binding, err: err}); completionErr != nil {
		return errors.Join(contractErr, completionErr)
	}
	return contractErr
}

// Await waits for the authoritative transaction result. Cancellation wins
// while the request is still queued. Once the Coordinator claims it, Await
// waits for the result even if ctx is canceled: abandoning an in-flight commit
// would let the executor remove a child whose durable Run may already exist.
func (confirmation ChildOpeningConfirmation) Await(ctx context.Context) (ChildRunBinding, error) {
	if confirmation.exchange == nil {
		return ChildRunBinding{}, errors.New("runs: await child opening without a confirmation")
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
	result chan childOpeningResult
}

type childOpeningResult struct {
	binding ChildRunBinding
	err     error
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

func (exchange *childOpeningExchange) complete(result childOpeningResult) error {
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
	exchange.result <- result
	return nil
}

func (exchange *childOpeningExchange) await(ctx context.Context) (ChildRunBinding, error) {
	select {
	case result := <-exchange.result:
		return result.binding, result.err
	case <-ctx.Done():
	}

	exchange.mu.Lock()
	switch exchange.state {
	case childOpeningPending:
		exchange.state = childOpeningCanceled
		exchange.mu.Unlock()
		return ChildRunBinding{}, ctx.Err()
	case childOpeningClaimed:
		exchange.mu.Unlock()
		result := <-exchange.result
		return result.binding, result.err
	case childOpeningCompleted:
		exchange.mu.Unlock()
		result := <-exchange.result
		return result.binding, result.err
	case childOpeningCanceled:
		exchange.mu.Unlock()
		return ChildRunBinding{}, ctx.Err()
	default:
		exchange.mu.Unlock()
		return ChildRunBinding{}, errors.New("runs: child opening confirmation has an invalid state")
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
