package retry

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// Mock errors for testing
var (
	errTemporary = errors.New("temporary error")
	errFatal     = errors.New("fatal error")
)

// mockSleep returns a function that simulates sleep without actually sleeping
func mockSleep() func(duration time.Duration) <-chan time.Time {
	return func(duration time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}
}

// TestDefaultStrategy tests the default strategy configuration
func TestDefaultStrategy(t *testing.T) {
	s := defaultStrategy()

	if s.maxAttempts != 3 {
		t.Errorf("expected maxAttempts=3, got %d", s.maxAttempts)
	}

	if s.delayConfig.BaseDelay != 100*time.Millisecond {
		t.Errorf("expected BaseDelay=100ms, got %v", s.delayConfig.BaseDelay)
	}

	if s.delayConfig.MaxDelay != 0 {
		t.Errorf("expected MaxDelay=0, got %v", s.delayConfig.MaxDelay)
	}

	if s.delayConfig.MaxJitter != 100*time.Millisecond {
		t.Errorf("expected MaxJitter=100ms, got %v", s.delayConfig.MaxJitter)
	}

	if s.context != context.Background() {
		t.Error("expected context.Background()")
	}
}

// TestDoSuccess tests successful operation without retry
func TestDoSuccess(t *testing.T) {
	callCount := 0
	operation := func() error {
		callCount++
		return nil
	}

	err := Do(operation, WithMaxAttempts(3), WithSleep(mockSleep()))

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

// TestDoRetryUntilSuccess tests retry until operation succeeds
func TestDoRetryUntilSuccess(t *testing.T) {
	callCount := 0
	operation := func() error {
		callCount++
		if callCount < 3 {
			return errTemporary
		}
		return nil
	}

	err := Do(operation, WithMaxAttempts(5), WithSleep(mockSleep()))

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

// TestDoMaxAttemptsReached tests failure after max attempts
func TestDoMaxAttemptsReached(t *testing.T) {
	callCount := 0
	operation := func() error {
		callCount++
		return errTemporary
	}

	err := Do(operation, WithMaxAttempts(3), WithSleep(mockSleep()))

	if err == nil {
		t.Error("expected error, got nil")
	}

	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}

	if !errors.Is(err, errTemporary) {
		t.Errorf("expected error to wrap errTemporary, got %v", err)
	}
}

// TestDoWithUnlimitedAttempts tests unlimited retry mode
func TestDoWithUnlimitedAttempts(t *testing.T) {
	callCount := int32(0)
	operation := func() error {
		count := atomic.AddInt32(&callCount, 1)
		if count < 10 {
			return errTemporary
		}
		return nil
	}

	err := Do(operation, WithUnlimitedAttempts(), WithSleep(mockSleep()))

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if atomic.LoadInt32(&callCount) != 10 {
		t.Errorf("expected 10 calls, got %d", callCount)
	}
}

// TestDoWithContext tests context cancellation
func TestDoWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	callCount := 0
	operation := func() error {
		callCount++
		if callCount == 2 {
			cancel() // Cancel context on second attempt
		}
		return errTemporary
	}

	err := Do(operation, WithContext(ctx), WithMaxAttempts(10), WithSleep(mockSleep()))

	if err == nil {
		t.Error("expected error, got nil")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got %v", err)
	}

	// Should be called at least twice (before cancellation)
	if callCount < 2 {
		t.Errorf("expected at least 2 calls, got %d", callCount)
	}
}

// TestDoWithContextTimeout tests context timeout
func TestDoWithContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	callCount := 0
	operation := func() error {
		callCount++
		return errTemporary
	}

	// Use real sleep to allow timeout to occur
	err := Do(operation,
		WithContext(ctx),
		WithMaxAttempts(10),
		WithBaseDelay(100*time.Millisecond),
	)

	if err == nil {
		t.Error("expected error, got nil")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

// TestDoWithContextAlreadyCancelled tests context cancelled before first attempt
func TestDoWithContextAlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	callCount := 0
	operation := func() error {
		callCount++
		return nil
	}

	err := Do(operation, WithContext(ctx), WithSleep(mockSleep()))

	if err == nil {
		t.Error("expected error, got nil")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}

	if callCount != 0 {
		t.Errorf("expected 0 calls, got %d", callCount)
	}
}

// TestDoWithRetryCondition tests custom retry condition
func TestDoWithRetryCondition(t *testing.T) {
	callCount := 0
	operation := func() error {
		callCount++
		if callCount == 2 {
			return errFatal // Should not retry
		}
		return errTemporary
	}

	err := Do(operation,
		WithMaxAttempts(10),
		WithSleep(mockSleep()),
		WithRetryCondition(func(err error) bool {
			return !errors.Is(err, errFatal)
		}),
	)

	if err == nil {
		t.Error("expected error, got nil")
	}

	if !errors.Is(err, errFatal) {
		t.Errorf("expected errFatal, got %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

// TestDoWithOnRetry tests retry callback
func TestDoWithOnRetry(t *testing.T) {
	callCount := 0
	retryCount := 0
	var retryAttempts []int
	var retryErrors []error

	operation := func() error {
		callCount++
		if callCount < 3 {
			return fmt.Errorf("attempt %d failed", callCount)
		}
		return nil
	}

	err := Do(operation,
		WithMaxAttempts(5),
		WithSleep(mockSleep()),
		WithOnRetry(func(attempt int, err error) {
			retryCount++
			retryAttempts = append(retryAttempts, attempt)
			retryErrors = append(retryErrors, err)
		}),
	)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if retryCount != 2 {
		t.Errorf("expected 2 retries, got %d", retryCount)
	}

	expectedAttempts := []int{1, 2}
	if len(retryAttempts) != len(expectedAttempts) {
		t.Errorf("expected %d retry attempts, got %d", len(expectedAttempts), len(retryAttempts))
	}

	for i, expected := range expectedAttempts {
		if retryAttempts[i] != expected {
			t.Errorf("retry %d: expected attempt %d, got %d", i, expected, retryAttempts[i])
		}
	}
}

// TestDoWithOnSuccess tests success callback
func TestDoWithOnSuccess(t *testing.T) {
	callCount := 0
	successCalled := false
	var successAttempt int

	operation := func() error {
		callCount++
		if callCount < 3 {
			return errTemporary
		}
		return nil
	}

	err := Do(operation,
		WithMaxAttempts(5),
		WithSleep(mockSleep()),
		WithOnSuccess(func(attempt int) {
			successCalled = true
			successAttempt = attempt
		}),
	)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if !successCalled {
		t.Error("expected success callback to be called")
	}

	if successAttempt != 3 {
		t.Errorf("expected success on attempt 3, got %d", successAttempt)
	}
}

// TestDoWithResult tests operation with return value
func TestDoWithResult(t *testing.T) {
	callCount := 0
	operation := func() (string, error) {
		callCount++
		if callCount < 3 {
			return "", errTemporary
		}
		return "success", nil
	}

	result, err := DoWithResult(operation, WithMaxAttempts(5), WithSleep(mockSleep()))

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result != "success" {
		t.Errorf("expected result='success', got '%s'", result)
	}

	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

// TestDoWithResultFailure tests operation with result that fails
func TestDoWithResultFailure(t *testing.T) {
	operation := func() (int, error) {
		return 0, errTemporary
	}

	result, err := DoWithResult(operation, WithMaxAttempts(3), WithSleep(mockSleep()))

	if err == nil {
		t.Error("expected error, got nil")
	}

	if result != 0 {
		t.Errorf("expected zero value result, got %d", result)
	}

	if !errors.Is(err, errTemporary) {
		t.Errorf("expected errTemporary, got %v", err)
	}
}

// TestRetrier tests reusable Retrier
func TestRetrier(t *testing.T) {
	retrier := NewRetrier(
		WithMaxAttempts(3),
		WithSleep(mockSleep()),
	)

	// Test first operation
	callCount1 := 0
	err1 := retrier.Do(func() error {
		callCount1++
		if callCount1 < 2 {
			return errTemporary
		}
		return nil
	})

	if err1 != nil {
		t.Errorf("operation 1: expected no error, got %v", err1)
	}

	if callCount1 != 2 {
		t.Errorf("operation 1: expected 2 calls, got %d", callCount1)
	}

	// Test second operation (should be independent)
	callCount2 := 0
	err2 := retrier.Do(func() error {
		callCount2++
		return nil
	})

	if err2 != nil {
		t.Errorf("operation 2: expected no error, got %v", err2)
	}

	if callCount2 != 1 {
		t.Errorf("operation 2: expected 1 call, got %d", callCount2)
	}
}

// TestResultRetrier tests reusable ResultRetrier
func TestResultRetrier(t *testing.T) {
	retrier := NewResultRetrier[int](WithMaxAttempts(3),
		WithSleep(mockSleep()),
	)

	// Test first operation
	callCount1 := 0
	result1, err1 := retrier.Do(func() (int, error) {
		callCount1++
		if callCount1 < 2 {
			return 0, errTemporary
		}
		return 42, nil
	})

	if err1 != nil {
		t.Errorf("operation 1: expected no error, got %v", err1)
	}

	if result1 != 42 {
		t.Errorf("operation 1: expected result=42, got %d", result1)
	}

	// Test second operation
	result2, err2 := retrier.Do(func() (int, error) {
		return 99, nil
	})

	if err2 != nil {
		t.Errorf("operation 2: expected no error, got %v", err2)
	}

	if result2 != 99 {
		t.Errorf("operation 2: expected result=99, got %d", result2)
	}
}

// TestWithBaseDelay tests base delay configuration
func TestWithBaseDelay(t *testing.T) {
	tests := []struct {
		name     string
		delay    time.Duration
		expected time.Duration
	}{
		{"positive delay", 500 * time.Millisecond, 500 * time.Millisecond},
		{"zero delay", 0, 0},
		{"negative delay", -100 * time.Millisecond, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := defaultStrategy()
			WithBaseDelay(tt.delay)(s)

			if s.delayConfig.BaseDelay != tt.expected {
				t.Errorf("expected BaseDelay=%v, got %v", tt.expected, s.delayConfig.BaseDelay)
			}
		})
	}
}

// TestWithMaxDelay tests max delay configuration
func TestWithMaxDelay(t *testing.T) {
	tests := []struct {
		name     string
		delay    time.Duration
		expected time.Duration
	}{
		{"positive delay", 30 * time.Second, 30 * time.Second},
		{"zero delay", 0, 0},
		{"negative delay", -5 * time.Second, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := defaultStrategy()
			WithMaxDelay(tt.delay)(s)

			if s.delayConfig.MaxDelay != tt.expected {
				t.Errorf("expected MaxDelay=%v, got %v", tt.expected, s.delayConfig.MaxDelay)
			}
		})
	}
}

// TestWithMaxJitter tests max jitter configuration
func TestWithMaxJitter(t *testing.T) {
	tests := []struct {
		name     string
		jitter   time.Duration
		expected time.Duration
	}{
		{"positive jitter", 200 * time.Millisecond, 200 * time.Millisecond},
		{"zero jitter", 0, 0},
		{"negative jitter", -50 * time.Millisecond, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := defaultStrategy()
			WithMaxJitter(tt.jitter)(s)

			if s.delayConfig.MaxJitter != tt.expected {
				t.Errorf("expected MaxJitter=%v, got %v", tt.expected, s.delayConfig.MaxJitter)
			}
		})
	}
}

// TestWithBackoffStep tests backoff step configuration
func TestWithBackoffStep(t *testing.T) {
	tests := []struct {
		name     string
		step     int
		expected int
	}{
		{"positive step", 10, 10},
		{"zero step", 0, 0},
		{"negative step", -5, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := defaultStrategy()
			WithBackoffStep(tt.step)(s)

			if s.delayConfig.MaxBackoffStep != tt.expected {
				t.Errorf("expected MaxBackoffStep=%d, got %d", tt.expected, s.delayConfig.MaxBackoffStep)
			}
		})
	}
}

// TestWithDelayConfig tests complete delay configuration
func TestWithDelayConfig(t *testing.T) {
	config := DelayConfig{
		BaseDelay:      200 * time.Millisecond,
		MaxDelay:       10 * time.Second,
		MaxJitter:      50 * time.Millisecond,
		MaxBackoffStep: 5,
	}

	s := defaultStrategy()
	WithDelayConfig(config)(s)

	if s.delayConfig.BaseDelay != config.BaseDelay {
		t.Errorf("BaseDelay: expected %v, got %v", config.BaseDelay, s.delayConfig.BaseDelay)
	}

	if s.delayConfig.MaxDelay != config.MaxDelay {
		t.Errorf("MaxDelay: expected %v, got %v", config.MaxDelay, s.delayConfig.MaxDelay)
	}

	if s.delayConfig.MaxJitter != config.MaxJitter {
		t.Errorf("MaxJitter: expected %v, got %v", config.MaxJitter, s.delayConfig.MaxJitter)
	}

	if s.delayConfig.MaxBackoffStep != config.MaxBackoffStep {
		t.Errorf("MaxBackoffStep: expected %d, got %d", config.MaxBackoffStep, s.delayConfig.MaxBackoffStep)
	}
}

// TestWithDelayConfigNegativeValues tests delay config with negative values
func TestWithDelayConfigNegativeValues(t *testing.T) {
	config := DelayConfig{
		BaseDelay:      -100 * time.Millisecond,
		MaxDelay:       -5 * time.Second,
		MaxJitter:      -50 * time.Millisecond,
		MaxBackoffStep: -10,
	}

	s := defaultStrategy()
	WithDelayConfig(config)(s)

	if s.delayConfig.BaseDelay != 0 {
		t.Errorf("BaseDelay: expected 0, got %v", s.delayConfig.BaseDelay)
	}

	if s.delayConfig.MaxDelay != 0 {
		t.Errorf("MaxDelay: expected 0, got %v", s.delayConfig.MaxDelay)
	}

	if s.delayConfig.MaxJitter != 0 {
		t.Errorf("MaxJitter: expected 0, got %v", s.delayConfig.MaxJitter)
	}

	if s.delayConfig.MaxBackoffStep != 0 {
		t.Errorf("MaxBackoffStep: expected 0, got %d", s.delayConfig.MaxBackoffStep)
	}
}
