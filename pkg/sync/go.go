package sync

import (
	"fmt"
	"runtime/debug"
	"time"
)

// Go launches a goroutine with built-in panic recovery.
// This function provides a safer way to start goroutines by automatically recovering from panics
// and invoking provided error handlers. It captures panic information including timestamp and stack trace
// for better debugging.
//
// Parameters:
//   - fn: The function to execute in the goroutine. This is the main function that will run concurrently.
//   - errfns: Optional variadic list of error handler functions that will be called if a panic occurs.
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
func Go(fn func(), errfns ...func(error)) {
	if fn == nil {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				if len(errfns) == 0 {
					return
				}
				timestamp := time.Now().Format(time.RFC3339Nano)
				stackTrace := string(debug.Stack())
				err := fmt.Errorf("panic recovered: \n%+v\n timestamp: \n%s\nstack trace:\n%s\n", r, timestamp, stackTrace)
				for _, errfn := range errfns {
					errfn(err)
				}
			}
		}()
		fn()
	}()
}
