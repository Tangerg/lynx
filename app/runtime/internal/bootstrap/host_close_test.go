package bootstrap

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/teardown"
)

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
			goalDriver:          shutdownFunc{stop: recordStop("goals"), wait: recordWait("goals")},
			mcpCoordinator:      shutdownFunc{stop: recordStop("mcp"), wait: recordWait("mcp")},
			codebaseCoordinator: shutdownFunc{stop: recordStop("codebase"), wait: recordWait("codebase")},
			runCoordinator:      shutdownFunc{stop: recordStop("active-runs"), wait: recordWait("active-runs")},
			executor:            shutdownFunc{stop: recordStop("active-execution-tree"), wait: recordWait("active-execution-tree")},
			runEffectTasks:      shutdownFunc{stop: recordStop("effects"), wait: recordWait("effects")},
			toolResources: terminalClosers([]func() error{
				closerFunc(record("tool-1", nil)),
				closerFunc(record("tool-2", nil)),
			}),
			hostResources: terminalClosers([]func() error{
				closerFunc(record("resource-1", nil)),
				closerFunc(record("resource-2", nil)),
			}),
		},
	}
	copyOfHost := host

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

func TestHostCloseAdvancesPastCompletedCloserError(t *testing.T) {
	closeErr := errors.New("terminal close diagnostic")
	var toolCalls, resourceCalls int
	oneShotToolClose := sync.OnceValue(func() error {
		toolCalls++
		return closeErr
	})
	host := Host{lifetime: &hostLifetime{
		// A2A, LSP, Shells and SQLite all use this one-shot close shape: the
		// resource reaches its terminal state on the first call even when that
		// call reports a diagnostic. Replaying the same cached error can never
		// make more cleanup progress.
		toolResources: terminalClosers([]func() error{oneShotToolClose}),
		hostResources: terminalClosers([]func() error{func() error {
			resourceCalls++
			return nil
		}}),
	}}

	if err := host.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("first Close error = %v, want terminal diagnostic", err)
	}
	if toolCalls != 1 {
		t.Fatalf("one-shot tool close calls = %d, want 1", toolCalls)
	}
	if resourceCalls != 1 {
		t.Fatalf("dependent resource close calls = %d, want 1 after tool reached its terminal state", resourceCalls)
	}
	if err := host.Close(); err != nil {
		t.Fatalf("second Close = %v, want already-closed Host", err)
	}
	if toolCalls != 1 || resourceCalls != 1 {
		t.Fatalf("second Close replayed terminal work: tool=%d resource=%d", toolCalls, resourceCalls)
	}
}

func TestHostCloseDoesNotCloseDependenciesAfterComponentJoinTimeout(t *testing.T) {
	toolClosed := false
	host := Host{lifetime: &hostLifetime{
		shutdownTimeout: time.Millisecond,
		runCoordinator: shutdownFunc{
			wait: func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			},
		},
		toolResources: terminalClosers([]func() error{func() error {
			toolClosed = true
			return nil
		}}),
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
		runCoordinator: shutdownFunc{
			stop: func() { stops++ },
			wait: func(ctx context.Context) error {
				if ready {
					return nil
				}
				<-ctx.Done()
				return ctx.Err()
			},
		},
		toolResources: terminalClosers([]func() error{func() error { closed++; return nil }}),
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

func TestHostCloseBoundsNonCooperativeToolCloserWithoutConcurrentRetry(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	host := Host{lifetime: &hostLifetime{
		shutdownTimeout: time.Millisecond,
		toolResources: []*teardown.Step{teardown.Terminal(func(context.Context) error {
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
