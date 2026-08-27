package agentexec

import (
	"context"
	"errors"
	"fmt"
	"sync"

	agent "github.com/Tangerg/scope/agent"
)

type dispatchAttemptContextKey struct{}

// dispatchAttempt is scoped to exactly one EffectRequest. It lets invocation
// decorators distinguish a definite failure before an external call from a
// projection failure after any call in the Effect has crossed that boundary.
type dispatchAttempt struct {
	effectID agent.EffectID
	failure  context.Context
	fail     context.CancelCauseFunc

	mu            sync.Mutex
	externalCalls uint32
	projectionErr error
}

func newDispatchAttempt(parent context.Context, effectID agent.EffectID) *dispatchAttempt {
	failure, fail := context.WithCancelCause(context.WithoutCancel(parent))
	return &dispatchAttempt{effectID: effectID, failure: failure, fail: fail}
}

func withDispatchAttempt(ctx context.Context, attempt *dispatchAttempt) context.Context {
	return context.WithValue(ctx, dispatchAttemptContextKey{}, attempt)
}

func dispatchAttemptFrom(ctx context.Context, effectID agent.EffectID) (*dispatchAttempt, error) {
	if ctx == nil {
		return nil, errors.New("agentexec: missing dispatch context")
	}
	attempt, ok := ctx.Value(dispatchAttemptContextKey{}).(*dispatchAttempt)
	if !ok || attempt == nil || attempt.effectID != effectID {
		return nil, fmt.Errorf("agentexec: invocation attribution does not match dispatch Effect %s", effectID)
	}
	return attempt, nil
}

func (d *dispatchAttempt) beginExternalCall() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.projectionErr != nil {
		return d.projectionErr
	}
	d.externalCalls++
	return nil
}

func (d *dispatchAttempt) recordProjectionFailure(err error) {
	if err == nil {
		return
	}
	d.mu.Lock()
	if d.projectionErr == nil {
		d.projectionErr = err
	} else {
		d.projectionErr = errors.Join(d.projectionErr, err)
	}
	fail := d.fail
	d.mu.Unlock()
	if fail != nil {
		fail(err)
	}
}

// projectionContext detaches a post-external projection from ordinary Effect
// cancellation while still retiring it when another member of the same Effect
// proves the aggregate outcome indeterminate.
func (d *dispatchAttempt) projectionContext(parent context.Context) (context.Context, context.CancelFunc) {
	bound, cancel := context.WithCancelCause(parent)
	stop := context.AfterFunc(d.failure, func() {
		cancel(context.Cause(d.failure))
	})
	return bound, func() {
		stop()
		cancel(nil)
	}
}

func (d *dispatchAttempt) close() {
	if d.fail != nil {
		d.fail(context.Canceled)
	}
}

func (d *dispatchAttempt) indeterminateFailure() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.externalCalls == 0 || d.projectionErr == nil {
		return nil
	}
	return d.projectionErr
}

func (d *dispatchAttempt) crossedExternalBoundary() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.externalCalls > 0
}
