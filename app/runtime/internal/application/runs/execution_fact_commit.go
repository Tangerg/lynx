package runs

import (
	"context"
	"errors"
	"sync"
)

// ExecutionFactCommit asks the Run pump to durably project one authoritative
// executor fact before the external invocation that produced it may settle.
// It travels on the same ordered executor stream as ordinary facts, preserving
// the pump as the sole reducer and persistence writer.
type ExecutionFactCommit struct {
	executorPayloadBase
	Fact  ExecutionFact
	state *executionFactCommitState
}

// ExecutionFactReceipt is the producer side of an [ExecutionFactCommit]. Await
// returns only after the Run pump has either committed every derived write or
// rejected the fact. Abandoning observation completes outstanding receipts with
// context cancellation through the producer's own wait context.
type ExecutionFactReceipt struct {
	state *executionFactCommitState
}

type executionFactCommitState struct {
	once sync.Once
	done chan error
}

// NewExecutionFactCommit creates one authoritative fact request and its
// one-consumer receipt. The fact remains an Application-owned closed value;
// executor implementations receive no persistence handle or transaction capability.
func NewExecutionFactCommit(fact ExecutionFact) (ExecutionFactCommit, ExecutionFactReceipt, error) {
	if fact == nil {
		return ExecutionFactCommit{}, ExecutionFactReceipt{}, errors.New("runs: execution fact commit requires a fact")
	}
	state := &executionFactCommitState{done: make(chan error, 1)}
	return ExecutionFactCommit{Fact: fact, state: state}, ExecutionFactReceipt{state: state}, nil
}

func (e ExecutionFactCommit) validate() error {
	if e.Fact == nil || e.state == nil || e.state.done == nil {
		return errors.New("runs: malformed execution fact commit")
	}
	return nil
}

// Complete resolves the producer receipt after the consumer has committed or
// rejected Fact. The Run pump is the production consumer; focused executor
// harnesses may use the same handshake with their own transactional fake.
func (e ExecutionFactCommit) Complete(err error) {
	if e.state == nil {
		return
	}
	e.state.once.Do(func() {
		e.state.done <- err
		close(e.state.done)
	})
}

// Await waits for the authoritative projection result or for ctx to stop
// waiting. Context cancellation never turns an uncommitted fact into success.
func (e ExecutionFactReceipt) Await(ctx context.Context) error {
	if e.state == nil || e.state.done == nil {
		return errors.New("runs: malformed execution fact receipt")
	}
	select {
	case err := <-e.state.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
