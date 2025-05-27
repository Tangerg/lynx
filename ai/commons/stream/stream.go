package stream

import (
	"context"
	"errors"
	"io"
	"sync"
)

// ErrStreamClosed is returned when attempting to operate on a stream that has been closed.
// This error is returned in the following scenarios:
//   - Writing to a stream that has already been closed
//   - Attempting to close a stream that is already closed (duplicate close)
var ErrStreamClosed = errors.New("stream already closed")

// Reader defines the interface for reading values from a stream.
// The Read operation is context-aware and supports cancellation.
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
// The Write operation is context-aware and supports cancellation.
// Type parameter T represents the type of values that can be written to the stream.
type Writer[T any] interface {
	// Write attempts to write a single value to the stream.
	// It blocks until the value is accepted, the context is cancelled, or the stream is closed.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - t: The value to write to the stream
	//
	// Returns:
	//   - error: nil on success, ctx.Err() if context was cancelled,
	//            ErrStreamClosed if attempting to write to a closed stream,
	//            or other errors for exceptional conditions
	Write(ctx context.Context, t T) error
}

// ReadWriter combines both Reader and Writer interfaces, providing bidirectional stream operations.
// This interface allows a single stream to support both reading and writing operations
// with the same value type T.
//
// Type parameter T represents the type of values that can be both read from and written to the stream.
type ReadWriter[T any] interface {
	Reader[T]
	Writer[T]
}

// Closer is an alias for io.Closer, providing the standard Close method.
// This allows streams to implement proper resource cleanup and lifecycle management.
//
// Error behavior:
//   - First call to Close(): should return nil on successful closure
//   - Subsequent calls to Close(): should return ErrStreamClosed
//   - After closure: Write operations return ErrStreamClosed
//   - After closure: Read operations return io.EOF when all buffered data is consumed
type Closer = io.Closer

var (
	_ ReadWriter[any] = (*Stream[any])(nil)
	_ Closer          = (*Stream[any])(nil)
)

// Stream implements a stream using Go channels as the underlying transport mechanism.
// It provides thread-safe read/write operations with proper lifecycle management.
// The stream supports buffering and graceful shutdown semantics.
//
// Type parameter T represents the type of values that flow through the stream.
type Stream[T any] struct {
	value  chan T
	closed chan struct{}
	once   sync.Once
}

// Read attempts to read a single value from the channel stream.
// It respects context cancellation and properly handles stream closure.
//
// The method blocks until:
//   - A value becomes available on the channel
//   - The provided context is cancelled
//   - The stream is closed (channel is closed)
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//
// Returns:
//   - T: The value read from the stream (zero value if error occurred)
//   - error: nil on success, ctx.Err() if context was cancelled,
//     io.EOF if stream is closed and no more data is available
func (c *Stream[T]) Read(ctx context.Context) (v T, err error) {
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

// Write attempts to write a single value to the channel stream.
// It respects context cancellation and prevents writes to closed streams.
//
// The method blocks until:
//   - The value is successfully written to the channel
//   - The provided context is cancelled
//   - The stream is detected as closed
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - t: The value to write to the stream
//
// Returns:
//   - error: nil on success, ctx.Err() if context was cancelled,
//     ErrStreamClosed if attempting to write to a closed stream
func (c *Stream[T]) Write(ctx context.Context, t T) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.closed:
		return ErrStreamClosed
	case c.value <- t:
		return nil
	}
}

// Close shuts down the channel stream, preventing further writes and eventually reads.
// This method is safe to call concurrently and multiple times.
//
// The closure process:
//  1. Signals closure state via the closed channel
//  2. Closes the value channel to prevent new writes and signal EOF to readers
//  3. Uses sync.Once to ensure the closure logic runs exactly once
//
// Returns:
//   - error: nil on successful closure (first call),
//     ErrStreamClosed if the stream was already closed (subsequent calls)
func (c *Stream[T]) Close() error {
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

// NewStream creates a new Stream instance with optional buffering.
// The function accepts multiple size parameters but only uses the first non-negative value.
//
// Buffer behavior:
//   - If no sizes provided or all are negative: creates an unbuffered channel (synchronous)
//   - If a non-negative size is provided: creates a buffered channel with that capacity
//   - Only the first valid (non-negative) size parameter is used
//
// Parameters:
//   - sizes: Optional buffer sizes (variadic). Only the first non-negative value is used.
//
// Returns:
//   - *Stream[T]: A new channel stream instance ready for use
//
// Example usage:
//
//	stream1 := NewStream[int]()        // Unbuffered
//	stream2 := NewStream[int](10)      // Buffered with capacity 10
//	stream3 := NewStream[int](-1,5)   // Buffered with capacity 5 (ignores -1)
func NewStream[T any](sizes ...int) *Stream[T] {
	var bufferSize = 0
	for _, size := range sizes {
		if size >= 0 {
			bufferSize = size
			break
		}
	}
	return &Stream[T]{
		value:  make(chan T, bufferSize),
		closed: make(chan struct{}),
	}
}
