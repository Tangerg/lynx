// Package retry executes operations with configurable retry policies.
//
// Use [Do] for a one-shot retry, [DoWithResult] when the operation
// returns a value, or build a reusable [Retrier] / [ResultRetrier]
// with [NewRetrier] / [NewResultRetrier]. Behavior is configured with
// [Option] values: max attempts, delay strategy, retry predicate,
// context, and callbacks.
//
// Defaults: 3 attempts, exponential backoff with jitter, base delay
// 100ms, no maximum delay, retry on every error.
//
// Example — basic retry:
//
//	err := retry.Do(func() error {
//	    return callRemote()
//	}, retry.WithMaxAttempts(5), retry.WithExponentialBackoff())
//
// Example — bounded retry with context timeout and result:
//
//	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
//	defer cancel()
//
//	body, err := retry.DoWithResult(func() ([]byte, error) {
//	    return fetch(ctx)
//	}, retry.WithContext(ctx), retry.WithMaxAttempts(0))
//
// For high-frequency call sites, build a [Retrier] once and reuse it.
package retry
