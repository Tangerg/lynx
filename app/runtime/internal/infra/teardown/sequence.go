package teardown

import (
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/completion"
)

// Sequence owns terminal resource steps in creation order. Shutdown starts one
// reverse-order generation that outlives any individual caller deadline; this
// lets a failed constructor return promptly without abandoning the dependency
// graph once an in-flight closer eventually finishes.
type Sequence struct {
	mu      sync.Mutex
	steps   []*Step
	attempt *sequenceAttempt
}

type sequenceAttempt struct {
	done chan struct{}
	err  error
}

func NewSequence(steps []*Step) *Sequence {
	return &Sequence{steps: slices.Clone(steps)}
}

// Shutdown starts or joins the one terminal teardown generation. settled=false
// means only that this caller stopped waiting; the Sequence still owns and runs
// the graph. settled=true returns the immutable aggregate diagnostic.
func (sequence *Sequence) Shutdown(ctx context.Context) (settled bool, err error) {
	if sequence == nil {
		return true, nil
	}
	if ctx == nil {
		return false, errors.New("teardown: sequence context is required")
	}
	sequence.mu.Lock()
	attempt := sequence.attempt
	if attempt == nil {
		if err := ctx.Err(); err != nil {
			sequence.mu.Unlock()
			return false, err
		}
		attempt = &sequenceAttempt{done: make(chan struct{})}
		sequence.attempt = attempt
		go sequence.run(context.WithoutCancel(ctx), attempt)
	}
	sequence.mu.Unlock()

	if err := completion.Wait(ctx, attempt.done); err != nil {
		return false, err
	}
	return true, attempt.err
}

func (sequence *Sequence) run(ctx context.Context, attempt *sequenceAttempt) {
	var diagnostics []error
	for _, step := range slices.Backward(sequence.steps) {
		if step == nil {
			continue
		}
		err := step.run(ctx)
		diagnostics = append(diagnostics, err)
	}

	sequence.mu.Lock()
	sequence.steps = nil
	attempt.err = errors.Join(diagnostics...)
	close(attempt.done)
	sequence.mu.Unlock()
}
