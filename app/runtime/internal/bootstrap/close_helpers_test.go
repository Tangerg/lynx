package bootstrap

import (
	"errors"
	"io"
	"slices"
	"sync"
	"testing"
)

func TestRunClosersRunsAllInReverseAndJoinsErrors(t *testing.T) {
	firstErr := errors.New("first")
	lastErr := errors.New("last")
	var calls []int
	err := runClosers([]func() error{
		func() error { calls = append(calls, 1); return firstErr },
		nil,
		func() error { calls = append(calls, 3); return lastErr },
	})
	if !errors.Is(err, firstErr) || !errors.Is(err, lastErr) {
		t.Fatalf("runClosers err = %v, want both errors", err)
	}
	if !slices.Equal(calls, []int{3, 1}) {
		t.Fatalf("calls = %v, want [3 1]", calls)
	}
}

func TestHostCloseOwnsReverseOrderAndIsIdempotentAcrossCopies(t *testing.T) {
	toolErr := errors.New("tool close")
	resourceErr := errors.New("resource close")
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
	recordVoid := func(name string) func() {
		return func() { _ = record(name, nil)() }
	}
	host := Host{
		lifetime: &hostLifetime{
			integrations: shutdownFunc(recordVoid("integrations")),
			codebase:     shutdownFunc(recordVoid("codebase")),
			coordinator:  shutdownFunc(recordVoid("active-runs")),
			dispatcher:   closerFunc(record("active-turn-tree", nil)),
			effectsTasks: shutdownFunc(recordVoid("effects")),
			toolClosers: []func() error{
				record("tool-1", nil),
				record("tool-2", toolErr),
			},
			resources: []io.Closer{
				closerFunc(record("resource-1", nil)),
				closerFunc(record("resource-2", resourceErr)),
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
		if !errors.Is(err, toolErr) || !errors.Is(err, resourceErr) {
			t.Fatalf("Close error = %v, want joined tool/resource errors", err)
		}
	}
	wantCalls := []string{
		"integrations",
		"codebase",
		"active-runs",
		"active-turn-tree",
		"effects",
		"tool-2",
		"tool-1",
		"resource-2",
		"resource-1",
	}
	if !slices.Equal(calls, wantCalls) {
		t.Fatalf("close calls = %v, want %v", calls, wantCalls)
	}
}

type closerFunc func() error

func (f closerFunc) Close() error { return f() }

type shutdownFunc func()

func (f shutdownFunc) Close() { f() }
