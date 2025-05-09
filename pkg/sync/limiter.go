package sync

// Limiter implements a concurrency limiter that restricts the number of
// concurrent operations to a configurable maximum.
//
// It works as a semaphore that allows at most N concurrent operations
// to execute simultaneously. This is useful for:
// - Limiting concurrent connections to external services
// - Controlling resource usage in concurrent operations
// - Preventing overload in resource-intensive tasks
//
// Example:
//
//	package main
//
//	import (
//	    "fmt"
//	    "sync"
//	    "time"
//	    "github.com/Tangerg/lynx/pkg/sync"
//	)
//
//	func main() {
//	    // Create a limiter allowing 3 concurrent operations
//	    limiter := sync.NewLimiter(3)
//
//	    // WaitGroup to wait for all goroutines to finish
//	    var wg sync.WaitGroup
//
//	    // Launch 10 goroutines, but only 3 will run concurrently
//	    for i := 0; i < 10; i++ {
//	        wg.Add(1)
//	        go func(id int) {
//	            defer wg.Done()
//
//	            fmt.Printf("Goroutine %d attempting to acquire slot\n", id)
//	            limiter.Acquire()
//	            defer limiter.Release()
//
//	            fmt.Printf("Goroutine %d acquired slot, working...\n", id)
//	            // Simulate work
//	            time.Sleep(2 * time.Second)
//	            fmt.Printf("Goroutine %d finished work\n", id)
//	        }(i)
//	    }
//
//	    wg.Wait()
//	    fmt.Println("All goroutines completed")
//	}
type Limiter struct {
	semaphore chan struct{} // Channel used as a counting semaphore to track available slots
}

// NewLimiter creates a new Limiter with the specified maximum number of
// concurrent operations.
//
// The parameter 'max' defines the upper limit of concurrent operations.
// It must be greater than zero, otherwise the function will panic.
//
// Example:
//
//	// Create a limiter that allows 10 concurrent operations
//	limiter := NewLimiter(10)
func NewLimiter(max int) *Limiter {
	if max <= 0 {
		panic("max must be > 0")
	}
	return &Limiter{
		semaphore: make(chan struct{}, max),
	}
}

// Acquire acquires a resource from the limiter, blocking if the maximum
// number of concurrent operations has been reached.
//
// This method should be called before starting a concurrent operation.
// If all slots are currently in use, this call will block until a slot
// becomes available when another goroutine calls Release().
//
// Example:
//
//	func processItem(item Item, limiter *Limiter) {
//	    limiter.Acquire() // Wait for an available slot
//	    defer limiter.Release() // Ensure slot is released when done
//
//	    // Process item (concurrent work happens here)
//	    process(item)
//	}
func (l *Limiter) Acquire() {
	l.semaphore <- struct{}{}
}

// Release releases a resource back to the limiter.
//
// This method should be called when a concurrent operation completes.
// It frees up a slot so another goroutine waiting in Acquire can proceed.
// Failing to call Release after Acquire will eventually cause deadlock
// if enough goroutines are waiting.
func (l *Limiter) Release() {
	<-l.semaphore
}
