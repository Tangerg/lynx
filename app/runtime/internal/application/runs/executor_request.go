package runs

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// executorRequest is the package-private ownership protocol for an executor
// control request whose durable decision belongs to the Run pump. Cancellation
// may withdraw a queued request; after claim, the producer must observe the
// consumer's conclusive result so it cannot act on an ambiguous write-set.
type executorRequest[T any] struct {
	mu     sync.Mutex
	state  executorRequestState
	result chan executorRequestResult[T]
}

type executorRequestState uint8

const (
	executorRequestPending executorRequestState = iota
	executorRequestClaimed
	executorRequestCompleted
	executorRequestCanceled
)

type executorRequestResult[T any] struct {
	value T
	err   error
}

func newExecutorRequest[T any]() *executorRequest[T] {
	return &executorRequest[T]{result: make(chan executorRequestResult[T], 1)}
}

func (request *executorRequest[T]) claim() bool {
	if request == nil {
		return false
	}
	request.mu.Lock()
	defer request.mu.Unlock()
	if request.state != executorRequestPending {
		return false
	}
	request.state = executorRequestClaimed
	return true
}

func (request *executorRequest[T]) complete(value T, err error) error {
	if request == nil {
		return errors.New("runs: complete nil executor request")
	}
	request.mu.Lock()
	if request.state != executorRequestClaimed {
		state := request.state
		request.mu.Unlock()
		return fmt.Errorf("runs: complete executor request in %s state; want claimed", state)
	}
	request.state = executorRequestCompleted
	request.mu.Unlock()
	request.result <- executorRequestResult[T]{value: value, err: err}
	return nil
}

func (request *executorRequest[T]) await(ctx context.Context) (T, error) {
	var zero T
	if request == nil || request.result == nil {
		return zero, errors.New("runs: await malformed executor request")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case result := <-request.result:
		return result.value, result.err
	case <-ctx.Done():
	}

	request.mu.Lock()
	switch request.state {
	case executorRequestPending:
		request.state = executorRequestCanceled
		request.mu.Unlock()
		return zero, ctx.Err()
	case executorRequestClaimed, executorRequestCompleted:
		request.mu.Unlock()
		result := <-request.result
		return result.value, result.err
	case executorRequestCanceled:
		request.mu.Unlock()
		return zero, ctx.Err()
	default:
		request.mu.Unlock()
		return zero, errors.New("runs: executor request has an invalid state")
	}
}

func (state executorRequestState) String() string {
	switch state {
	case executorRequestPending:
		return "pending"
	case executorRequestClaimed:
		return "claimed"
	case executorRequestCompleted:
		return "completed"
	case executorRequestCanceled:
		return "canceled"
	default:
		return "invalid"
	}
}
