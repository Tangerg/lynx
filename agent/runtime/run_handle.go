package runtime

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/agent/core"
)

// RunCompletion is the immutable outcome of one Process run-loop entry.
//
// Failure is the process-domain failure recorded by the state machine.
// RuntimeError is an operational error from driving or finalizing the run. Both may be
// present: for example, an action can fail and its final automatic snapshot can
// fail independently. The result history is captured at the same joined
// boundary and queried through [CompletionResult].
type RunCompletion struct {
	Status       core.ProcessStatus
	Failure      error
	RuntimeError error

	results []any
}

// Error joins the independent process and runtime failures without assigning
// an arbitrary precedence to either.
func (c RunCompletion) Error() error {
	if c.Failure == nil {
		return c.RuntimeError
	}
	if c.RuntimeError == nil {
		return c.Failure
	}
	if errors.Is(c.RuntimeError, c.Failure) {
		return c.RuntimeError
	}
	if errors.Is(c.Failure, c.RuntimeError) {
		return c.Failure
	}
	return errors.Join(c.Failure, c.RuntimeError)
}

// CompletionResult returns the most-recent result assignable to T from the
// completion snapshot.
func CompletionResult[T any](completion RunCompletion) (T, bool) {
	for index := len(completion.results) - 1; index >= 0; index-- {
		if result, ok := completion.results[index].(T); ok {
			return result, true
		}
	}
	var zero T
	return zero, false
}

// RunHandle owns one asynchronous entry into a Process run loop. Callers join its
// stable completion snapshot instead of reconstructing an outcome from mutable
// Process state.
type RunHandle struct {
	process    *Process
	done       chan struct{}
	completion RunCompletion
}

func newRunHandle(process *Process) *RunHandle {
	return &RunHandle{process: process, done: make(chan struct{})}
}

// Process returns the process driven by this run.
func (h *RunHandle) Process() *Process {
	if h == nil {
		return nil
	}
	return h.process
}

// Await joins the run and returns its immutable completion. Canceling ctx
// abandons only this observation; it does not cancel the Process. It is safe for
// multiple observers and repeated calls.
func (h *RunHandle) Await(ctx context.Context) (RunCompletion, error) {
	if h == nil || h.process == nil || h.done == nil {
		return RunCompletion{}, errors.New("runtime.RunHandle.Await: invalid run handle")
	}
	ctx = normalizeContext(ctx)
	select {
	case <-h.done:
		return h.completion, nil
	default:
	}
	select {
	case <-h.done:
		return h.completion, nil
	case <-ctx.Done():
		select {
		case <-h.done:
			return h.completion, nil
		default:
			return RunCompletion{}, ctx.Err()
		}
	}
}

func (p *Process) captureCompletion(err error) RunCompletion {
	return RunCompletion{
		Status:       p.Status(),
		Failure:      p.Failure(),
		RuntimeError: err,
		results:      p.Blackboard().Objects(),
	}
}

func (h *RunHandle) complete(completion RunCompletion) {
	h.completion = completion
	close(h.done)
}
