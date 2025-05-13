package safe

import (
	"fmt"
	"runtime/debug"
	"sync/atomic"
	"time"
)

// PanicError represents a recovered panic with additional metadata.
// It stores the time when panic occurred, the original panic information,
// stack trace, and a cached error message for performance.
type PanicError struct {
	time  time.Time              // Timestamp when the panic occurred
	info  any                    // The value passed to panic()
	stack []byte                 // Stack trace at the time of panic
	cache atomic.Pointer[string] // Cached error message to avoid regeneration
}

// Error implements the error interface for PanicError.
// It returns a formatted error message including timestamp, error information,
// and stack trace, using caching for better performance.
func (e *PanicError) Error() string {
	if e.cache.Load() == nil {
		timestamp := e.time.Format(time.RFC3339Nano)
		stackTrace := string(e.stack)
		err := fmt.Sprintf("panic: \ntimestamp: %s, \nerror: %+v, \nstack: %s", timestamp, e.info, stackTrace)
		e.cache.Store(&err)
	}
	cache := e.cache.Load()
	return *cache
}

// NewPanicError creates a new PanicError instance with the given panic information and stack trace.
// This is typically used internally by recovery functions.
func NewPanicError(info any, stack []byte) error {
	return &PanicError{
		time:  time.Now(),
		info:  info,
		stack: stack,
	}
}

// Go launches a goroutine with built-in panic recovery.
// This function provides a safer way to start goroutines by automatically recovering from panics
// and invoking provided error handlers. It captures panic information including timestamp and stack trace
// for better debugging.
//
// Parameters:
//   - fn: The function to execute in the goroutine. This is the main function that will run concurrently.
//   - panicFns: Optional variadic list of error handler functions that will be called if a panic occurs.
//     Each handler receives the error containing panic details, timestamp, and stack trace.
//
// Example:
//
//	// Simple usage with a logger as error handler
//	Go(func() {
//	    // Your concurrent code here
//	    processData()
//	}, func(err error) {
//	    log.Printf("Error in goroutine: %v", err)
//	})
//
// The function does not return any values and does not wait for the goroutine to complete.
func Go(fn func(), panicFns ...func(error)) {
	withRecoverFn := WithRecover(fn, panicFns...)
	if withRecoverFn == nil {
		return
	}
	go withRecoverFn()
}

// WithRecover wraps a function with panic recovery logic.
// It returns a new function that will execute the provided function and recover from any panics.
// If panic occurs, it creates a PanicError and passes it to each of the provided error handler functions.
//
// Parameters:
//   - fn: The function to wrap with recovery logic
//   - panicFns: Optional list of functions to handle the panic error
//
// Returns:
//   - A function that executes the original function with panic recovery
//   - If fn is nil, returns nil
//
// This function can be used directly when you want recovery but don't need a new goroutine.
func WithRecover(fn func(), panicFns ...func(error)) func() {
	if fn == nil {
		return fn
	}
	return func() {
		defer func() {
			if r := recover(); r != nil {
				if len(panicFns) == 0 {
					return
				}
				err := NewPanicError(r, debug.Stack())
				for _, panicFn := range panicFns {
					panicFn(err)
				}
			}
		}()
		fn()
	}
}
