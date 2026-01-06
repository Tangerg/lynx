// Package retry provides a simple and flexible retry mechanism for Go operations.
//
// This package allows you to retry operations that may fail temporarily, with
// configurable strategies for delay, backoff, and termination conditions.
// It supports both operations with and without return values, context-based
// cancellation, and custom retry logic.
//
// # Design Philosophy
//
// This package is inspired by https://github.com/avast/retry-go but redesigned
// with a focus on simplicity and practical usage. Key differences include:
//
//   - Simplified error handling: returns only the last error instead of aggregating all errors
//   - Cleaner API: method-based retry operations with reusable Retrier instances
//   - Removed over-engineered features: no error aggregation, no per-error attempt limits
//   - Enhanced usability: predefined delay strategy functions and better documentation
//
// # Basic Usage
//
// Simple retry with default configuration (3 attempts, exponential backoff):
//
//	err := retry.Do(func() error {
//		return performOperation()
//	})
//
// Retry with custom configuration:
//
//	err := retry.Do(
//		func() error {
//			return performOperation()
//		},
//		retry.WithMaxAttempts(5),
//		retry.WithExponentialBackoff(),
//	)
//
// Retry operation with return value:
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
//	)
//
// # Reusable Retrier
//
// For high-frequency operations, create a retrier once and reuse it:
//
//	retrier := retry.NewRetrier(
//		retry.WithMaxAttempts(5),
//		retry.WithExponentialBackoff(),
//	)
//
//	// Reuse retrier multiple times
//	for _, task := range tasks {
//		err := retrier.Do(func() error {
//			return processTask(task)
//		})
//		if err != nil {
//			log.Printf("Task failed: %v", err)
//		}
//	}
//
// # Context and Timeout
//
// Use context for timeout and cancellation control:
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
// # Delay Strategies
//
// The package provides several built-in delay strategies:
//
// Exponential Backoff with Jitter (default):
//
//	retry.WithExponentialBackoff()
//	// Delay = (baseDelay * 2^attempt) + random(0, maxJitter)
//
// Fixed Delay:
//
//	retry.WithFixedDelay()
//	// Delay = baseDelay (constant)
//
// Full Jitter Backoff (recommended by AWS for distributed systems):
//
//	retry.WithFullJitter()
//	// Delay = random(0, min(maxDelay, baseDelay * 2^attempt))
//
// Custom delay strategy:
//
//	retry.WithDelayFunc(func(attempt int, err error, config retry.delayConfig) time.Duration {
//		if errors.Is(err, ErrRateLimited) {
//			return 5 * time.Second
//		}
//		return config.baseDelay * time.Duration(attempt)
//	})
//
// # Conditional Retry
//
// Control which errors should trigger retries:
//
//	err := retry.Do(
//		func() error {
//			return performOperation()
//		},
//		retry.WithRetryCondition(func(err error) bool {
//			// Don't retry on fatal errors
//			return !errors.Is(err, ErrFatalError)
//		}),
//	)
//
// Only retry temporary errors:
//
//	retry.WithRetryCondition(func(err error) bool {
//		var tempErr interface{ Temporary() bool }
//		return errors.As(err, &tempErr) && tempErr.Temporary()
//	})
//
// # Monitoring and Callbacks
//
// Monitor retry attempts:
//
//	err := retry.Do(
//		func() error {
//			return performOperation()
//		},
//		retry.WithOnRetry(func(attempt int, err error) {
//			log.Printf("Attempt %d failed: %v", attempt, err)
//		}),
//		retry.WithOnSuccess(func(attempt int) {
//			if attempt > 1 {
//				log.Printf("Succeeded after %d attempts", attempt)
//			}
//		}),
//	)
//
// # Configuration Options
//
// Attempt Control:
//   - WithMaxAttempts(n): Set maximum retry attempts (0 for unlimited)
//   - WithUnlimitedAttempts(): Retry until success or context cancellation
//
// Delay Configuration:
//   - WithBaseDelay(d): Set base delay between retries
//   - WithMaxDelay(d): Cap maximum delay
//   - WithMaxJitter(d): Set maximum random jitter
//
// Delay Strategies:
//   - WithExponentialBackoff(): Exponential backoff with jitter (default)
//   - WithFixedDelay(): Fixed delay between retries
//   - WithFullJitter(): Full jitter backoff (AWS recommended)
//   - WithDelayFunc(f): Custom delay calculation function
//
// Retry Control:
//   - WithContext(ctx): Set context for cancellation/timeout
//   - WithRetryCondition(f): Custom retry condition function
//
// Callbacks:
//   - WithOnRetry(f): Called before each retry attempt
//   - WithOnSuccess(f): Called after successful operation
//
// Testing:
//   - WithSleep(f): Replace sleep function for testing
//
// # Performance Considerations
//
// For high-frequency operations, create a retrier once and reuse it to avoid
// repeated allocations:
//
//	// Good: Create once, reuse many times
//	retrier := retry.NewRetrier(opts...)
//	for i := 0; i < 10000; i++ {
//		retrier.Do(operation)
//	}
//
//	// Bad: Create new retrier each time
//	for i := 0; i < 10000; i++ {
//		retry.Do(operation, opts...)
//	}
//
// # Testing
//
// Mock sleep for fast tests:
//
//	func TestRetry(t *testing.T) {
//		mockSleep := func(d time.Duration) <-chan time.Time {
//			ch := make(chan time.Time, 1)
//			ch <- time.Now()  // Return immediately
//			return ch
//		}
//
//		err := retry.Do(
//			func() error {
//				// Test operation
//			},
//			retry.WithSleep(mockSleep),
//		)
//	}
//
// # Error Handling
//
// The package wraps errors with context information:
//
//	operation failed after 3 attempts (max attempts reached): attempt 3 failed: connection refused
//	operation cancelled after 2 attempts: context deadline exceeded (last error: timeout)
//
// Use errors.Is and errors.As to check wrapped errors:
//
//	err := retry.Do(operation, opts...)
//	if errors.Is(err, ErrConnectionRefused) {
//		// Handle connection error
//	}
//
// # Best Practices
//
// 1. Always use context with timeout for unlimited retries:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
//	defer cancel()
//	retry.Do(operation, retry.WithContext(ctx), retry.WithUnlimitedAttempts())
//
// 2. Set appropriate max delay to prevent excessive waiting:
//
//	retry.Do(operation,
//		retry.WithExponentialBackoff(),
//		retry.WithMaxDelay(30*time.Second),
//	)
//
// 3. Use retry condition to fail fast on permanent errors:
//
//	retry.WithRetryCondition(func(err error) bool {
//		return !errors.Is(err, ErrPermanentFailure)
//	})
//
// 4. Reuse retrier instances in hot paths for better performance.
//
// # Comparison with avast/retry-go
//
//	| Feature                  | avast/retry-go | This Package    |
//	|--------------------------|----------------|-----------------|
//	| API Style                | Function-based | Method-based    |
//	| Error Aggregation        | All errors     | Last error only |
//	| Reusable Retrier         | Yes            | Yes             |
//	| Per-error Attempt Limits | Yes            | No              |
//	| Unrecoverable Error Type | Yes            | No              |
//	| Predefined Strategies    | No             | Yes             |
//	| OnSuccess Callback       | No             | Yes             |
//
// Migration example:
//
//	// avast/retry-go v5.x
//	retry.New(retry.Attempts(3)).Do(func() error { ... })
//
//	// This package
//	retry.NewRetrier(retry.WithMaxAttempts(3)).Do(func() error { ... })
//
// # References
//
//   - Inspired by: https://github.com/avast/retry-go
//   - AWS Exponential Backoff: https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/
package retry
