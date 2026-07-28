package runtime

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/agent/core"
)

// RunCompletion is the immutable outcome of one Process run segment.
//
// Failure is the process-domain failure recorded by the state machine. Err is
// an operational error from driving or finalizing this segment. Both may be
// present: for example, an action can fail and its final automatic snapshot can
// fail independently. The result history is captured at the same joined
// boundary and queried through [CompletionResult].
type RunCompletion struct {
	Status  core.ProcessStatus
	Failure error
	Err     error

	results []any
}

// Error joins the independent process and segment failures without assigning
// an arbitrary precedence to either.
func (c RunCompletion) Error() error {
	if c.Failure == nil {
		return c.Err
	}
	if c.Err == nil {
		return c.Failure
	}
	if errors.Is(c.Err, c.Failure) {
		return c.Err
	}
	if errors.Is(c.Failure, c.Err) {
		return c.Failure
	}
	return errors.Join(c.Failure, c.Err)
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

// Segment owns one asynchronous entry into a Process run loop. Callers join its
// stable completion snapshot instead of reconstructing an outcome from mutable
// Process state.
type Segment struct {
	process    *Process
	done       chan struct{}
	completion RunCompletion
}

func newSegment(process *Process) *Segment {
	return &Segment{process: process, done: make(chan struct{})}
}

// Process returns the process driven by this segment.
func (s *Segment) Process() *Process {
	if s == nil {
		return nil
	}
	return s.process
}

// Await joins the segment and returns its immutable completion. Canceling ctx
// abandons only this observation; it does not cancel the Process. It is safe for
// multiple observers and repeated calls.
func (s *Segment) Await(ctx context.Context) (RunCompletion, error) {
	if s == nil || s.process == nil || s.done == nil {
		return RunCompletion{}, errors.New("runtime.Segment.Await: invalid segment")
	}
	ctx = normalizeContext(ctx)
	select {
	case <-s.done:
		return s.completion, nil
	default:
	}
	select {
	case <-s.done:
		return s.completion, nil
	case <-ctx.Done():
		select {
		case <-s.done:
			return s.completion, nil
		default:
			return RunCompletion{}, ctx.Err()
		}
	}
}

func (p *Process) captureCompletion(err error) RunCompletion {
	return RunCompletion{
		Status:  p.Status(),
		Failure: p.Failure(),
		Err:     err,
		results: p.Blackboard().Objects(),
	}
}

func (s *Segment) complete(completion RunCompletion) {
	s.completion = completion
	close(s.done)
}
