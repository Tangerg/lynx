package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

func TestRunCompletionErrorPreservesIndependentFailures(t *testing.T) {
	t.Parallel()

	processFailure := errors.New("process failed")
	finalizeFailure := errors.New("snapshot failed")
	completion := RunCompletion{
		Failure:      processFailure,
		RuntimeError: finalizeFailure,
	}

	if err := completion.Error(); !errors.Is(err, processFailure) || !errors.Is(err, finalizeFailure) {
		t.Fatalf("Error() = %v, want both independent failures", err)
	}
}

func TestRunCompletionErrorDoesNotDuplicateNestedFailure(t *testing.T) {
	t.Parallel()

	processFailure := errors.New("process failed")
	runFailure := errors.Join(processFailure, errors.New("snapshot failed"))
	completion := RunCompletion{Failure: processFailure, RuntimeError: runFailure}

	if err := completion.Error(); err != runFailure {
		t.Fatalf("Error() = %v, want the encompassing run failure", err)
	}
}

func TestCompletionResultFindsLatestValueOfRequestedType(t *testing.T) {
	t.Parallel()

	type output struct{ Value int }
	completion := RunCompletion{
		Status:  core.StatusCompleted,
		results: []any{output{Value: 1}, "unrelated tail", output{Value: 2}, 42},
	}

	got, ok := CompletionResult[output](completion)
	if !ok || got.Value != 2 {
		t.Fatalf("CompletionResult[output]() = (%+v, %v), want ({2}, true)", got, ok)
	}
	if _, ok := CompletionResult[bool](completion); ok {
		t.Fatal("CompletionResult[bool]() found an absent result")
	}
}

func TestRunHandleAwaitCancellationDoesNotConsumeCompletion(t *testing.T) {
	t.Parallel()

	handle := &RunHandle{process: &Process{}, done: make(chan struct{})}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := handle.Await(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Await() = %v, want context.Canceled", err)
	}

	handle.completion = RunCompletion{Status: core.StatusCompleted}
	close(handle.done)
	completion, err := handle.Await(t.Context())
	if err != nil || completion.Status != core.StatusCompleted {
		t.Fatalf("second Await() = (%+v, %v), want completed", completion, err)
	}
}

func TestRunHandleAwaitPrefersObservableCompletion(t *testing.T) {
	t.Parallel()

	done := make(chan struct{})
	close(done)
	handle := &RunHandle{
		process:    &Process{},
		done:       done,
		completion: RunCompletion{Status: core.StatusWaiting},
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	completion, err := handle.Await(ctx)
	if err != nil || completion.Status != core.StatusWaiting {
		t.Fatalf("Await() = (%+v, %v), want waiting completion", completion, err)
	}
}
