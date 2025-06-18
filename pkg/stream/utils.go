package stream

import (
	"context"
	"io"
	"slices"
)

// OfSliceReader creates a Reader from a slice of values, emitting each item sequentially.
// This function converts static data collections into readable streams by creating
// a buffered channel with capacity equal to the slice length and pre-loading all items.
//
// The resulting stream is "cold" - all data is immediately available without blocking,
// and the stream will emit io.EOF after all items have been consumed.
//
// Type parameter T represents the type of values in the slice and the resulting stream.
//
// Parameters:
//   - items: A slice of values to be emitted by the stream
//
// Returns:
//   - Reader[T]: A stream that emits each item from the slice in order,
//     followed by io.EOF when all items have been consumed
//
// Example usage:
//
//	numbers := OfSliceReader([]int{1, 2, 3, 4, 5})
//	for {
//	    val, err := numbers.Read(ctx)
//	    if err == io.EOF {
//	        break // All items consumed
//	    }
//	    fmt.Println(val) // Prints: 1, 2, 3, 4, 5
//	}
func OfSliceReader[T any](items []T) Reader[T] {
	cs := NewStream[T](len(items))
	ctx := context.Background()
	for _, item := range items {
		_ = cs.Write(ctx, item)
	}
	_ = cs.Close()
	return cs
}

// OfChannelReader creates a Reader from a receive-only channel by draining all
// immediately available values using non-blocking operations.
//
// This function reads available data from the source channel without blocking:
//   - If data is available, it reads and collects the value
//   - If the channel is closed, it creates a stream with all collected data
//   - If no data is immediately available, it creates a stream with collected data so far
//
// The non-blocking approach prevents goroutine leaks and is suitable for:
//   - Draining buffered channels without waiting
//   - Converting immediately available channel data to reusable streams
//   - Scenarios requiring immediate results without blocking
//
// Type parameter T represents the type of values in the channel and resulting stream.
//
// Parameters:
//   - items: A receive-only channel to drain data from
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
//	// Channel has buffered data but is not closed
//
//	reader := OfChannelReader(ch)
//	// All buffered data is now available in the reader
func OfChannelReader[T any](items <-chan T) Reader[T] {
	itemSlice := make([]T, 0)
	for {
		select {
		case item, ok := <-items:
			if !ok {
				return OfSliceReader(itemSlice)
			}
			itemSlice = append(itemSlice, item)
		default:
			return OfSliceReader(itemSlice)
		}
	}
}

// Pipe creates a connected Reader-Writer pair sharing the same underlying stream.
// This function returns two interfaces to a single Stream instance, enabling
// decoupled producer-consumer patterns where readers and writers can be passed
// to different goroutines or functions.
//
// The pipe uses an unbuffered stream by default, providing synchronous communication:
//   - Write operations block until a corresponding Read is ready
//   - Read operations block until a corresponding Write provides data
//   - This ensures synchronous handoff between producer and consumer
//
// Type parameter T represents the type of values flowing through the pipe.
//
// Parameters:
//   - sizes: Optional buffer sizes (variadic). Only the first value is used.
//
// Returns:
//   - Reader[T]: Interface for reading from the pipe
//   - Writer[T]: Interface for writing to the pipe
//
// Example usage:
//
//	reader, writer := Pipe[int]()
//
//	// Producer goroutine
//	go func() {
//	    defer writer.Close()
//	    for i := 0; i < 5; i++ {
//	        writer.Write(ctx, i)
//	    }
//	}()
//
//	// Consumer goroutine
//	for {
//	    val, err := reader.Read(ctx)
//	    if err == io.EOF {
//	        break
//	    }
//	    fmt.Println(val)
//	}
func Pipe[T any](sizes ...int) (Reader[T], Writer[T]) {
	cs := NewStream[T](sizes...)
	return cs, cs
}

// teeReader implements a Reader that duplicates each read operation to a Writer.
// It performs a "tee" operation similar to Unix tee command or Go's io.TeeReader,
// reading from a source while simultaneously writing the same data to a destination.
//
// The teeReader maintains original read semantics while adding the side effect
// of duplicating each successfully read value to the destination writer.
type teeReader[T any] struct {
	reader Reader[T] // Source reader to read data from
	writer Writer[T] // Destination writer to duplicate data to
}

// Read performs a tee operation: reads from source and writes to destination.
// This method ensures data flows through both read and write paths, making it
// useful for monitoring, logging, or creating branched data streams.
//
// Operation sequence:
//  1. Read a value from the source reader using the provided context
//  2. If read fails, return immediately with the read error
//  3. If read succeeds, attempt to write the same value to the destination
//  4. Return the read value along with any write error
//
// Error handling guarantees:
//   - Source read errors take precedence over write errors
//   - Read values are always returned when source read succeeds, even if write fails
//   - No write attempt is made if the source read fails
//   - The same context is used for both read and write operations
//
// Parameters:
//   - ctx: Context for cancellation and timeout control for both operations
//
// Returns:
//   - T: The value read from source (returned when source read succeeds)
//   - error: nil if both operations succeed, source read error if read fails,
//     write error if read succeeds but write fails
func (t *teeReader[T]) Read(ctx context.Context) (val T, err error) {
	val, err = t.reader.Read(ctx)
	if err != nil {
		return
	}
	err = t.writer.Write(ctx, val)
	return
}

// TeeReader creates a Reader that reads from a source and simultaneously writes
// each successfully read value to a destination writer. This enables transparent
// data duplication for logging, debugging, monitoring, or creating data pipelines
// with side effects.
//
// The tee operation preserves original read semantics:
//   - If source read fails, no write occurs and read error is returned
//   - If source read succeeds but write fails, write error is returned with read value
//   - Read values are always returned when source read succeeds, regardless of write status
//   - Operations are atomic per read call
//
// Common use cases:
//   - Logging data flow for debugging purposes
//   - Creating audit trails of stream data
//   - Branching data streams to multiple destinations
//   - Monitoring stream contents without affecting main flow
//
// Type parameter T represents the type of values flowing through the streams.
//
// Parameters:
//   - reader: Source Reader to read data from
//   - writer: Destination Writer to duplicate data to
//
// Returns:
//   - Reader[T]: A new Reader that performs the tee operation
//
// Example usage:
//
//	source := OfSliceReader([]int{1, 2, 3})
//	logStream := NewStream[int]()
//
//	tee := TeeReader(source, logStream)
//
//	// Reading from tee also writes to logStream
//	val, err := tee.Read(ctx) // val=1, also written to logStream
//
//	// Separate goroutine can monitor the log stream
//	go func() {
//	    for {
//	        logVal, err := logStream.Read(ctx)
//	        if err == io.EOF {
//	            break
//	        }
//	        log.Printf("Data flow: %v", logVal)
//	    }
//	}()
func TeeReader[T any](reader Reader[T], writer Writer[T]) Reader[T] {
	return &teeReader[T]{
		reader: reader,
		writer: writer,
	}
}

// multiReader implements a Reader that sequentially reads from multiple readers.
// It reads from each reader in order until EOF, then moves to the next reader.
// When all readers are exhausted, it returns EOF.
type multiReader[T any] struct {
	readers []Reader[T]
}

// Read reads from the current reader in the sequence. When a reader returns EOF,
// it is removed from the list and reading continues with the next reader.
// The method includes optimization to flatten nested multiReaders to prevent
// performance degradation from excessive nesting.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//
// Returns:
//   - T: Value read from the current reader
//   - error: Error from the current reader, or EOF when all readers are exhausted
func (m *multiReader[T]) Read(ctx context.Context) (v T, err error) {
	for len(m.readers) > 0 {
		// Optimization: flatten nested multiReaders
		if len(m.readers) == 1 {
			if mr, ok := m.readers[0].(*multiReader[T]); ok {
				m.readers = mr.readers
				continue
			}
		}
		v, err = m.readers[0].Read(ctx)
		if err == io.EOF {
			m.readers = m.readers[1:] // Remove exhausted reader
			continue
		}
		return
	}
	return v, io.EOF
}

// MultiReader creates a Reader that sequentially reads from multiple readers.
// It reads all data from the first reader, then the second, and so on.
// This is useful for concatenating multiple data sources into a single stream.
//
// Type parameter T represents the type of values in the readers.
//
// Parameters:
//   - readers: Variable number of readers to read from sequentially
//
// Returns:
//   - Reader[T]: A reader that concatenates all input readers
//
// Example usage:
//
//	reader1 := OfSliceReader([]int{1, 2, 3})
//	reader2 := OfSliceReader([]int{4, 5, 6})
//	combined := MultiReader(reader1, reader2)
//	// Will read: 1, 2, 3, 4, 5, 6, then EOF
func MultiReader[T any](readers ...Reader[T]) Reader[T] {
	return &multiReader[T]{
		readers: slices.Clone(readers),
	}
}

// multiWriter implements a Writer that writes to multiple writers simultaneously.
// Each write operation is broadcasted to all writers in the list.
// If any write fails, the operation stops and returns the error.
type multiWriter[T any] struct {
	writers []Writer[T]
}

// Write writes the value to all writers in sequence. If any writer returns
// an error, the operation stops immediately and returns that error.
// This provides fail-fast behavior for broadcasting operations.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - v: Value to write to all writers
//
// Returns:
//   - error: nil if all writes succeed, or the first error encountered
func (m *multiWriter[T]) Write(ctx context.Context, v T) error {
	for _, writer := range m.writers {
		err := writer.Write(ctx, v)
		if err != nil {
			return err
		}
	}
	return nil
}

// MultiWriter creates a Writer that writes to multiple writers simultaneously.
// Each write operation is broadcasted to all destination writers.
// This is useful for duplicating data to multiple destinations, such as
// logging while writing to primary storage.
//
// Type parameter T represents the type of values to write.
//
// Parameters:
//   - writers: Variable number of writers to write to simultaneously
//
// Returns:
//   - Writer[T]: A writer that broadcasts to all input writers
//
// Example usage:
//
//	primary := NewStream[int]()
//	backup := NewStream[int]()
//	logger := NewStream[int]()
//
//	multi := MultiWriter(primary, backup, logger)
//	multi.Write(ctx, 42) // Writes to all three streams
func MultiWriter[T any](writers ...Writer[T]) Writer[T] {
	return &multiWriter[T]{
		writers: slices.Clone(writers),
	}
}

// mapperReader implements a Reader that applies a transformation function to each value
// read from the source reader. It provides a type-safe way to convert a stream of
// type T to a stream of type U using a pure transformation function.
//
// The mapperReader maintains the original stream semantics:
//   - Errors from the source reader are propagated unchanged
//   - EOF signals are preserved to maintain proper stream termination
//   - Context cancellation is handled through the source reader
//   - No data buffering is performed, ensuring low memory usage
//
// Type parameters:
//   - T: The input type of values from the source reader
//   - U: The output type of values after transformation
type mapperReader[T, U any] struct {
	reader Reader[T] // Source reader to read values from
	mapper func(T) U // Pure transformation function applied to each value
}

// Read transforms a value from the source reader using the mapper function.
// This method implements the Reader[U] interface by reading a value of type T
// from the source and converting it to type U.
//
// Operation sequence:
//  1. Read a value from the source reader using the provided context
//  2. If the read operation fails (including EOF), return immediately with the error
//  3. If the read succeeds, apply the mapper function to transform the value
//  4. Return the transformed value
//
// Error handling:
//   - Source reader errors (including io.EOF) are returned unchanged
//   - No additional errors are introduced by the transformation
//   - The mapper function is assumed to be pure and not fail
//
// Parameters:
//   - ctx: Context for cancellation and timeout control, passed to source reader
//
// Returns:
//   - U: The transformed value when source read succeeds
//   - error: Any error from the source reader, or nil on successful transformation
func (m *mapperReader[T, U]) Read(ctx context.Context) (v U, err error) {
	t, err := m.reader.Read(ctx)
	if err != nil {
		return
	}
	return m.mapper(t), nil
}

// Map creates a Reader that applies a transformation function to each value
// from the source reader. This enables type-safe stream transformations without
// data copying or buffering.
//
// The transformation is applied lazily on each Read() call, making it suitable for:
//   - Converting data types in streaming pipelines
//   - Applying business logic transformations
//   - Format conversions (e.g., JSON parsing, data normalization)
//   - Any pure functional transformation
//
// The mapper function should be:
//   - Pure (no side effects)
//   - Fast (as it's called for every value)
//   - Deterministic (same input produces same output)
//
// Type parameters:
//   - T: The input type of values from the source reader
//   - U: The output type of values after transformation
//
// Parameters:
//   - reader: The source Reader[T] to transform values from
//   - mapper: A pure function that transforms values from T to U.
//     Must not be nil.
//
// Returns:
//   - Reader[U]: A new Reader that emits transformed values of type U
//
// Panics:
//   - If mapper is nil
//
// Example usage:
//
//	// Convert integers to strings
//	intReader := OfSliceReader([]int{1, 2, 3, 4, 5})
//	stringReader := MapReader(intReader, func(i int) string {
//	    return fmt.Sprintf("number_%d", i)
//	})
//
//	// Process the transformed stream
//	for {
//	    str, err := stringReader.Read(ctx)
//	    if err == io.EOF {
//	        break
//	    }
//	    fmt.Println(str) // Prints: number_1, number_2, etc.
//	}
//
//	// Chain multiple transformations
//	pipeline := MapReader(
//	    MapReader(rawDataReader, parseData),    // []byte -> Data
//	    func(data Data) ProcessedData {         // Data -> ProcessedData
//	        return ProcessData(data)
//	    },
//	)
func Map[T, U any](reader Reader[T], mapper func(T) U) Reader[U] {
	if mapper == nil {
		panic("nil mapper function")
	}
	return &mapperReader[T, U]{
		reader: reader,
		mapper: mapper,
	}
}

// filterReader implements a Reader that filters values from the source reader
// based on a predicate function. It provides a type-safe way to create a stream
// that only contains values matching specific criteria.
//
// The filterReader maintains the original stream semantics:
//   - Errors from the source reader are propagated unchanged
//   - EOF signals are preserved to maintain proper stream termination
//   - Context cancellation is handled through the source reader
//   - Values are filtered lazily without pre-buffering
//   - No data copying is performed for values that pass the filter
//
// The filtering process is transparent to the caller - filtered values are
// simply skipped, and only matching values are returned through Read() calls.
//
// Type parameter:
//   - T: The type of values being filtered
type filterReader[T any] struct {
	reader    Reader[T]    // Source reader to read values from
	predicate func(T) bool // Predicate function that determines if a value should be included
}

// Read filters values from the source reader using the predicate function.
// This method implements the Reader[T] interface by reading values from the
// source until it finds one that passes the filter, or until an error occurs.
//
// Operation sequence:
//  1. Read a value from the source reader using the provided context
//  2. If the read operation fails (including EOF), return immediately with the error
//  3. If the read succeeds, apply the predicate function to test the value
//  4. If the predicate returns true, return the value
//  5. If the predicate returns false, repeat from step 1 (skip this value)
//
// Error handling:
//   - Source reader errors (including io.EOF) are returned unchanged
//   - No additional errors are introduced by the filtering process
//   - The predicate function is assumed to be pure and not fail
//   - Context cancellation is respected during the filtering loop
//
// Performance considerations:
//   - The method may need to read multiple values from the source to find one match
//   - In worst case (no matches), it will read until source EOF
//   - Each Read() call processes values lazily without buffering
//
// Parameters:
//   - ctx: Context for cancellation and timeout control, passed to source reader
//
// Returns:
//   - T: A value that satisfies the predicate when source read succeeds
//   - error: Any error from the source reader, or nil on successful filtering
func (f *filterReader[T]) Read(ctx context.Context) (v T, err error) {
	for {
		v, err = f.reader.Read(ctx)
		if err != nil {
			return
		}
		if f.predicate(v) {
			return
		}
	}
}

// Filter creates a Reader that only emits values from the source reader that
// satisfy the given predicate function. This enables type-safe stream filtering
// without data copying or buffering.
//
// The filtering is applied lazily on each Read() call, making it suitable for:
//   - Removing invalid or unwanted data from streams
//   - Implementing conditional data processing
//   - Creating subsets of data based on business logic
//   - Filtering out null, empty, or malformed values
//   - Any conditional data selection
//
// The predicate function should be:
//   - Pure (no side effects)
//   - Fast (as it's called for every value, including filtered ones)
//   - Deterministic (same input produces same output)
//   - Safe for concurrent use if the reader is used across goroutines
//
// Type parameter:
//   - T: The type of values being filtered
//
// Parameters:
//   - reader: The source Reader[T] to filter values from
//   - predicate: A pure function that returns true for values to keep,
//     false for values to skip. Must not be nil.
//
// Returns:
//   - Reader[T]: A new Reader that emits only values satisfying the predicate
//
// Panics:
//   - If predicate is nil
//
// Example usage:
//
//	// Filter positive numbers only
//	allNumbers := OfSliceReader([]int{-2, -1, 0, 1, 2, 3})
//	positiveNumbers := Filter(allNumbers, func(i int) bool {
//	    return i > 0
//	})
//
//	// Process the filtered stream
//	for {
//	    num, err := positiveNumbers.Read(ctx)
//	    if err == io.EOF {
//	        break
//	    }
//	    fmt.Println(num) // Prints: 1, 2, 3
//	}
//
//	// Filter valid email addresses
//	emailReader := OfSliceReader([]string{"valid@example.com", "invalid", "test@test.org"})
//	validEmails := Filter(emailReader, func(email string) bool {
//	    return strings.Contains(email, "@") && strings.Contains(email, ".")
//	})
//
//	// Chain filtering with other operations
//	pipeline := Map(
//	    Filter(
//	        rawDataReader,
//	        func(data RawData) bool { return data.IsValid() }, // Filter valid data
//	    ),
//	    func(data RawData) ProcessedData { return process(data) }, // Transform valid data
//	)
func Filter[T any](reader Reader[T], predicate func(T) bool) Reader[T] {
	if predicate == nil {
		panic("nil predicate function")
	}
	return &filterReader[T]{
		reader:    reader,
		predicate: predicate,
	}
}

// flatMapReader implements a Reader that flattens nested streams by applying a
// mapper function that converts each input value into a Reader, then flattens
// all resulting streams into a single output stream.
//
// This corresponds to Java's stream.flatMap() operation and is useful for:
//   - Converting nested collections into flat streams
//   - Processing hierarchical data structures
//   - Expanding single values into multiple values
//   - Joining related data from multiple sources
//
// The flattening process maintains order: all values from the first mapped stream
// are emitted before any values from the second mapped stream, and so on.
//
// Type parameters:
//   - T: The input type of values from the source reader
//   - U: The output type of values from the mapped readers
type flatMapReader[T, U any] struct {
	reader  Reader[T]         // Source reader to read values from
	mapper  func(T) Reader[U] // Function that maps each T to a Reader[U]
	current Reader[U]         // Currently active sub-reader, nil when no active reader
}

// Read flattens the nested streams by reading from the current sub-reader,
// and switching to the next sub-reader when the current one is exhausted.
//
// Operation sequence:
//  1. If there's a current sub-reader, try to read from it
//  2. If current sub-reader returns EOF, discard it and get next sub-reader
//  3. If no current sub-reader, read next value from source and map it to new sub-reader
//  4. Return the value from the sub-reader, or error if any step fails
//
// Error handling:
//   - Errors from sub-readers are propagated immediately (except EOF)
//   - Errors from source reader are propagated immediately
//   - EOF from source reader becomes EOF for the flat stream
//   - EOF from sub-readers triggers transition to next sub-reader
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//
// Returns:
//   - U: The next value from the current sub-reader
//   - error: Any error from source or sub-readers, or nil on success
func (f *flatMapReader[T, U]) Read(ctx context.Context) (v U, err error) {
	for {
		if f.current != nil {
			v, err = f.current.Read(ctx)
			if err == nil {
				return
			}
			if err == io.EOF {
				f.current = nil
				continue
			}
			return
		}

		sourceVal, err1 := f.reader.Read(ctx)
		if err1 != nil {
			return v, err1
		}

		f.current = f.mapper(sourceVal)
	}
}

// FlatMap creates a Reader that applies a mapper function to each value from the
// source reader, where the mapper returns a Reader, and then flattens all the
// resulting readers into a single stream.
//
// This enables powerful stream transformations such as:
//   - Expanding single items into multiple items
//   - Processing nested data structures
//   - Joining data from related sources
//   - Converting hierarchical data into flat streams
//
// The mapper function should:
//   - Return a valid Reader[U] for each input value
//   - Be pure and fast (called for every source value)
//   - Handle empty readers gracefully (they will be skipped)
//
// Type parameters:
//   - T: The input type of values from the source reader
//   - U: The output type of values from the mapped readers
//
// Parameters:
//   - reader: The source Reader[T] to read values from
//   - mapper: Function that converts each T into a Reader[U]. Must not be nil.
//
// Returns:
//   - Reader[U]: A new Reader that emits flattened values of type U
//
// Panics:
//   - If mapper is nil
//
// Example usage:
//
//	// Split strings into individual characters
//	stringReader := SliceReader([]string{"abc", "def", "ghi"})
//	charReader := FlatMap(stringReader, func(s string) Reader[rune] {
//	    return SliceReader([]rune(s))
//	})
//
//	// Result stream will emit: 'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i'
//
//	// Expand users to their email addresses
//	userReader := SliceReader([]User{user1, user2, user3})
//	emailReader := FlatMap(userReader, func(user User) Reader[string] {
//	    return SliceReader(user.EmailAddresses)
//	})
//
//	// Process file lines from multiple files
//	fileReader := SliceReader([]string{"file1.txt", "file2.txt"})
//	lineReader := FlatMap(fileReader, func(filename string) Reader[string] {
//	    return FileLineReader(filename) // Hypothetical reader for file lines
//	})
func FlatMap[T, U any](reader Reader[T], mapper func(T) Reader[U]) Reader[U] {
	if mapper == nil {
		panic("nil mapper function")
	}
	return &flatMapReader[T, U]{
		reader: reader,
		mapper: mapper,
	}
}

// distinctReader implements a Reader that filters out duplicate values from
// the source reader, ensuring each unique value appears only once in the output stream.
// It uses a hash set to track seen values for efficient duplicate detection.
//
// This corresponds to Java's stream.distinct() operation and is useful for:
//   - Removing duplicate entries from data streams
//   - Creating unique value sets from potentially repetitive sources
//   - Data deduplication in processing pipelines
//   - Ensuring referential integrity in data processing
//
// Memory considerations:
//   - Maintains a set of all seen values in memory
//   - Memory usage grows with the number of unique values
//   - Not suitable for infinite streams with unbounded unique values
//
// Type parameter:
//   - T: The type of values being deduplicated, must be comparable
type distinctReader[T comparable] struct {
	reader Reader[T]      // Source reader to read values from
	seen   map[T]struct{} // Set of values already seen, using empty struct for memory efficiency
}

// Read returns the next unique value from the source reader, skipping any
// values that have been seen before.
//
// Operation sequence:
//  1. Read a value from the source reader
//  2. If read fails, return the error immediately
//  3. Check if the value has been seen before
//  4. If not seen, add to seen set and return the value
//  5. If already seen, continue to next value (skip duplicate)
//
// Error handling:
//   - Source reader errors are propagated unchanged
//   - No additional errors are introduced by deduplication
//   - EOF from source becomes EOF for distinct stream
//
// Performance characteristics:
//   - O(1) average time complexity for duplicate checking
//   - O(n) space complexity where n is number of unique values
//   - May read many values to find next unique one
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//
// Returns:
//   - T: The next unique value from the source reader
//   - error: Any error from the source reader, or nil on success
func (d *distinctReader[T]) Read(ctx context.Context) (v T, err error) {
	for {
		v, err = d.reader.Read(ctx)
		if err != nil {
			return
		}

		if _, exists := d.seen[v]; !exists {
			d.seen[v] = struct{}{}
			return
		}
	}
}

// Distinct creates a Reader that filters out duplicate values from the source
// reader, ensuring each unique value appears at most once in the output stream.
//
// The distinct operation preserves the order of first occurrence - when a value
// is encountered for the first time, it's included in the output at that position.
// Subsequent occurrences of the same value are filtered out.
//
// Important considerations:
//   - The type T must be comparable (can be used as map key)
//   - Memory usage grows with the number of unique values
//   - Not suitable for infinite streams with unbounded unique values
//   - Best used on finite streams or streams with bounded unique values
//
// Type parameter:
//   - T: The type of values being deduplicated, must be comparable
//
// Parameters:
//   - reader: The source Reader[T] to remove duplicates from
//
// Returns:
//   - Reader[T]: A new Reader that emits unique values only
//
// Example usage:
//
//	// Remove duplicate numbers
//	numbers := SliceReader([]int{1, 2, 3, 2, 4, 1, 5})
//	uniqueNumbers := Distinct(numbers)
//	// Result stream: 1, 2, 3, 4, 5
//
//	// Remove duplicate strings (case-sensitive)
//	words := SliceReader([]string{"apple", "banana", "apple", "cherry", "banana"})
//	uniqueWords := Distinct(words)
//	// Result stream: "apple", "banana", "cherry"
//
//	// Chain with other operations
//	pipeline := Map(
//	    Distinct(
//	        Map(rawDataReader, extractKey), // Extract comparable keys
//	    ),
//	    processUniqueKey, // Process each unique key
//	)
func Distinct[T comparable](reader Reader[T]) Reader[T] {
	return &distinctReader[T]{
		reader: reader,
		seen:   make(map[T]struct{}),
	}
}
