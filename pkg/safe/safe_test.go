package safe

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewPanicError(t *testing.T) {
	tests := []struct {
		name string
		info any
		want []string
	}{
		{"string", "boom", []string{"boom", "panic recovered", "timestamp:", "error:", "stack:"}},
		{"int", 42, []string{"42"}},
		{"error value", errors.New("custom"), []string{"custom"}},
		{"nil", nil, []string{"panic recovered"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := time.Now()
			err := NewPanicError(tt.info, []byte("fake stack"))
			after := time.Now()

			pe, ok := errors.AsType[*PanicError](err)
			if !ok {
				t.Fatalf("type = %T, want *PanicError", err)
			}
			if pe.time.Before(before) || pe.time.After(after) {
				t.Errorf("timestamp %v out of range [%v, %v]", pe.time, before, after)
			}
			if pe.info != tt.info {
				t.Errorf("info = %v, want %v", pe.info, tt.info)
			}
			msg := err.Error()
			for _, want := range tt.want {
				if !strings.Contains(msg, want) {
					t.Errorf("message missing %q: %s", want, msg)
				}
			}
		})
	}
}

func TestWithRecover_NilFn(t *testing.T) {
	if got := WithRecover(nil); got != nil {
		t.Errorf("WithRecover(nil) = %p, want nil", got)
	}
}

func TestWithRecover_NoPanic(t *testing.T) {
	called := false
	WithRecover(func() { called = true })()
	if !called {
		t.Error("fn was not called")
	}
}

func TestWithRecover_RecoversPanic(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want string
	}{
		{"string", "boom", "boom"},
		{"int", 123, "123"},
		{"error", errors.New("err"), "err"},
		{"struct", struct{ Code int }{500}, "500"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got error
			WithRecover(func() { panic(tt.val) }, func(err error) { got = err })()
			if got == nil {
				t.Fatal("handler not called")
			}
			if !strings.Contains(got.Error(), tt.want) {
				t.Errorf("err = %q, want substring %q", got, tt.want)
			}
		})
	}
}

func TestWithRecover_MultipleHandlers(t *testing.T) {
	var n atomic.Int32
	h := func(error) { n.Add(1) }
	WithRecover(func() { panic("x") }, h, h, h)()
	if got := n.Load(); got != 3 {
		t.Errorf("handler calls = %d, want 3", got)
	}
}

func TestWithRecover_NoHandlerSwallowsPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic propagated: %v", r)
		}
	}()
	WithRecover(func() { panic("x") })()
}

func TestWithRecover_HandlerPanicDoesNotPropagate(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("handler panic propagated: %v", r)
		}
	}()
	bad := func(error) { panic("handler boom") }
	WithRecover(func() { panic("orig") }, bad)()
}

func TestGo_RunsFn(t *testing.T) {
	done := make(chan struct{})
	Go(func() { close(done) })
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("fn did not run")
	}
}

func TestGo_NilFnIsNop(t *testing.T) {
	Go(nil) // must not panic
}

func TestGo_RecoversPanic(t *testing.T) {
	got := make(chan error, 1)
	Go(func() { panic("goroutine boom") }, func(err error) { got <- err })
	select {
	case err := <-got:
		if !strings.Contains(err.Error(), "goroutine boom") {
			t.Errorf("err = %q, want substring %q", err, "goroutine boom")
		}
	case <-time.After(time.Second):
		t.Fatal("handler not called")
	}
}

func TestGo_ConcurrentPanics(t *testing.T) {
	const n = 100
	var got atomic.Int32
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		Go(func() {
			defer wg.Done()
			panic("x")
		}, func(error) { got.Add(1) })
	}
	wg.Wait()
	// Allow handlers to run after wg.Done.
	deadline := time.Now().Add(2 * time.Second)
	for got.Load() < n && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got.Load() != n {
		t.Errorf("handler calls = %d, want %d", got.Load(), n)
	}
}

func BenchmarkWithRecover_NoPanic(b *testing.B) {
	wrapped := WithRecover(func() {})
	for b.Loop() {
		wrapped()
	}
}

func BenchmarkWithRecover_Panic(b *testing.B) {
	wrapped := WithRecover(func() { panic("x") }, func(error) {})
	for b.Loop() {
		wrapped()
	}
}

func BenchmarkNewPanicError(b *testing.B) {
	stack := []byte("test stack trace\nline 2\nline 3")
	for b.Loop() {
		_ = NewPanicError("boom", stack)
	}
}
