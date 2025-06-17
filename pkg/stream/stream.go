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
// transport mechanism. It provides thread-safe read/write operations with
// proper lifecycle management and graceful shutdown semantics.
//
// The implementation supports:
//   - Buffered and unbuffered channels
//   - Context-aware operations with cancellation support
//   - Thread-safe concurrent access
//   - Graceful shutdown with proper resource cleanup
//   - Prevention of operations on closed streams
//
// Type parameter T represents the type of values that flow through the stream.
type stream[T any] struct {
	// value is the underlying channel for data transport
	value chan T
	// closed signals when the stream has been closed
	closed chan struct{}
	// once ensures Close() operations are executed exactly once
	once sync.Once
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
func (c *stream[T]) Read(ctx context.Context) (v T, err error) {
	select {
	case <-ctx.Done():
		err = ctx.Err()
		return
	case val, ok := <-c.value:
		if !ok {
			err = io.EOF
			return
		}
		v = val
		return
	}
}

// Write attempts to write a single value to the channel-based stream.
// It implements context-aware writing with proper handling of cancellation
// and stream closure prevention.
//
// Behavior:
//   - Blocks until the value is successfully written to the channel
//   - Respects context cancellation and returns ctx.Err()
//   - Prevents writes to closed streams by returning ErrStreamClosed
//   - For buffered streams, may complete immediately if buffer has space
//   - For unbuffered streams, blocks until a reader is available
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
func (c *stream[T]) Write(ctx context.Context, v T) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.closed:
		return ErrStreamClosed
	case c.value <- v:
		return nil
	}
}

// Close shuts down the stream, preventing further writes and eventually reads.
// It implements graceful shutdown semantics with proper resource cleanup.
//
// Closure Process:
//  1. Signals closure state via the closed channel to prevent new writes
//  2. Closes the value channel to prevent new writes and signal EOF to readers
//  3. Uses sync.Once to ensure the closure logic runs exactly once
//
// Behavior:
//   - First call: Performs actual closure and returns nil
//   - Subsequent calls: Returns ErrStreamClosed immediately
//   - Ongoing Read operations will receive io.EOF after buffered data is consumed
//   - Ongoing Write operations will receive ErrStreamClosed
//   - New operations after closure will fail immediately
//
// Thread Safety:
// This method is safe for concurrent use by multiple goroutines and
// can be called multiple times without causing panics.
//
// Returns:
//   - error: nil on successful closure (first call),
//     ErrStreamClosed if the stream was already closed (subsequent calls)
func (c *stream[T]) Close() error {
	select {
	case <-c.closed:
		return ErrStreamClosed
	default:
		c.once.Do(func() {
			close(c.closed)
			close(c.value)
		})
		return nil
	}
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
//   - Unbuffered (size 0): Write operations block until a reader is available
//   - Buffered (size > 0): Write operations complete immediately until buffer is full
//   - Buffer full: Write operations block until buffer space becomes available
//
// Thread Safety:
// The returned Stream is safe for concurrent use by multiple goroutines.
//
// Parameters:
//   - sizes: Optional buffer sizes (variadic). Only the first value is used.
//
// Returns:
//   - Stream[T]: A new stream instance ready for use
//
// Example Usage:
//
//	unbuffered := NewStream[int]()           // Synchronous communication
//	buffered := NewStream[int](10)           // Buffer capacity of 10
//	alsoUnbuffered := NewStream[int](-1)     // Unbuffered (negative treated as 0)
//	ignored := NewStream[string](5,10)      // Buffer capacity of 5 (10 is ignored)
func NewStream[T any](sizes ...int) Stream[T] {
	size, _ := pkgSlices.First(sizes)
	if size < 0 {
		size = 0
	}
	return &stream[T]{
		value:  make(chan T, size),
		closed: make(chan struct{}),
	}
}
