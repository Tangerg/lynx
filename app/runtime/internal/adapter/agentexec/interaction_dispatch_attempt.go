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

	mu            sync.Mutex
	externalCalls uint32
	projectionErr error
}

func newDispatchAttempt(effectID agent.EffectID) *dispatchAttempt {
	return &dispatchAttempt{effectID: effectID}
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
	defer attempt.mu.Unlock()
	if attempt.projectionErr == nil {
		attempt.projectionErr = err
	} else {
		attempt.projectionErr = errors.Join(attempt.projectionErr, err)
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
