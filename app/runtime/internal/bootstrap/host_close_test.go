package bootstrap

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/infra/teardown"
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
			goalDriver:     shutdownFunc{stop: recordStop("goals"), wait: recordWait("goals")},
			mcpCoordinator: shutdownFunc{stop: recordStop("mcp"), wait: recordWait("mcp")},
			runCoordinator: shutdownFunc{stop: recordStop("active-runs"), wait: recordWait("active-runs")},
			executor:       shutdownFunc{stop: recordStop("active-execution-tree"), wait: recordWait("active-execution-tree")},
			runEffectTasks: shutdownFunc{stop: recordStop("effects"), wait: recordWait("effects")},
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
		"stop active-runs",
		"stop effects",
		"wait goals",
		"wait mcp",
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

func TestHostCloseContinuesGraphAfterCallerTimeout(t *testing.T) {
	releaseComponent := make(chan struct{})
	toolClosed := make(chan struct{})
	host := Host{lifetime: &hostLifetime{
		shutdownTimeout: time.Millisecond,
		runCoordinator: shutdownFunc{
			wait: func(ctx context.Context) error {
				select {
				case <-releaseComponent:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			},
		},
		toolResources: terminalClosers([]func() error{func() error {
			close(toolClosed)
			return nil
		}}),
	}}
	if err := host.Close(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close error = %v, want deadline exceeded", err)
	}
	select {
	case <-toolClosed:
		t.Fatal("tool dependency closed despite an unjoined component")
	default:
	}

	// A failed Open does not return the Host, so no external caller exists to
	// issue another Close. The Host generation itself must retain this graph and
	// advance once the component finishes after the caller's wait deadline.
	close(releaseComponent)
	select {
	case <-toolClosed:
	case <-time.After(time.Second):
		t.Fatal("Host abandoned its dependent resource graph after caller timeout")
	}
}

func TestHostCloseStartsNewGenerationAfterComponentError(t *testing.T) {
	want := errors.New("component did not settle")
	var stops, attempts, closed int
	host := Host{lifetime: &hostLifetime{
		runCoordinator: shutdownFunc{
			stop: func() { stops++ },
			wait: func(context.Context) error {
				attempts++
				if attempts == 1 {
					return want
				}
				return nil
			},
		},
		toolResources: terminalClosers([]func() error{func() error { closed++; return nil }}),
	}}
	if err := host.Close(); !errors.Is(err, want) {
		t.Fatalf("first Close error = %v, want component failure", err)
	}
	if stops != 1 || attempts != 1 || closed != 0 {
		t.Fatalf("after failed generation: stops=%d attempts=%d closed=%d, want 1/1/0", stops, attempts, closed)
	}
	if err := host.Close(); err != nil {
		t.Fatalf("retry Close: %v", err)
	}
	if stops != 1 || attempts != 2 || closed != 1 {
		t.Fatalf("after retry generation: stops=%d attempts=%d closed=%d, want 1/2/1", stops, attempts, closed)
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

func (c closerFunc) Close() error { return c() }

type shutdownFunc struct {
	stop func()
	wait func(context.Context) error
}

func (s shutdownFunc) BeginShutdown() {
	if s.stop != nil {
		s.stop()
	}
}

func (s shutdownFunc) AwaitShutdown(ctx context.Context) error {
	if s.wait == nil {
		return nil
	}
	return s.wait(ctx)
}

func (s shutdownFunc) Cancel() { s.BeginShutdown() }

func (s shutdownFunc) Wait(ctx context.Context) error { return s.AwaitShutdown(ctx) }
