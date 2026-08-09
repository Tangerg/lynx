package bootstrap

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/shutdown"
)

func TestClosePendingResourcesPreservesDependenciesAfterFailure(t *testing.T) {
	firstErr := errors.New("first")
	lastErr := errors.New("last")
	var calls []int
	pending, err := closePendingResources(t.Context(), []ShutdownResource{
		shutdownResourceFunc(func(context.Context) error { calls = append(calls, 1); return firstErr }),
		nil,
		shutdownResourceFunc(func(context.Context) error { calls = append(calls, 3); return lastErr }),
	})
	if !errors.Is(err, lastErr) || errors.Is(err, firstErr) {
		t.Fatalf("closePendingResources err = %v, want only the first reverse-order failure", err)
	}
	if !slices.Equal(calls, []int{3}) {
		t.Fatalf("calls = %v, want [3]", calls)
	}
	if len(pending) != 3 || pending[0] == nil || pending[1] != nil || pending[2] == nil {
		t.Fatalf("pending = %v, want the unresolved creation-order prefix", pending)
	}
}

func TestHostCloseOwnsReverseOrderAndIsIdempotentAcrossCopies(t *testing.T) {
	var (
		mu    sync.Mutex
		calls []string
	)
	record := func(name string, err error) func() error {
		return func() error {
			mu.Lock()
			calls = append(calls, name)
			mu.Unlock()
			return err
		}
	}
	recordStop := func(name string) func() {
		return func() { _ = record("stop "+name, nil)() }
	}
	recordWait := func(name string) func(context.Context) error {
		return func(context.Context) error { return record("wait "+name, nil)() }
	}
	host := Host{
		lifetime: &hostLifetime{
			goals:        shutdownFunc{stop: recordStop("goals"), wait: recordWait("goals")},
			mcp:          shutdownFunc{stop: recordStop("mcp"), wait: recordWait("mcp")},
			codebase:     shutdownFunc{stop: recordStop("codebase"), wait: recordWait("codebase")},
			coordinator:  shutdownFunc{stop: recordStop("active-runs"), wait: recordWait("active-runs")},
			execution:    shutdownFunc{stop: recordStop("active-execution-tree"), wait: recordWait("active-execution-tree")},
			effectsTasks: shutdownFunc{stop: recordStop("effects"), wait: recordWait("effects")},
			toolClosers: []ShutdownResource{
				closerFunc(record("tool-1", nil)),
				closerFunc(record("tool-2", nil)),
			},
			resources: []ShutdownResource{
				closerFunc(record("resource-1", nil)),
				closerFunc(record("resource-2", nil)),
			},
		},
	}
	copyOfHost := host
	copyOfHost.Stack = Stack{}

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for index := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if index%2 == 0 {
				errs <- host.Close()
				return
			}
			errs <- copyOfHost.Close()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Close error = %v, want nil", err)
		}
	}
	wantCalls := []string{
		"stop goals",
		"stop mcp",
		"stop codebase",
		"stop active-runs",
		"stop effects",
		"wait goals",
		"wait mcp",
		"wait codebase",
		"wait active-runs",
		"wait effects",
		"stop active-execution-tree",
		"wait active-execution-tree",
		"tool-2",
		"tool-1",
		"resource-2",
		"resource-1",
	}
	if !slices.Equal(calls, wantCalls) {
		t.Fatalf("close calls = %v, want %v", calls, wantCalls)
	}
}

func TestHostCloseRetriesOnlyUnclosedDependencies(t *testing.T) {
	toolErr := errors.New("tool close")
	resourceErr := errors.New("resource close")
	var toolCalls, resourceCalls, successfulCalls int
	host := Host{lifetime: &hostLifetime{
		toolClosers: []ShutdownResource{
			closerFunc(func() error { successfulCalls++; return nil }),
			closerFunc(func() error {
				toolCalls++
				if toolCalls == 1 {
					return toolErr
				}
				return nil
			}),
		},
		resources: []ShutdownResource{closerFunc(func() error {
			resourceCalls++
			if resourceCalls == 1 {
				return resourceErr
			}
			return nil
		})},
	}}
	if err := host.Close(); !errors.Is(err, toolErr) || errors.Is(err, resourceErr) {
		t.Fatalf("first Close error = %v, want tool dependency error only", err)
	}
	if successfulCalls != 0 || toolCalls != 1 || resourceCalls != 0 {
		t.Fatalf("first close calls = success:%d tool:%d resource:%d", successfulCalls, toolCalls, resourceCalls)
	}
	if err := host.Close(); !errors.Is(err, resourceErr) {
		t.Fatalf("second Close error = %v, want resource error", err)
	}
	if successfulCalls != 1 || toolCalls != 2 || resourceCalls != 1 {
		t.Fatalf("second close calls = success:%d tool:%d resource:%d", successfulCalls, toolCalls, resourceCalls)
	}
	if err := host.Close(); err != nil {
		t.Fatalf("third Close: %v", err)
	}
	if successfulCalls != 1 || toolCalls != 2 || resourceCalls != 2 {
		t.Fatalf("retry close replayed closed dependency: success:%d tool:%d resource:%d", successfulCalls, toolCalls, resourceCalls)
	}
}

func TestHostCloseDoesNotCloseDependenciesAfterComponentJoinTimeout(t *testing.T) {
	toolClosed := false
	host := Host{lifetime: &hostLifetime{
		shutdownTimeout: time.Millisecond,
		coordinator: shutdownFunc{
			wait: func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			},
		},
		toolClosers: []ShutdownResource{closerFunc(func() error {
			toolClosed = true
			return nil
		})},
	}}
	if err := host.Close(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close error = %v, want deadline exceeded", err)
	}
	if toolClosed {
		t.Fatal("tool dependency closed despite an unjoined component")
	}
}

func TestHostCloseRetriesDrainAfterTimeout(t *testing.T) {
	var (
		stops  int
		ready  bool
		closed int
	)
	host := Host{lifetime: &hostLifetime{
		shutdownTimeout: time.Millisecond,
		coordinator: shutdownFunc{
			stop: func() { stops++ },
			wait: func(ctx context.Context) error {
				if ready {
					return nil
				}
				<-ctx.Done()
				return ctx.Err()
			},
		},
		toolClosers: []ShutdownResource{closerFunc(func() error { closed++; return nil })},
	}}
	if err := host.Close(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first Close error = %v, want deadline exceeded", err)
	}
	if stops != 1 || closed != 0 {
		t.Fatalf("after timed out close: stops=%d closed=%d, want 1/0", stops, closed)
	}
	ready = true
	if err := host.Close(); err != nil {
		t.Fatalf("retry Close: %v", err)
	}
	if stops != 1 || closed != 1 {
		t.Fatalf("after retry close: stops=%d closed=%d, want 1/1", stops, closed)
	}
}

func TestHostCloseBoundsAndRetriesContextAwareResource(t *testing.T) {
	var (
		attempts int
		ready    bool
	)
	host := Host{lifetime: &hostLifetime{
		shutdownTimeout: time.Millisecond,
		resources: []ShutdownResource{shutdownResourceFunc(func(ctx context.Context) error {
			attempts++
			if ready {
				return nil
			}
			<-ctx.Done()
			return ctx.Err()
		})},
	}}
	if err := host.Close(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first Close error = %v, want deadline exceeded", err)
	}
	if attempts != 1 {
		t.Fatalf("resource shutdown attempts = %d, want 1", attempts)
	}
	ready = true
	if err := host.Close(); err != nil {
		t.Fatalf("retry Close: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("resource shutdown attempts = %d, want 2", attempts)
	}
}

func TestHostCloseBoundsNonCooperativeToolCloserWithoutConcurrentRetry(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	host := Host{lifetime: &hostLifetime{
		shutdownTimeout: time.Millisecond,
		toolClosers: []ShutdownResource{shutdown.New(func(context.Context) error {
			calls.Add(1)
			close(started)
			<-release // Models a third-party Close with no cancellation support.
			return nil
		})},
	}}
	if err := host.Close(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first Close error = %v, want deadline exceeded", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("non-cooperative closer did not start")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("closer calls after deadline = %d, want 1", got)
	}

	close(release)
	host.lifetime.shutdownTimeout = time.Second
	if err := host.Close(); err != nil {
		t.Fatalf("retry Close: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("retry launched a second closer = %d, want 1", got)
	}
}

type closerFunc func() error

func (f closerFunc) Close() error { return f() }

func (f closerFunc) Shutdown(context.Context) error { return f() }

type shutdownResourceFunc func(context.Context) error

func (f shutdownResourceFunc) Shutdown(ctx context.Context) error { return f(ctx) }

type shutdownFunc struct {
	stop func()
	wait func(context.Context) error
}

func (f shutdownFunc) BeginShutdown() {
	if f.stop != nil {
		f.stop()
	}
}

func (f shutdownFunc) AwaitShutdown(ctx context.Context) error {
	if f.wait == nil {
		return nil
	}
	return f.wait(ctx)
}

func (f shutdownFunc) Cancel() { f.BeginShutdown() }

func (f shutdownFunc) Wait(ctx context.Context) error { return f.AwaitShutdown(ctx) }
