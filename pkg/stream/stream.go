package stream

import (
	"context"
	"errors"
	"io"
	"sync"

	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

// ErrStreamClosed is returned when attempting to operate on a stream that has been closed.
// This error occurs in the following scenarios:
//   - Writing to a stream that has already been closed
//   - Attempting to close a stream that is already closed (duplicate close)
var ErrStreamClosed = errors.New("stream already closed")

// Reader defines the interface for reading values from a stream.
// All read operations are context-aware and support cancellation.
// Type parameter T represents the type of values that can be read from the stream.
type Reader[T any] interface {
	// Read attempts to read a single value from the stream.
	// It blocks until a value is available, the context is cancelled, or the stream is closed.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//
	// Returns:
	//   - T: The value read from the stream (zero value if error occurred)
	//   - error: nil on success, ctx.Err() if context was cancelled,
	//            io.EOF if stream is closed and all data has been consumed,
	//            or other errors for exceptional conditions
	Read(ctx context.Context) (T, error)
}

// Writer defines the interface for writing values to a stream.
// All write operations are context-aware and support cancellation.
// Type parameter T represents the type of values that can be written to the stream.
type Writer[T any] interface {
	// Write attempts to write a single value to the stream.
	// It blocks until the value is accepted, the context is cancelled, or the stream is closed.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - v: The value to write to the stream
	//
	// Returns:
	//   - error: nil on success, ctx.Err() if context was cancelled,
	//            ErrStreamClosed if attempting to write to a closed stream,
	//            or other errors for exceptional conditions
	Write(ctx context.Context, v T) error
}

// Stream combines Reader and Writer interfaces with io.Closer to provide
// a complete bidirectional streaming interface. It supports both reading
// and writing operations with proper lifecycle management.
type Stream[T any] interface {
	Reader[T]
	Writer[T]
	io.Closer
}

// stream implements the Stream interface using Go channels as the underlying
// transport mechanism with RWMutex for synchronization.
//
// The implementation provides:
//   - Thread-safe read/write operations using RWMutex
//   - Buffered and unbuffered channels
//   - Context-aware operations with cancellation support
//   - Graceful shutdown with proper resource cleanup
//   - Complete elimination of race conditions
//   - Proper EOF semantics for readers
//
// Synchronization Strategy:
//   - Write operations acquire read lock (RLock): allows concurrent writes,
//     but blocks when Close() holds write lock
//   - Close operation acquires write lock (Lock): waits for all writes to complete,
//     then safely closes the channel
//   - Read operations don't need locks: channel closure provides sufficient synchronization
//
// Type parameter T represents the type of values that flow through the stream.
type stream[T any] struct {
	// value is the underlying channel for data transport
	value chan T

	// mu protects the isClosed flag and coordinates Write/Close operations
	// - Read lock (RLock): held during Write operations
	// - Write lock (Lock): held during Close operation
	mu sync.RWMutex

	// isClosed tracks whether the stream has been closed
	// Protected by mu, so no need for atomic operations
	isClosed bool
}

// Read attempts to read a single value from the channel-based stream.
// It implements context-aware reading with proper handling of cancellation
// and stream closure states.
//
// Behavior:
//   - Blocks until a value becomes available on the channel
//   - Respects context cancellation and returns ctx.Err()
//   - Returns io.EOF when the stream is closed and no more data is available
//   - Returns zero value of type T when an error occurs
//   - Allows reading buffered data even after the stream is closed
//
// Synchronization:
// Read does not acquire any locks because:
//   - Channel operations are already thread-safe
//   - Channel closure (done by Close()) provides memory barrier
//   - Once channel is closed, all readers will see it
//
// Thread Safety:
// This method is safe for concurrent use by multiple goroutines.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//
// Returns:
//   - T: The value read from the stream (zero value if error occurred)
//   - error: nil on success, ctx.Err() if context was cancelled,
//     io.EOF if stream is closed and no more data is available
//
// Example:
//
//	ctx := context.Background()
//	value, err := stream.Read(ctx)
//	if err == io.EOF {
//	    // Stream is closed and no more data available
//	} else if err != nil {
//	    // Context cancelled or other error
//	} else {
//	    // Successfully read value
//	}
func (s *stream[T]) Read(ctx context.Context) (v T, err error) {
	select {
	// Priority 1: Check if context is cancelled
	// This ensures that cancellation requests are handled promptly
	case <-ctx.Done():
		return v, ctx.Err()

	// Priority 2: Attempt to read from the value channel
	// The ok idiom is used to detect channel closure
	case val, ok := <-s.value:
		if !ok {
			// Channel closed: no more data available
			return v, io.EOF
		}
		// Successfully read a value
		return val, nil
	}
}

// Write attempts to write a single value to the channel-based stream.
// It implements context-aware writing with proper handling of cancellation
// and stream closure prevention using RWMutex for synchronization.
//
// Behavior:
//   - Blocks until the value is successfully written to the channel
//   - Respects context cancellation and returns ctx.Err()
//   - Prevents writes to closed streams by returning ErrStreamClosed
//   - For buffered streams, may complete immediately if buffer has space
//   - For unbuffered streams, blocks until a reader is available
//
// Synchronization:
// The method acquires a read lock (RLock) which:
//   - Allows multiple concurrent Write operations
//   - Prevents Close() from executing while Write is in progress
//   - Ensures Write cannot send to a closed channel
//
// Lock Lifetime:
// The read lock is held for the entire duration of the write operation,
// including the channel send. This is necessary because:
//   - We must check isClosed atomically with the channel send
//   - If we released the lock before sending, Close() could close the channel
//     in between, causing a panic
//
// Thread Safety:
// This method is safe for concurrent use by multiple goroutines.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - v: The value to write to the stream
//
// Returns:
//   - error: nil on success, ctx.Err() if context was cancelled,
//     ErrStreamClosed if attempting to write to a closed stream
//
// Example:
//
//	ctx := context.Background()
//	err := stream.Write(ctx, 42)
//	if errors.Is(err, ErrStreamClosed) {
//	    // Stream was closed, cannot write
//	} else if err != nil {
//	    // Context cancelled or other error
//	} else {
//	    // Successfully wrote value
//	}
func (s *stream[T]) Write(ctx context.Context, v T) error {
	// Acquire read lock - allows concurrent writes, blocks Close()
	// This lock must be held during the entire write operation to prevent
	// Close() from closing the channel while we're sending
	s.mu.RLock()

	// Check if stream is closed under the protection of the lock
	// This check is atomic with the channel send operation below
	if s.isClosed {
		s.mu.RUnlock()
		return ErrStreamClosed
	}

	// Perform channel send while holding the read lock
	// This guarantees that Close() cannot close the channel during send
	select {
	case <-ctx.Done():
		// Context cancelled - release lock and return error
		s.mu.RUnlock()
		return ctx.Err()

	case s.value <- v:
		// Successfully sent value - release lock and return
		s.mu.RUnlock()
		return nil
	}
}

// Close shuts down the stream, preventing further writes and eventually reads.
// It implements graceful shutdown semantics with proper resource cleanup using
// RWMutex to ensure thread-safe, race-free closure.
//
// Closure Process:
//  1. Acquires write lock (Lock) - this waits for all ongoing Write operations
//     to complete, as they hold read locks
//  2. Checks if already closed - returns error if so
//  3. Sets isClosed flag to true
//  4. Closes the value channel - this signals EOF to all readers
//  5. Releases write lock
//
// Behavior:
//   - First call: Waits for ongoing writes, closes channel, returns nil
//   - Subsequent calls: Returns ErrStreamClosed immediately
//   - Ongoing Write operations will complete before channel is closed
//   - New Write operations will see isClosed=true and return error
//   - Read operations will receive buffered data, then io.EOF
//
// Synchronization Guarantees:
// The write lock ensures that:
//   - All Write operations holding read locks complete before Close proceeds
//   - No new Write operations can acquire read lock while Close holds write lock
//   - Channel is only closed when no Write operation is in progress
//   - Therefore, Write will never panic due to send on closed channel
//
// Thread Safety:
// This method is safe for concurrent use by multiple goroutines and
// can be called multiple times without causing panics.
//
// Returns:
//   - error: nil on successful closure (first call),
//     ErrStreamClosed if the stream was already closed (subsequent calls)
//
// Example:
//
//	// Safe concurrent close
//	go stream.Close()
//	go stream.Close()
//	// One returns nil, other returns ErrStreamClosed
//
//	// Close while operations are in progress
//	go stream.Write(ctx, value)  // Will complete or receive ErrStreamClosed
//	go stream.Read(ctx)           // Will receive io.EOF after drain
//	stream.Close()                // Waits for write, then closes
func (s *stream[T]) Close() error {
	// Acquire write lock - this blocks until all read locks are released
	// (i.e., all ongoing Write operations have completed)
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already closed
	if s.isClosed {
		return ErrStreamClosed
	}

	// Mark as closed
	s.isClosed = true

	// Close the channel to signal EOF to readers
	// This is safe because:
	// 1. We hold the write lock, so no Write operation can be in progress
	// 2. No new Write operations can start (they will see isClosed=true)
	close(s.value)

	return nil
}

// IsClosed reports whether the stream has been closed.
// This method provides a non-blocking way to check the stream's state
// without performing actual I/O operations.
//
// The check is performed under read lock protection, ensuring consistent
// state observation.
//
// Thread Safety:
// This method is safe for concurrent use by multiple goroutines.
//
// Returns:
//   - bool: true if the stream has been closed, false otherwise
//
// Note:
// The returned value represents the state at the moment of the call.
// The stream might be closed immediately after this method returns false.
// For critical sections, rely on the error returns from Read/Write operations
// rather than this check.
//
// Example:
//
//	if stream.IsClosed() {
//	    // Stream is closed at this moment
//	    // But still handle ErrStreamClosed from Write for correctness
//	}
func (s *stream[T]) IsClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isClosed
}

// NewStream creates a new Stream instance with configurable buffering.
// The function provides simple buffer size configuration for common use cases.
//
// Buffer Configuration:
//   - No parameters: Creates an unbuffered (synchronous) stream
//   - First parameter < 0: Creates an unbuffered stream (treated as 0)
//   - First parameter >= 0: Used as buffer capacity
//   - Additional parameters: Ignored (only first parameter is considered)
//
// Buffer Behavior:
//
//   - Unbuffered (size 0): Write operations block until a reader is available.
//     This provides synchronous communication guarantees - the writer knows
//     the value has been received when Write() returns.
//
//   - Buffered (size > 0): Write operations complete immediately until buffer is full.
//     This provides asynchronous communication - writers can proceed without
//     waiting for readers, improving throughput in producer-consumer scenarios.
//
//   - Buffer full: Write operations block until buffer space becomes available
//     (either through reads or context cancellation).
//
// Memory Allocation:
// The buffer is allocated eagerly when the stream is created. For large buffer
// sizes, consider the memory implications:
//   - Unbuffered: ~64 bytes overhead (channel + RWMutex + bool)
//   - Buffered: 64 bytes + (buffer_size * sizeof(T))
//
// Thread Safety:
// The returned Stream is safe for concurrent use by multiple goroutines.
// Multiple readers and writers can operate on the stream simultaneously.
//
// Parameters:
//   - sizes: Optional buffer sizes (variadic). Only the first value is used.
//
// Returns:
//   - Stream[T]: A new stream instance ready for use
//
// Example Usage:
//
//	// Unbuffered: synchronous communication
//	unbuffered := NewStream[int]()
//
//	// Buffered: asynchronous communication with capacity of 10
//	buffered := NewStream[int](10)
//
//	// Also unbuffered: negative values treated as 0
//	alsoUnbuffered := NewStream[int](-1)
//
//	// Only first parameter used: buffer capacity of 5
//	stream := NewStream[string](5,10,15)
//
//	// Full example with writer and reader
//	s := NewStream[int](5)
//	ctx := context.Background()
//
//	// Writer goroutine
//	go func() {
//	    defer s.Close()
//	    for i := 0; i < 10; i++ {
//	        if err := s.Write(ctx, i); err != nil {
//	            break
//	        }
//	    }
//	}()
//
//	// Reader goroutine
//	for {
//	    val, err := s.Read(ctx)
//	    if err == io.EOF {
//	        break
//	    }
//	    if err != nil {
//	        // handle error
//	        break
//	    }
//	    fmt.Println(val)
//	}
func NewStream[T any](sizes ...int) Stream[T] {
	// Extract the first size parameter, defaulting to 0 if none provided
	size, _ := pkgSlices.First(sizes)

	// Normalize negative sizes to 0 (unbuffered)
	// This provides consistent behavior regardless of input
	if size < 0 {
		size = 0
	}

	// Create and return the stream instance
	return &stream[T]{
		// Create the value channel with specified capacity
		// - size=0: unbuffered channel (synchronous)
		// - size>0: buffered channel (asynchronous up to capacity)
		value: make(chan T, size),

		// mu is zero-initialized and ready to use
		// isClosed is zero-initialized to false (stream is open)
	}
}
