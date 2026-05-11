package retry

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

var (
	errTemporary = errors.New("temporary error")
	errFatal     = errors.New("fatal error")
)

// noSleep returns immediately, eliminating real waits in tests.
func noSleep() func(time.Duration) <-chan time.Time {
	return func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}
}

func TestDefaultStrategy(t *testing.T) {
	s := defaultStrategy()
	if s.maxAttempts != 3 {
		t.Errorf("maxAttempts = %d, want 3", s.maxAttempts)
	}
	if s.delayConfig.BaseDelay != 100*time.Millisecond {
		t.Errorf("BaseDelay = %v", s.delayConfig.BaseDelay)
	}
	if s.delayConfig.MaxJitter != 100*time.Millisecond {
		t.Errorf("MaxJitter = %v", s.delayConfig.MaxJitter)
	}
	if s.context != context.Background() {
		t.Error("context != Background()")
	}
}

func TestDo_Success(t *testing.T) {
	calls := 0
	err := Do(func() error { calls++; return nil }, WithSleep(noSleep()))
	if err != nil {
		t.Errorf("err = %v", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestDo_RetryUntilSuccess(t *testing.T) {
	calls := 0
	err := Do(func() error {
		calls++
		if calls < 3 {
			return errTemporary
		}
		return nil
	}, WithMaxAttempts(5), WithSleep(noSleep()))
	if err != nil {
		t.Errorf("err = %v", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestDo_MaxAttemptsExhausted(t *testing.T) {
	calls := 0
	err := Do(func() error { calls++; return errTemporary },
		WithMaxAttempts(3), WithSleep(noSleep()))
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errTemporary) {
		t.Errorf("err = %v, want wraps errTemporary", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestDo_Unlimited(t *testing.T) {
	var calls atomic.Int32
	err := Do(func() error {
		if calls.Add(1) < 10 {
			return errTemporary
		}
		return nil
	}, WithUnlimitedAttempts(), WithSleep(noSleep()))
	if err != nil {
		t.Errorf("err = %v", err)
	}
	if got := calls.Load(); got != 10 {
		t.Errorf("calls = %d, want 10", got)
	}
}

func TestDo_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	calls := 0
	err := Do(func() error {
		calls++
		if calls == 2 {
			cancel()
		}
		return errTemporary
	}, WithContext(ctx), WithMaxAttempts(10), WithSleep(noSleep()))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if calls < 2 {
		t.Errorf("calls = %d, want >= 2", calls)
	}
}

func TestDo_ContextDeadlineExceeded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := Do(func() error { return errTemporary },
		WithContext(ctx), WithMaxAttempts(10), WithBaseDelay(100*time.Millisecond))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want DeadlineExceeded", err)
	}
}

func TestDo_PreCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	calls := 0
	err := Do(func() error { calls++; return nil },
		WithContext(ctx), WithSleep(noSleep()))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if calls != 0 {
		t.Errorf("calls = %d, want 0", calls)
	}
}

func TestDo_RetryCondition(t *testing.T) {
	calls := 0
	err := Do(func() error {
		calls++
		if calls == 2 {
			return errFatal
		}
		return errTemporary
	},
		WithMaxAttempts(10),
		WithSleep(noSleep()),
		WithRetryCondition(func(err error) bool { return !errors.Is(err, errFatal) }),
	)
	if !errors.Is(err, errFatal) {
		t.Errorf("err = %v, want wraps errFatal", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestDo_OnRetryCallback(t *testing.T) {
	calls := 0
	var attempts []int
	err := Do(func() error {
		calls++
		if calls < 3 {
			return fmt.Errorf("fail %d", calls)
		}
		return nil
	},
		WithMaxAttempts(5),
		WithSleep(noSleep()),
		WithOnRetry(func(attempt int, _ error) { attempts = append(attempts, attempt) }),
	)
	if err != nil {
		t.Errorf("err = %v", err)
	}
	if want := []int{1, 2}; !equalIntSlice(attempts, want) {
		t.Errorf("attempts = %v, want %v", attempts, want)
	}
}

func TestDo_OnSuccessCallback(t *testing.T) {
	calls := 0
	gotAttempt := 0
	err := Do(func() error {
		calls++
		if calls < 3 {
			return errTemporary
		}
		return nil
	},
		WithMaxAttempts(5),
		WithSleep(noSleep()),
		WithOnSuccess(func(a int) { gotAttempt = a }),
	)
	if err != nil {
		t.Errorf("err = %v", err)
	}
	if gotAttempt != 3 {
		t.Errorf("success attempt = %d, want 3", gotAttempt)
	}
}

func TestDoWithResult(t *testing.T) {
	calls := 0
	got, err := DoWithResult(func() (string, error) {
		calls++
		if calls < 3 {
			return "", errTemporary
		}
		return "ok", nil
	}, WithMaxAttempts(5), WithSleep(noSleep()))
	if err != nil || got != "ok" {
		t.Errorf("got %q err %v", got, err)
	}
}

func TestDoWithResult_Failure(t *testing.T) {
	got, err := DoWithResult(func() (int, error) { return 0, errTemporary },
		WithMaxAttempts(2), WithSleep(noSleep()))
	if got != 0 {
		t.Errorf("got %d, want zero", got)
	}
	if !errors.Is(err, errTemporary) {
		t.Errorf("err = %v", err)
	}
}

func TestRetrier_Reusable(t *testing.T) {
	r := NewRetrier(WithMaxAttempts(3), WithSleep(noSleep()))
	for i := range 3 {
		calls := 0
		err := r.Do(func() error {
			calls++
			if calls < 2 {
				return errTemporary
			}
			return nil
		})
		if err != nil {
			t.Errorf("[%d] err = %v", i, err)
		}
	}
}

func TestExponentialBackoff(t *testing.T) {
	cfg := DelayConfig{BaseDelay: 100 * time.Millisecond, MaxBackoffStep: 10}
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 100 * time.Millisecond},
		{1, 200 * time.Millisecond},
		{2, 400 * time.Millisecond},
		{3, 800 * time.Millisecond},
	}
	for _, tt := range tests {
		if got := ExponentialBackoff(tt.attempt, nil, cfg); got != tt.want {
			t.Errorf("attempt %d: got %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

func TestExponentialBackoff_MaxDelay(t *testing.T) {
	cfg := DelayConfig{
		BaseDelay:      100 * time.Millisecond,
		MaxDelay:       500 * time.Millisecond,
		MaxBackoffStep: 10,
	}
	if got := ExponentialBackoff(10, nil, cfg); got != 500*time.Millisecond {
		t.Errorf("got %v, want capped at 500ms", got)
	}
}

func TestFixedDelay(t *testing.T) {
	cfg := DelayConfig{BaseDelay: 250 * time.Millisecond}
	for _, attempt := range []int{0, 1, 5} {
		if got := FixedDelay(attempt, nil, cfg); got != cfg.BaseDelay {
			t.Errorf("attempt %d: got %v", attempt, got)
		}
	}
}

func TestRandomJitter(t *testing.T) {
	cfg := DelayConfig{MaxJitter: 100 * time.Millisecond}
	for range 100 {
		got := RandomJitter(0, nil, cfg)
		if got < 0 || got >= 100*time.Millisecond {
			t.Fatalf("got %v, out of [0, 100ms)", got)
		}
	}
	if got := RandomJitter(0, nil, DelayConfig{}); got != 0 {
		t.Errorf("zero MaxJitter: got %v, want 0", got)
	}
}

func TestFullJitterBackoff(t *testing.T) {
	cfg := DelayConfig{BaseDelay: 100 * time.Millisecond, MaxBackoffStep: 10}
	for attempt := 1; attempt < 5; attempt++ {
		got := FullJitterBackoff(attempt, nil, cfg)
		ceiling := cfg.BaseDelay << attempt
		if got < 0 || got >= ceiling {
			t.Errorf("attempt %d: got %v, ceiling %v", attempt, got, ceiling)
		}
	}
}

func TestCombineDelays(t *testing.T) {
	d := CombineDelays(
		func(_ int, _ error, _ DelayConfig) time.Duration { return 100 * time.Millisecond },
		func(_ int, _ error, _ DelayConfig) time.Duration { return 50 * time.Millisecond },
	)
	if got := d(1, nil, DelayConfig{}); got != 150*time.Millisecond {
		t.Errorf("got %v, want 150ms", got)
	}
}

func TestWithDelayConfig_NormalizesNegatives(t *testing.T) {
	r := NewRetrier(WithDelayConfig(DelayConfig{
		BaseDelay:      -100 * time.Millisecond,
		MaxDelay:       -1 * time.Second,
		MaxJitter:      -50 * time.Millisecond,
		MaxBackoffStep: -5,
	}))
	cfg := r.inner.strategy.delayConfig
	if cfg.BaseDelay != 0 || cfg.MaxDelay != 0 || cfg.MaxJitter != 0 {
		t.Errorf("negative not normalized: %+v", cfg)
	}
}

func TestStrategyOptions_NilFunctionsIgnored(t *testing.T) {
	var nilCtx context.Context //nolint:staticcheck // intentionally test nil-context guard
	r := NewRetrier(
		WithRetryCondition(nil),
		WithDelayFunc(nil),
		WithSleep(nil),
		WithOnRetry(nil),
		WithOnSuccess(nil),
		WithContext(nilCtx),
	)
	// No panic, defaults preserved.
	err := r.Do(func() error { return nil })
	if err != nil {
		t.Errorf("err = %v", err)
	}
}

func TestCalculateMaxBackoffStep(t *testing.T) {
	tests := []struct {
		base time.Duration
		min  int
	}{
		{0, 1},                       // any positive step is OK
		{1 * time.Nanosecond, 60},    // ~62
		{100 * time.Millisecond, 30}, // baseline check
		{1 * time.Second, 25},
	}
	for _, tt := range tests {
		got := calculateMaxBackoffStep(tt.base)
		if got < tt.min {
			t.Errorf("base %v: got %d, want >= %d", tt.base, got, tt.min)
		}
	}
}

func equalIntSlice(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func BenchmarkDo_NoRetry(b *testing.B) {
	op := func() error { return nil }
	for b.Loop() {
		_ = Do(op, WithSleep(noSleep()))
	}
}

func BenchmarkRetrier_Reused(b *testing.B) {
	r := NewRetrier(WithSleep(noSleep()))
	op := func() error { return nil }
	for b.Loop() {
		_ = r.Do(op)
	}
}

func BenchmarkExponentialBackoff(b *testing.B) {
	cfg := DelayConfig{BaseDelay: time.Millisecond, MaxBackoffStep: 10}
	for b.Loop() {
		_ = ExponentialBackoff(5, nil, cfg)
	}
}
