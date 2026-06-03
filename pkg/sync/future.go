package sync

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Sentinel errors returned by Future operations.
var (
	// ErrFutureCancelled is returned by Get-style methods when the
	// future was canceled.
	ErrFutureCancelled = errors.New("future canceled")
	// ErrFutureTimedOut is returned by GetWithTimeout when the deadline
	// elapsed before completion.
	ErrFutureTimedOut = errors.New("future timed out")
)

// FutureState identifies the lifecycle stage of a Future. The
// transitions are: Created → Running → (Succeeded | Failed | Canceled).
type FutureState int32

const (
	FutureStateCreated   FutureState = iota // not yet started
	FutureStateRunning                      // currently executing
	FutureStateSucceeded                    // completed without error
	FutureStateFailed                       // completed with error
	FutureStateCancelled                    // canceled before completion
)

// IsCreated reports whether the future is in the Created state.
func (s FutureState) IsCreated() bool { return s == FutureStateCreated }

// IsRunning reports whether the future is currently executing.
func (s FutureState) IsRunning() bool { return s == FutureStateRunning }

// IsSucceeded reports whether the future completed without error.
func (s FutureState) IsSucceeded() bool { return s == FutureStateSucceeded }

// IsFailed reports whether the future completed with an error.
func (s FutureState) IsFailed() bool { return s == FutureStateFailed }

// IsCancelled reports whether the future was canceled.
func (s FutureState) IsCancelled() bool { return s == FutureStateCancelled }

// int32 returns the underlying value for atomic storage.
func (s FutureState) int32() int32 { return int32(s) }

// Future represents a typed asynchronous computation producing a value
// of type V.
type Future[V any] interface {
	// Cancel attempts to cancel execution. If mayInterruptIfRunning is
	// true and the task is running, the interrupt channel is closed.
	// Returns true if this call performed the cancellation.
	Cancel(mayInterruptIfRunning bool) bool

	// IsCancelled reports whether the future was canceled.
	IsCancelled() bool

	// IsDone reports whether the future has completed for any reason.
	IsDone() bool

	// Get blocks until completion and returns the result.
	Get() (V, error)

	// GetWithTimeout blocks for at most timeout. If the timeout
	// elapses, the task is canceled and [ErrFutureTimedOut] is
	// returned. A non-positive timeout returns immediately.
	GetWithTimeout(timeout time.Duration) (V, error)

	// GetWithContext blocks until ctx is done. On cancellation the
	// task is canceled and the context error is joined with any
	// existing task error.
	GetWithContext(ctx context.Context) (V, error)

	// TryGet returns the result without blocking. The boolean is true
	// only when the future has completed.
	TryGet() (V, error, bool)

	// State returns the current [FutureState].
	State() FutureState
}

// FutureTask is the concrete [Future] implementation. It is safe for
// concurrent use.
type FutureTask[V any] struct {
	task      func(interrupt <-chan struct{}) (V, error)
	state     atomic.Int32
	value     V
	error     error
	done      chan struct{}
	interrupt chan struct{}
	runOnce   sync.Once
	doneOnce  sync.Once
}

// NewFutureTask returns a FutureTask that will run task when [FutureTask.Run]
// is called. The interrupt channel passed to task is closed if the
// future is canceled. Panics if task is nil.
//
// Example:
//
//	ft := sync.NewFutureTask(func(stop <-chan struct{}) (int, error) {
//	    select {
//	    case <-time.After(time.Second):
//	        return 42, nil
//	    case <-stop:
//	        return 0, errors.New("interrupted")
//	    }
//	})
//	go ft.Run()
//	v, err := ft.Get()
func NewFutureTask[V any](task func(interrupt <-chan struct{}) (V, error)) *FutureTask[V] {
	if task == nil {
		panic("sync: task is nil")
	}
	return &FutureTask[V]{
		task:      task,
		done:      make(chan struct{}),
		interrupt: make(chan struct{}),
	}
}

// NewFutureTaskAndRun creates a FutureTask and submits it to the
// default pool returned by [DefaultPool].
func NewFutureTaskAndRun[V any](task func(interrupt <-chan struct{}) (V, error)) (*FutureTask[V], error) {
	return NewFutureTaskAndRunWithPool(task, DefaultPool())
}

// NewFutureTaskAndRunWithPool creates a FutureTask and submits it to
// the given pool. Panics if pool is nil.
func NewFutureTaskAndRunWithPool[V any](task func(interrupt <-chan struct{}) (V, error), pool Pool) (*FutureTask[V], error) {
	if pool == nil {
		panic("sync: pool is nil")
	}
	f := NewFutureTask(task)
	if err := pool.Submit(f.Run); err != nil {
		return nil, err
	}
	return f, nil
}

// complete records the final value or error and closes the done /
// interrupt channels. It runs at most once.
func (f *FutureTask[V]) complete(v V, err error) {
	f.doneOnce.Do(func() {
		if err != nil {
			f.state.Store(FutureStateFailed.int32())
			f.error = err
		} else {
			f.state.Store(FutureStateSucceeded.int32())
			f.value = v
		}
		close(f.done)
		select {
		case <-f.interrupt:
		default:
			close(f.interrupt)
		}
	})
}

// Run executes the task. Subsequent calls are no-ops.
func (f *FutureTask[V]) Run() {
	if !f.State().IsCreated() {
		return
	}
	f.runOnce.Do(func() {
		if f.state.CompareAndSwap(FutureStateCreated.int32(), FutureStateRunning.int32()) {
			v, err := f.task(f.interrupt)
			f.complete(v, err)
		}
	})
}

// Cancel implements [Future.Cancel].
func (f *FutureTask[V]) Cancel(mayInterruptIfRunning bool) bool {
	canceled := false
	f.doneOnce.Do(func() {
		f.state.Store(FutureStateCancelled.int32())
		f.error = ErrFutureCancelled
		if mayInterruptIfRunning {
			close(f.interrupt)
		}
		close(f.done)
		canceled = true
	})
	return canceled
}

// IsCancelled reports whether the future was canceled.
func (f *FutureTask[V]) IsCancelled() bool { return f.State().IsCancelled() }

// IsDone reports whether the future has completed.
func (f *FutureTask[V]) IsDone() bool {
	select {
	case <-f.done:
		return true
	default:
		return false
	}
}

// Get blocks until completion and returns the result.
func (f *FutureTask[V]) Get() (V, error) {
	<-f.done
	return f.value, f.error
}

// GetWithTimeout blocks until completion or until timeout elapses.
// On timeout the task is canceled and [ErrFutureTimedOut] returned.
func (f *FutureTask[V]) GetWithTimeout(timeout time.Duration) (V, error) {
	if timeout <= 0 {
		return f.tryGetOrTimeout()
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-f.done:
		return f.value, f.error
	case <-timer.C:
		select {
		case <-f.done:
			return f.value, f.error
		default:
			f.Cancel(true)
			var zero V
			return zero, ErrFutureTimedOut
		}
	}
}

// tryGetOrTimeout returns the result if already available, otherwise
// cancels and returns [ErrFutureTimedOut].
func (f *FutureTask[V]) tryGetOrTimeout() (V, error) {
	select {
	case <-f.done:
		return f.value, f.error
	default:
		f.Cancel(true)
		var zero V
		return zero, ErrFutureTimedOut
	}
}

// GetWithContext blocks until completion or until ctx is done.
func (f *FutureTask[V]) GetWithContext(ctx context.Context) (V, error) {
	select {
	case <-f.done:
		return f.value, f.error
	case <-ctx.Done():
		select {
		case <-f.done:
			return f.value, f.error
		default:
			f.Cancel(true)
			var zero V
			return zero, errors.Join(f.error, ctx.Err())
		}
	}
}

// TryGet returns the result without blocking. The boolean is true only
// after completion.
func (f *FutureTask[V]) TryGet() (V, error, bool) {
	select {
	case <-f.done:
		return f.value, f.error, true
	default:
		var zero V
		return zero, nil, false
	}
}

// State returns the current [FutureState].
func (f *FutureTask[V]) State() FutureState {
	return FutureState(f.state.Load())
}
