package stream

import (
	"context"
)

// SliceReader creates a Reader from a slice of values, emitting each item sequentially.
// This is a convenience function for creating streams from static data collections.
// The resulting stream is "cold" - it contains all data immediately and will emit
// io.EOF after all items have been read.
//
// The function creates a buffered channel with capacity equal to the slice length,
// writes all items synchronously, and then closes the stream. This ensures that
// all data is immediately available for consumption without blocking.
//
// Type parameter T represents the type of values in the slice and the resulting stream.
//
// Parameters:
//   - items: A slice of values to be emitted by the stream
//
// Returns:
//   - Reader[T]: A stream that will emit each item from the slice in order,
//     followed by io.EOF when all items have been consumed
//
// Example usage:
//
//	numbers := SliceReader([]int{1, 2, 3, 4, 5})
//	for {
//	    val, err := numbers.Read(ctx)
//	    if err == io.EOF {
//	        break // All items consumed
//	    }
//	    fmt.Println(val) // Prints: 1, 2, 3, 4, 5
//	}
func SliceReader[T any](items []T) Reader[T] {
	cs := NewStream[T](len(items))
	ctx := context.Background()
	for _, item := range items {
		_ = cs.Write(ctx, item)
	}
	_ = cs.Close()
	return cs
}

// ChannelReader creates a Reader from a receive-only channel by consuming available values.
// This function reads all immediately available data from the source channel using
// non-blocking operations, then creates a stream from the collected data.
//
// The function uses a select statement with a default case to avoid blocking:
//   - If data is available, it reads and collects the value
//   - If the channel is closed, it returns a stream with all collected data
//   - If no data is immediately available, it returns a stream with collected data so far
//
// This approach is suitable for:
//   - Channels that may or may not be closed
//   - Draining buffered channels without blocking
//   - Converting available channel data to streams
//   - Scenarios where you want immediate results without waiting
//
// Memory considerations:
//   - Only loads immediately available data into memory
//   - Non-blocking operation prevents goroutine leaks
//   - The resulting stream is "cold" - all collected data is immediately available
//
// Type parameter T represents the type of values in the channel and resulting stream.
//
// Parameters:
//   - items: A receive-only channel to consume data from.
//     The channel can be open, closed, or empty.
//
// Returns:
//   - Reader[T]: A stream containing all immediately available values from the channel
//
// Example usage:
//
//	ch := make(chan int, 3)
//	ch <- 1
//	ch <- 2
//	ch <- 3
//	// Note: channel not closed, but buffered data is available
//
//	reader := ChannelReader(ch)
//	// All buffered data is now available in the reader
func ChannelReader[T any](items <-chan T) Reader[T] {
	itemSlice := make([]T, 0)
	for {
		select {
		case item, ok := <-items:
			if !ok {
				return SliceReader(itemSlice)
			}
			itemSlice = append(itemSlice, item)
		default:
			return SliceReader(itemSlice)
		}
	}
}

// Pipe creates a connected Reader-Writer pair using the same underlying stream.
// This function returns two interfaces to the same Stream instance, allowing
// for decoupled producer-consumer patterns where the reader and writer can be
// passed to different goroutines or functions.
//
// The pipe creates an unbuffered stream by default, meaning:
//   - Write operations will block until a corresponding Read is ready
//   - Read operations will block until a corresponding Write provides data
//   - This provides synchronous communication between producer and consumer
//
// Type parameter T represents the type of values that will flow through the pipe.
//
// Returns:
//   - Reader[T]: Interface for reading from the pipe
//   - Writer[T]: Interface for writing to the pipe
//
// Example usage:
//
//	reader, writer := Pipe[int]()
//
//	// In producer goroutine
//	go func() {
//	    defer writer.Close()
//	    for i := 0; i < 5; i++ {
//	        writer.Write(ctx, i)
//	    }
//	}()
//
//	// In consumer goroutine
//	for {
//	    val, err := reader.Read(ctx)
//	    if err == io.EOF {
//	        break
//	    }
//	    fmt.Println(val)
//	}
func Pipe[T any]() (Reader[T], Writer[T]) {
	cs := NewStream[T]()
	return cs, cs
}

// teeReader implements a Reader that duplicates read operations to a Writer.
// It reads from the source reader and simultaneously writes the same data
// to the destination writer, implementing a "tee" operation similar to the
// Unix tee command or Go's io.TeeReader.
//
// The teeReader maintains the original read semantics while adding a side effect
// of writing each successfully read value to the destination writer.
type teeReader[T any] struct {
	reader Reader[T] // Source reader to read data from
	writer Writer[T] // Destination writer to duplicate data to
}

// TeeReader creates a new Reader that reads from the source reader and
// simultaneously writes each successfully read value to the destination writer.
// This is useful for logging, debugging, monitoring, or creating data pipelines
// with side effects.
//
// The tee operation maintains the original read semantics:
//   - If the source read fails, no write occurs and the read error is returned
//   - If the source read succeeds but the write fails, the write error is returned
//   - The read value is always returned when the source read succeeds, regardless of write status
//   - The operation is atomic per read - either both succeed or an error is returned
//
// Common use cases:
//   - Logging data flow for debugging
//   - Creating audit trails
//   - Branching data to multiple destinations
//   - Monitoring stream contents
//
// Type parameter T represents the type of values flowing through the streams.
//
// Parameters:
//   - reader: The source Reader to read data from
//   - writer: The destination Writer to duplicate data to
//
// Returns:
//   - Reader[T]: A new Reader that performs the tee operation
//
// Example usage:
//
//	source := SliceReader([]int{1, 2, 3})
//	logStream := NewStream[int]()
//
//	tee := TeeReader(source, logStream)
//
//	// Reading from tee also writes to logStream
//	val, err := tee.Read(ctx) // val=1, also written to logStream
//
//	// Separate goroutine can read from logStream for logging
//	go func() {
//	    for {
//	        logVal, err := logStream.Read(ctx)
//	        if err == io.EOF {
//	            break
//	        }
//	        log.Printf("Data: %v", logVal)
//	    }
//	}()
func TeeReader[T any](reader Reader[T], writer Writer[T]) Reader[T] {
	return &teeReader[T]{
		reader: reader,
		writer: writer,
	}
}

// Read performs a tee operation: reads from the source and writes to the destination.
// The method ensures that data flows through both the read and write paths,
// making it useful for monitoring, logging, or branching data streams.
//
// Operation sequence:
//  1. Read a value from the source reader using the provided context
//  2. If the read operation fails, return immediately with the read error
//  3. If the read operation succeeds, attempt to write the value to the destination
//  4. Return the successfully read value along with any write error
//
// Error handling:
//   - Source read errors take precedence over write errors
//   - Write errors are returned but don't prevent the read value from being returned
//   - Context cancellation during read will abort the entire operation
//   - Context cancellation during write (after successful read) will return the write error
//
// The method guarantees that:
//   - A successful read always returns the read value, even if write fails
//   - No write attempt is made if the read fails
//   - The same context is used for both read and write operations
//
// Parameters:
//   - ctx: Context for cancellation and timeout control for both operations
//
// Returns:
//   - T: The value read from the source (always returned if source read succeeds)
//   - error: nil if both read and write succeed,
//     source read error if source read fails,
//     write error if source read succeeds but write fails
func (t *teeReader[T]) Read(ctx context.Context) (val T, err error) {
	val, err = t.reader.Read(ctx)
	if err != nil {
		return
	}
	err = t.writer.Write(ctx, val)
	return
}
