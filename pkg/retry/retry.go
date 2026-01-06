package retry

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"time"
)

// Operation defines a retryable operation without return value.
type Operation func() error

// OperationWithResult defines a retryable operation with a return value of type T.
type OperationWithResult[T any] func() (T, error)

// delayConfig holds delay-related configuration for retry strategies.
type delayConfig struct {
	// Base delay duration between retries.
	baseDelay time.Duration

	// Maximum delay duration (0 means unlimited).
	maxDelay time.Duration

	// Maximum random jitter to add to delays.
	maxJitter time.Duration

	// Maximum backoff step to prevent overflow (computed internally).
	maxBackoffStep int
}

// Strategy encapsulates the retry strategy configuration.
// All fields are private and can only be set through Option functions.
// A Strategy can be safely reused across multiple retry operations.
//
// Example:
//
//	strategy := NewRetrier(
//		WithMaxAttempts(5),
//		WithExponentialBackoff(),
//	)
//
//	// Reuse strategy multiple times
//	err1 := strategy.Do(operation1)
//	err2 := strategy.Do(operation2)
type Strategy struct {
	// Context for cancellation control.
	context context.Context

	// Maximum number of attempts (0 means unlimited retries).
	maxAttempts int

	// Callback function invoked before each retry (attempt starts from 1).
	// This is called after an operation fails but before the delay.
	onRetry func(attempt int, err error)

	// Callback function invoked after a successful operation (attempt starts from 1).
	// This is useful for logging successful retries.
	onSuccess func(attempt int)

	// Function to determine if an error should trigger a retry.
	shouldRetry func(err error) bool

	// Function to compute delay before next retry (attempt starts from 1).
	computeDelay func(attempt int, err error, config delayConfig) time.Duration

	// Sleep function (can be replaced for testing).
	sleep func(duration time.Duration) <-chan time.Time

	// Embedded delay configuration.
	delayConfig
}

// DefaultStrategy returns a Strategy with sensible default values.
// Default configuration:
//   - Context: background context
//   - MaxAttempts: 3
//   - BaseDelay: 100ms
//   - MaxDelay: unlimited (0)
//   - MaxJitter: 100ms
//   - ComputeDelay: ExponentialBackoff with RandomJitter
//   - ShouldRetry: always retry
//   - OnRetry: no-op
//   - OnSuccess: no-op
//   - Sleep: time.After
func DefaultStrategy() *Strategy {
	return &Strategy{
		context:      context.Background(),
		maxAttempts:  3,
		onRetry:      func(attempt int, err error) {},
		onSuccess:    func(attempt int) {},
		shouldRetry:  func(err error) bool { return true },
		computeDelay: CombineDelays(ExponentialBackoff, RandomJitter),
		sleep:        time.After,
		delayConfig: delayConfig{
			baseDelay:      100 * time.Millisecond,
			maxDelay:       0,
			maxJitter:      100 * time.Millisecond,
			maxBackoffStep: 0,
		},
	}
}

// Option is a function that configures a Strategy.
type Option func(*Strategy)

// WithContext sets the context for retry operations.
// The context can be used to cancel retries or set timeouts.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//	err := Do(operation, WithContext(ctx))
//
// Default: context.Background()
func WithContext(ctx context.Context) Option {
	return func(s *Strategy) {
		if ctx != nil {
			s.context = ctx
		}
	}
}

// WithMaxAttempts sets the maximum number of retry attempts.
// Set to 0 for unlimited retries (until success or context cancellation).
// Negative values are treated as 0.
//
// Example:
//
//	err := Do(operation, WithMaxAttempts(5))  // Try up to 5 times
//
// Default: 3
func WithMaxAttempts(maxAttempts int) Option {
	return func(s *Strategy) {
		if maxAttempts < 0 {
			maxAttempts = 0
		}
		s.maxAttempts = maxAttempts
	}
}

// WithUnlimitedAttempts configures unlimited retries until success or context cancellation.
// This is equivalent to WithMaxAttempts(0).
// Use with caution - always pair with a context timeout to prevent infinite loops.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
//	defer cancel()
//	err := Do(operation, WithUnlimitedAttempts(), WithContext(ctx))
func WithUnlimitedAttempts() Option {
	return WithMaxAttempts(0)
}

// WithOnRetry sets a callback function invoked before each retry.
// The callback receives the attempt number (starting from 1) and the error that triggered the retry.
// This is called after an operation fails but before sleeping for the next attempt.
//
// Example:
//
//	err := Do(operation,
//		WithOnRetry(func(attempt int, err error) {
//			log.Printf("Retry %d: %v", attempt, err)
//		}),
//	)
//
// Default: no-op function
func WithOnRetry(onRetry func(attempt int, err error)) Option {
	return func(s *Strategy) {
		if onRetry != nil {
			s.onRetry = onRetry
		}
	}
}

// WithOnSuccess sets a callback function invoked after a successful operation.
// The callback receives the attempt number (starting from 1).
// This is useful for logging when an operation succeeds after retries.
//
// Example:
//
//	err := Do(operation,
//		WithOnSuccess(func(attempt int) {
//			if attempt > 1 {
//				log.Printf("Succeeded after %d attempts", attempt)
//			}
//		}),
//	)
//
// Default: no-op function
func WithOnSuccess(onSuccess func(attempt int)) Option {
	return func(s *Strategy) {
		if onSuccess != nil {
			s.onSuccess = onSuccess
		}
	}
}

// WithRetryCondition sets a function to determine if an error should trigger a retry.
// Return true to retry, false to abort immediately.
//
// Example - Don't retry on specific errors:
//
//	err := Do(operation,
//		WithRetryCondition(func(err error) bool {
//			return !errors.Is(err, ErrFatalError)
//		}),
//	)
//
// Example - Only retry on temporary errors:
//
//	err := Do(operation,
//		WithRetryCondition(func(err error) bool {
//			var tempErr interface{ Temporary() bool }
//			return errors.As(err, &tempErr) && tempErr.Temporary()
//		}),
//	)
//
// Default: always returns true (retry all errors)
func WithRetryCondition(shouldRetry func(err error) bool) Option {
	return func(s *Strategy) {
		if shouldRetry != nil {
			s.shouldRetry = shouldRetry
		}
	}
}

// WithDelayFunc sets a custom delay computation function.
// The function receives the attempt number (starting from 1), the error, and delay configuration.
//
// Example - Custom delay based on error type:
//
//	err := Do(operation,
//		WithDelayFunc(func(attempt int, err error, config delayConfig) time.Duration {
//			if errors.Is(err, ErrRateLimited) {
//				return 5 * time.Second  // Longer delay for rate limits
//			}
//			return config.baseDelay * time.Duration(attempt)
//		}),
//	)
//
// Default: CombineDelays(ExponentialBackoff, RandomJitter)
func WithDelayFunc(computeDelay func(attempt int, err error, config delayConfig) time.Duration) Option {
	return func(s *Strategy) {
		if computeDelay != nil {
			s.computeDelay = computeDelay
		}
	}
}

// WithSleep sets a custom sleep function for testing purposes.
// The function should return a channel that delivers the current time after the specified duration.
//
// Example - Mock sleep for testing:
//
//	mockSleep := func(d time.Duration) <-chan time.Time {
//		ch := make(chan time.Time, 1)
//		ch <- time.Now()  // Return immediately
//		return ch
//	}
//	err := Do(operation, WithSleep(mockSleep))
//
// Default: time.After
func WithSleep(sleep func(duration time.Duration) <-chan time.Time) Option {
	return func(s *Strategy) {
		if sleep != nil {
			s.sleep = sleep
		}
	}
}

// WithBaseDelay sets the base delay duration between retries.
// This is the starting delay for exponential backoff strategies.
// Negative values are treated as 0.
//
// Example:
//
//	err := Do(operation, WithBaseDelay(200*time.Millisecond))
//
// Default: 100ms
func WithBaseDelay(delay time.Duration) Option {
	return func(s *Strategy) {
		if delay < 0 {
			delay = 0
		}
		s.baseDelay = delay
	}
}

// WithMaxDelay sets the maximum delay duration between retries.
// Set to 0 for unlimited delay (no cap on backoff).
// Negative values are treated as 0.
//
// Example:
//
//	err := Do(operation,
//		WithExponentialBackoff(),
//		WithMaxDelay(30*time.Second),  // Cap exponential growth
//	)
//
// Default: 0 (unlimited)
func WithMaxDelay(maxDelay time.Duration) Option {
	return func(s *Strategy) {
		if maxDelay < 0 {
			maxDelay = 0
		}
		s.maxDelay = maxDelay
	}
}

// WithMaxJitter sets the maximum random jitter to add to delays.
// Jitter helps prevent thundering herd problems in distributed systems.
// Negative values are treated as 0.
//
// Example:
//
//	err := Do(operation, WithMaxJitter(500*time.Millisecond))
//
// Default: 100ms
func WithMaxJitter(maxJitter time.Duration) Option {
	return func(s *Strategy) {
		if maxJitter < 0 {
			maxJitter = 0
		}
		s.maxJitter = maxJitter
	}
}

// WithBackoffStep sets the maximum backoff step for exponential backoff.
// This limits the exponential growth to prevent overflow (delay = baseDelay * 2^step).
// Set to 0 to use the automatically calculated safe maximum.
// Negative values are treated as 0.
//
// Example:
//
//	err := Do(operation,
//		WithExponentialBackoff(),
//		WithBackoffStep(10),  // Limit to 2^10 = 1024x multiplier
//	)
//
// Default: 0 (auto-calculated based on baseDelay to prevent overflow)
func WithBackoffStep(step int) Option {
	return func(s *Strategy) {
		if step < 0 {
			step = 0
		}
		s.maxBackoffStep = step
	}
}

// Predefined delay strategies

// WithExponentialBackoff configures exponential backoff with random jitter.
// This is the default delay strategy and provides good balance between
// retry frequency and avoiding overwhelming the target system.
//
// Formula: (baseDelay * 2^attempt) + random(0, maxJitter)
//
// Example:
//
//	err := Do(operation,
//		WithExponentialBackoff(),
//		WithBaseDelay(100*time.Millisecond),
//		WithMaxJitter(50*time.Millisecond),
//	)
func WithExponentialBackoff() Option {
	return WithDelayFunc(CombineDelays(ExponentialBackoff, RandomJitter))
}

// WithFixedDelay configures a fixed delay between retries.
// The delay is always equal to baseDelay, regardless of attempt number.
//
// Example:
//
//	err := Do(operation,
//		WithFixedDelay(),
//		WithBaseDelay(1*time.Second),
//	)
func WithFixedDelay() Option {
	return WithDelayFunc(FixedDelay)
}

// WithFullJitter configures full jitter backoff strategy.
// Returns a random duration between 0 and (baseDelay * 2^attempt).
// This strategy provides maximum randomization to minimize collision probability
// in distributed systems.
//
// Formula: random(0, min(maxDelay, baseDelay * 2^attempt))
//
// Recommended by AWS for distributed systems:
// https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/
//
// Example:
//
//	err := Do(operation,
//		WithFullJitter(),
//		WithBaseDelay(100*time.Millisecond),
//		WithMaxDelay(30*time.Second),
//	)
func WithFullJitter() Option {
	return WithDelayFunc(FullJitterBackoff)
}

// Delay computation functions

// ExponentialBackoff implements an exponential backoff delay strategy.
// The delay is calculated as: baseDelay * 2^attempt (capped at maxBackoffStep).
// This strategy provides increasingly longer delays between retries.
//
// Formula: baseDelay * 2^min(attempt, maxBackoffStep)
//
// This function is typically used with CombineDelays to add jitter:
//
//	CombineDelays(ExponentialBackoff, RandomJitter)
func ExponentialBackoff(attempt int, _ error, config delayConfig) time.Duration {
	if attempt <= 0 || config.baseDelay <= 0 {
		return config.baseDelay
	}

	step := attempt
	if step > config.maxBackoffStep {
		step = config.maxBackoffStep
	}

	// Use bit shift for efficient exponential calculation
	delay := config.baseDelay << step

	// Check for overflow (negative value after shift indicates overflow)
	if delay < 0 {
		return time.Duration(math.MaxInt64)
	}

	return delay
}

// FixedDelay implements a fixed delay strategy.
// The delay is always equal to baseDelay, regardless of the attempt number.
//
// This is useful when you want consistent retry intervals, such as
// polling a job status or waiting for a resource to become available.
func FixedDelay(_ int, _ error, config delayConfig) time.Duration {
	return config.baseDelay
}

// RandomJitter implements a random jitter delay strategy.
// Returns a random duration between 0 and maxJitter.
// This helps prevent synchronized retries in distributed systems (thundering herd).
//
// This function is typically combined with other delay strategies:
//
//	CombineDelays(FixedDelay, RandomJitter)       // Fixed + jitter
//	CombineDelays(ExponentialBackoff, RandomJitter) // Exponential + jitter
func RandomJitter(_ int, _ error, config delayConfig) time.Duration {
	if config.maxJitter <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(config.maxJitter)))
}

// FullJitterBackoff implements a full jitter backoff delay strategy.
// Returns a random duration between 0 and (baseDelay * 2^attempt).
// This combines exponential backoff with full randomization to minimize collision probability.
//
// Formula: random(0, min(maxDelay, baseDelay * 2^attempt))
//
// This strategy is recommended by AWS for distributed systems as it provides
// better distribution of retry timing compared to exponential backoff with fixed jitter.
//
// Reference: https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/
func FullJitterBackoff(attempt int, _ error, config delayConfig) time.Duration {
	if attempt <= 0 {
		return 0
	}

	// Calculate backoff ceiling: baseDelay * 2^attempt
	ceiling := config.baseDelay
	for i := 0; i < attempt; i++ {
		// Check max delay cap
		if config.maxDelay > 0 && ceiling >= config.maxDelay {
			ceiling = config.maxDelay
			break
		}
		// Check for overflow
		if ceiling > time.Duration(math.MaxInt64/2) {
			ceiling = time.Duration(math.MaxInt64)
			break
		}
		ceiling *= 2
	}

	// Apply max delay cap
	if config.maxDelay > 0 && ceiling > config.maxDelay {
		ceiling = config.maxDelay
	}

	if ceiling <= 0 {
		return 0
	}

	// Return random value between 0 and ceiling
	return time.Duration(rand.Int64N(int64(ceiling)))
}

// CombineDelays combines multiple delay strategies into a single strategy.
// The resulting delay is the sum of all individual strategy delays.
// This allows composing simple strategies into complex ones.
//
// Example - Exponential backoff with jitter:
//
//	CombineDelays(ExponentialBackoff, RandomJitter)
//
// Example - Fixed delay with jitter:
//
//	CombineDelays(FixedDelay, RandomJitter)
//
// Example - Multiple custom delays:
//
//	CombineDelays(
//		ExponentialBackoff,
//		RandomJitter,
//		func(attempt int, err error, config delayConfig) time.Duration {
//			// Add extra delay for specific errors
//			if errors.Is(err, ErrRateLimited) {
//				return 5 * time.Second
//			}
//			return 0
//		},
//	)
func CombineDelays(delayFuncs ...func(int, error, delayConfig) time.Duration) func(int, error, delayConfig) time.Duration {
	return func(attempt int, err error, config delayConfig) time.Duration {
		var total int64
		for _, delayFunc := range delayFuncs {
			delay := int64(delayFunc(attempt, err, config))

			// Prevent overflow
			if total > math.MaxInt64-delay {
				return time.Duration(math.MaxInt64)
			}
			total += delay
		}
		return time.Duration(total)
	}
}

// calculateMaxBackoffStep computes the maximum safe backoff step to prevent overflow.
// The calculation ensures that baseDelay * 2^step will not overflow time.Duration (int64).
//
// Returns the maximum step value that can be safely used with the given baseDelay.
//
// Example:
//   - baseDelay = 1ns:  maxStep = 62 (2^62 ≈ 146 years)
//   - baseDelay = 100ms: maxStep = 36 (100ms * 2^36 ≈ 7.5 years)
//   - baseDelay = 1s:   maxStep = 26 (1s * 2^26 ≈ 2.1 years)
func calculateMaxBackoffStep(baseDelay time.Duration) int {
	const maxBackoffStep = 62 // 2^62 nanoseconds ≈ 146 years

	if baseDelay <= 0 {
		return maxBackoffStep
	}

	// Calculate the maximum shift that won't overflow
	// baseDelay * 2^maxStep < MaxInt64
	// => maxStep < log2(MaxInt64 / baseDelay)
	maxStep := maxBackoffStep - int(math.Floor(math.Log2(float64(baseDelay))))
	if maxStep < 0 {
		return 0
	}
	return maxStep
}

// doRetry implements the core retry logic.
// It repeatedly executes the operation until success or a termination condition is met:
//   - The operation succeeds (returns nil error)
//   - The maximum number of attempts is reached
//   - The retry condition returns false (shouldRetry)
//   - The context is cancelled
//
// Returns the operation result and nil on success, or zero value and error on failure.
// The error is wrapped with context information (attempt number, reason for failure).
func doRetry[T any](operation OperationWithResult[T], strategy *Strategy) (T, error) {
	var (
		result  T
		attempt int
	)

	// Check if context is already cancelled before first attempt
	if err := context.Cause(strategy.context); err != nil {
		return result, fmt.Errorf("context cancelled before first attempt: %w", err)
	}

	for {
		// Execute operation (attempt number starts from 1)
		attempt++
		res, err := operation()

		// Operation succeeded
		if err == nil {
			strategy.onSuccess(attempt)
			return res, nil
		}

		// Check if we should retry this error
		if !strategy.shouldRetry(err) {
			return result, fmt.Errorf("operation failed after %d attempts (aborted by retry condition): %w",
				attempt, err)
		}

		// Check if max attempts reached
		if strategy.maxAttempts > 0 && attempt >= strategy.maxAttempts {
			return result, fmt.Errorf("operation failed after %d attempts (max attempts reached): %w",
				attempt, err)
		}

		// Invoke retry callback (before sleeping)
		strategy.onRetry(attempt, err)

		// Compute delay for next attempt
		delay := strategy.computeDelay(attempt, err, strategy.delayConfig)

		// Apply max delay cap if configured
		if strategy.maxDelay > 0 {
			delay = min(delay, strategy.maxDelay)
		}

		// Wait for delay or context cancellation
		select {
		case <-strategy.sleep(delay):
			// Delay completed, continue to next attempt
		case <-strategy.context.Done():
			// Context cancelled during sleep
			ctxErr := context.Cause(strategy.context)
			if strategy.maxAttempts == 0 {
				// Unlimited retry mode
				return result, fmt.Errorf("operation cancelled after %d attempts (unlimited retry mode): %w (last error: %v)",
					attempt, ctxErr, err)
			}
			return result, fmt.Errorf("operation cancelled after %d attempts: %w (last error: %v)",
				attempt, ctxErr, err)
		}
	}
}

// ResultRetrier provides retry functionality for operations that return a value.
// It can be reused across multiple retry operations with the same configuration.
//
// Example:
//
//	retrier := NewResultRetrier[[]byte](
//		WithMaxAttempts(5),
//		WithExponentialBackoff(),
//	)
//
//	// Reuse retrier for multiple operations
//	data1, err1 := retrier.Do(func() ([]byte, error) { return fetchData1() })
//	data2, err2 := retrier.Do(func() ([]byte, error) { return fetchData2() })
type ResultRetrier[T any] struct {
	strategy *Strategy
}

// NewResultRetrier creates a new ResultRetrier with the given options.
// The retrier can be safely reused across multiple retry operations.
//
// Type parameter T specifies the return type of the operations.
//
// Example:
//
//	retrier := NewResultRetrier[*http.Response](//WithMaxAttempts(3),
//		WithExponentialBackoff(),
//	)
func NewResultRetrier[T any](opts ...Option) *ResultRetrier[T] {
	strategy := DefaultStrategy()

	// Apply configuration options
	for _, opt := range opts {
		opt(strategy)
	}

	// Calculate maximum backoff step to prevent overflow
	if strategy.maxBackoffStep > 0 {
		strategy.maxBackoffStep = min(strategy.maxBackoffStep, calculateMaxBackoffStep(strategy.baseDelay))
	} else {
		strategy.maxBackoffStep = calculateMaxBackoffStep(strategy.baseDelay)
	}

	return &ResultRetrier[T]{
		strategy: strategy,
	}
}

// Do executes the operation with retry logic according to the retrier's configuration.
//
// Example:
//
//	retrier := NewResultRetrier[string](WithMaxAttempts(3))
//	result, err := retrier.Do(func() (string, error) {
//		return fetchData()
//	})
func (r *ResultRetrier[T]) Do(operation OperationWithResult[T]) (T, error) {
	return doRetry(operation, r.strategy)
}

// Retrier provides retry functionality for operations that don't return a value.
// It can be reused across multiple retry operations with the same configuration.
//
// Example:
//
//	retrier := NewRetrier(
//		WithMaxAttempts(5),
//		WithExponentialBackoff(),
//		WithOnRetry(func(attempt int, err error) {
//			log.Printf("Retry %d: %v", attempt, err)
//		}),
//	)
//
//	// Reuse retrier for multiple operations
//	err1 := retrier.Do(func() error { return operation1() })
//	err2 := retrier.Do(func() error { return operation2() })
type Retrier struct {
	resultRetrier *ResultRetrier[any]
}

// NewRetrier creates a new Retrier with the given options.
// The retrier can be safely reused across multiple retry operations.
//
// Example:
//
//	retrier := NewRetrier(
//		WithMaxAttempts(5),
//		WithBaseDelay(200*time.Millisecond),
//		WithExponentialBackoff(),
//	)
func NewRetrier(opts ...Option) *Retrier {
	return &Retrier{
		resultRetrier: NewResultRetrier[any](opts...),
	}
}

// Do executes the operation with retry logic according to the retrier's configuration.
//
// Example:
//
//	retrier := NewRetrier(WithMaxAttempts(3))
//	err := retrier.Do(func() error {
//		return performOperation()
//	})
func (r *Retrier) Do(operation Operation) error {
	_, err := r.resultRetrier.Do(func() (any, error) {
		return nil, operation()
	})
	return err
}

// Package-level convenience functions

// Do executes an operation with retry logic using the provided options.
// This is a convenience function for one-time retry operations.
// For repeated operations with the same configuration, consider using NewRetrier.
//
// The operation is retried according to the configured strategy until:
//   - The operation succeeds (returns nil error)
//   - The maximum number of attempts is reached
//   - The retry condition returns false
//   - The context is cancelled
//
// Example - Simple retry:
//
//	err := retry.Do(
//		func() error {
//			return performOperation()
//		},
//		retry.WithMaxAttempts(3),
//	)
//
// Example - With timeout:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//
//	err := retry.Do(
//		func() error {
//			return performOperation()
//		},
//		retry.WithContext(ctx),
//		retry.WithMaxAttempts(5),
//	)
//
// Example - With custom retry condition:
//
//	err := retry.Do(
//		func() error {
//			return performOperation()
//		},
//		retry.WithRetryCondition(func(err error) bool {
//			return !errors.Is(err, ErrFatalError)
//		}),
//	)
//
// Returns nil on success, or the last error if all retries are exhausted.
func Do(operation Operation, opts ...Option) error {
	return NewRetrier(opts...).Do(operation)
}

// DoWithResult executes an operation with retry logic and returns the result.
// This is a convenience function for one-time retry operations that return a value.
// For repeated operations with the same configuration, consider using NewResultRetrier.
//
// The operation is retried according to the configured strategy until:
//   - The operation succeeds (returns a value and nil error)
//   - The maximum number of attempts is reached
//   - The retry condition returns false
//   - The context is cancelled
//
// Type parameter T specifies the return type of the operation.
//
// Example - HTTP request with retry:
//
//	data, err := retry.DoWithResult(
//		func() ([]byte, error) {
//			resp, err := http.Get("https://api.example.com/data")
//			if err != nil {
//				return nil, err
//			}
//			defer resp.Body.Close()
//			return io.ReadAll(resp.Body)
//		},
//		retry.WithMaxAttempts(3),
//		retry.WithExponentialBackoff(),
//	)
//
// Example - Database query with retry:
//
//	user, err := retry.DoWithResult(
//		func() (*User, error) {
//			return db.GetUser(userID)
//		},
//		retry.WithMaxAttempts(5),
//		retry.WithRetryCondition(func(err error) bool {
//			// Retry on temporary database errors
//			return errors.Is(err, sql.ErrConnDone)
//		}),
//	)
//
// Returns the operation result and nil on success, or zero value and error on failure.
func DoWithResult[T any](operation OperationWithResult[T], opts ...Option) (T, error) {
	return NewResultRetrier[T](opts...).Do(operation)
}
