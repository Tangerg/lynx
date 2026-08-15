package agentexec

import (
	"context"
	"errors"
	"fmt"
	"sync"

	agent "github.com/Tangerg/lynx/agent"
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

func newDispatchAttempt(effectID agent.EffectID) *dispatchAttempt {
	failure, fail := context.WithCancelCause(context.Background())
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

func (attempt *dispatchAttempt) beginExternalCall() error {
	attempt.mu.Lock()
	defer attempt.mu.Unlock()
	if attempt.projectionErr != nil {
		return attempt.projectionErr
	}
	attempt.externalCalls++
	return nil
}

func (attempt *dispatchAttempt) recordProjectionFailure(err error) {
	if err == nil {
		return
	}
	attempt.mu.Lock()
	if attempt.projectionErr == nil {
		attempt.projectionErr = err
	} else {
		attempt.projectionErr = errors.Join(attempt.projectionErr, err)
	}
	fail := attempt.fail
	attempt.mu.Unlock()
	if fail != nil {
		fail(err)
	}
}

// projectionContext detaches a post-external projection from ordinary Effect
// cancellation while still retiring it when another member of the same Effect
// proves the aggregate outcome indeterminate.
func (attempt *dispatchAttempt) projectionContext(parent context.Context) (context.Context, context.CancelFunc) {
	bound, cancel := context.WithCancelCause(parent)
	stop := context.AfterFunc(attempt.failure, func() {
		cancel(context.Cause(attempt.failure))
	})
	return bound, func() {
		stop()
		cancel(nil)
	}
}

func (attempt *dispatchAttempt) close() {
	if attempt.fail != nil {
		attempt.fail(context.Canceled)
	}
}

func (attempt *dispatchAttempt) indeterminateFailure() error {
	attempt.mu.Lock()
	defer attempt.mu.Unlock()
	if attempt.externalCalls == 0 || attempt.projectionErr == nil {
		return nil
	}
	return attempt.projectionErr
}

func (attempt *dispatchAttempt) crossedExternalBoundary() bool {
	attempt.mu.Lock()
	defer attempt.mu.Unlock()
	return attempt.externalCalls > 0
}
