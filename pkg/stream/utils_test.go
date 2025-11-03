package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestOfSliceReader tests creating a reader from a slice
func TestOfSliceReader(t *testing.T) {
	t.Run("EmptySlice", func(t *testing.T) {
		reader := OfSliceReader([]int{})
		ctx := context.Background()

		_, err := reader.Read(ctx)
		if !errors.Is(err, io.EOF) {
			t.Errorf("Expected io.EOF for empty slice, got %v", err)
		}
	})

	t.Run("SingleElement", func(t *testing.T) {
		reader := OfSliceReader([]int{42})
		ctx := context.Background()

		val, err := reader.Read(ctx)
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
		if val != 42 {
			t.Errorf("Expected 42, got %d", val)
		}

		_, err = reader.Read(ctx)
		if !errors.Is(err, io.EOF) {
			t.Errorf("Expected io.EOF after single element, got %v", err)
		}
	})

	t.Run("MultipleElements", func(t *testing.T) {
		expected := []int{1, 2, 3, 4, 5}
		reader := OfSliceReader(expected)
		ctx := context.Background()

		for i, exp := range expected {
			val, err := reader.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %d, got %d", i, exp, val)
			}
		}

		_, err := reader.Read(ctx)
		if !errors.Is(err, io.EOF) {
			t.Errorf("Expected io.EOF after all elements, got %v", err)
		}
	})

	t.Run("StringSlice", func(t *testing.T) {
		words := []string{"hello", "world", "test"}
		reader := OfSliceReader(words)
		ctx := context.Background()

		for i, expected := range words {
			val, err := reader.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != expected {
				t.Errorf("Read %d: expected %s, got %s", i, expected, val)
			}
		}
	})
}

// TestOfChannelReader tests creating a reader from a channel
func TestOfChannelReader(t *testing.T) {
	t.Run("EmptyChannel", func(t *testing.T) {
		ch := make(chan int)
		close(ch)

		reader := OfChannelReader(ch)
		ctx := context.Background()

		_, err := reader.Read(ctx)
		if !errors.Is(err, io.EOF) {
			t.Errorf("Expected io.EOF for closed empty channel, got %v", err)
		}
	})

	t.Run("BufferedChannel", func(t *testing.T) {
		ch := make(chan int, 3)
		ch <- 1
		ch <- 2
		ch <- 3

		reader := OfChannelReader(ch)
		ctx := context.Background()

		expected := []int{1, 2, 3}
		for i, exp := range expected {
			val, err := reader.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %d, got %d", i, exp, val)
			}
		}

		_, err := reader.Read(ctx)
		if !errors.Is(err, io.EOF) {
			t.Errorf("Expected io.EOF after draining channel, got %v", err)
		}
	})

	t.Run("PartiallyFilledChannel", func(t *testing.T) {
		ch := make(chan string, 5)
		ch <- "a"
		ch <- "b"
		// Channel has capacity 5 but only 2 items

		reader := OfChannelReader(ch)
		ctx := context.Background()

		val1, _ := reader.Read(ctx)
		val2, _ := reader.Read(ctx)

		if val1 != "a" || val2 != "b" {
			t.Errorf("Expected 'a' and 'b', got '%s' and '%s'", val1, val2)
		}
	})

	t.Run("ClosedBufferedChannel", func(t *testing.T) {
		ch := make(chan int, 3)
		ch <- 10
		ch <- 20
		ch <- 30
		close(ch)

		reader := OfChannelReader(ch)
		ctx := context.Background()

		expected := []int{10, 20, 30}
		for i, exp := range expected {
			val, err := reader.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %d, got %d", i, exp, val)
			}
		}
	})
}

// TestPipe tests the Pipe function
func TestPipe(t *testing.T) {
	t.Run("BasicPipe", func(t *testing.T) {
		reader, writer := Pipe[int]()
		ctx := context.Background()

		go func() {
			for i := 0; i < 5; i++ {
				writer.Write(ctx, i)
			}
			writer.(io.Closer).Close()
		}()

		for i := 0; i < 5; i++ {
			val, err := reader.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != i {
				t.Errorf("Expected %d, got %d", i, val)
			}
		}

		_, err := reader.Read(ctx)
		if !errors.Is(err, io.EOF) {
			t.Errorf("Expected io.EOF after close, got %v", err)
		}
	})

	t.Run("BufferedPipe", func(t *testing.T) {
		reader, writer := Pipe[string](3)
		ctx := context.Background()

		// Write 3 items (should not block due to buffer)
		writer.Write(ctx, "a")
		writer.Write(ctx, "b")
		writer.Write(ctx, "c")

		val1, _ := reader.Read(ctx)
		val2, _ := reader.Read(ctx)
		val3, _ := reader.Read(ctx)

		if val1 != "a" || val2 != "b" || val3 != "c" {
			t.Errorf("Unexpected values: %s, %s, %s", val1, val2, val3)
		}

		writer.(io.Closer).Close()
	})

	t.Run("ConcurrentPipe", func(t *testing.T) {
		reader, writer := Pipe[int](10)
		ctx := context.Background()

		const numItems = 100
		done := make(chan struct{})

		go func() {
			defer close(done)
			for i := 0; i < numItems; i++ {
				writer.Write(ctx, i)
			}
			writer.(io.Closer).Close()
		}()

		count := 0
		for {
			_, err := reader.Read(ctx)
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("Read failed: %v", err)
			}
			count++
		}

		if count != numItems {
			t.Errorf("Expected %d items, got %d", numItems, count)
		}

		<-done
	})
}

// TestTeeReader tests the TeeReader function
func TestTeeReader(t *testing.T) {
	t.Run("BasicTee", func(t *testing.T) {
		source := OfSliceReader([]int{1, 2, 3})
		dest := NewStream[int](3)
		ctx := context.Background()

		tee := TeeReader(source, dest)

		// Read from tee
		val1, _ := tee.Read(ctx)
		val2, _ := tee.Read(ctx)
		val3, _ := tee.Read(ctx)

		if val1 != 1 || val2 != 2 || val3 != 3 {
			t.Errorf("Unexpected values from tee: %d, %d, %d", val1, val2, val3)
		}

		// Check destination received the same values
		destVal1, _ := dest.Read(ctx)
		destVal2, _ := dest.Read(ctx)
		destVal3, _ := dest.Read(ctx)

		if destVal1 != 1 || destVal2 != 2 || destVal3 != 3 {
			t.Errorf("Unexpected values in destination: %d, %d, %d", destVal1, destVal2, destVal3)
		}
	})

	t.Run("TeeWithEOF", func(t *testing.T) {
		source := OfSliceReader([]string{"hello"})
		dest := NewStream[string](1)
		ctx := context.Background()

		tee := TeeReader(source, dest)

		val, err := tee.Read(ctx)
		if err != nil || val != "hello" {
			t.Errorf("Expected 'hello', got %v with error %v", val, err)
		}

		_, err = tee.Read(ctx)
		if !errors.Is(err, io.EOF) {
			t.Errorf("Expected io.EOF, got %v", err)
		}
	})

	t.Run("TeeLogging", func(t *testing.T) {
		source := OfSliceReader([]int{10, 20, 30, 40, 50})
		logStream := NewStream[int](5)
		ctx := context.Background()

		tee := TeeReader(source, logStream)

		var logged []int
		var wg sync.WaitGroup
		wg.Add(1)

		// Logger goroutine
		go func() {
			defer wg.Done()
			for {
				val, err := logStream.Read(ctx)
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Errorf("Log read failed: %v", err)
					return
				}
				logged = append(logged, val)
			}
		}()

		// Main processing
		for {
			val, err := tee.Read(ctx)
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("Tee read failed: %v", err)
			}
			// Process val (just reading in this test)
			_ = val
		}

		logStream.Close()
		wg.Wait()

		expected := []int{10, 20, 30, 40, 50}
		if len(logged) != len(expected) {
			t.Errorf("Expected %d logged items, got %d", len(expected), len(logged))
		}
		for i, v := range logged {
			if v != expected[i] {
				t.Errorf("Logged item %d: expected %d, got %d", i, expected[i], v)
			}
		}
	})
}

// TestMultiReader tests the MultiReader function
func TestMultiReader(t *testing.T) {
	t.Run("TwoReaders", func(t *testing.T) {
		reader1 := OfSliceReader([]int{1, 2, 3})
		reader2 := OfSliceReader([]int{4, 5, 6})

		multi := MultiReader(reader1, reader2)
		ctx := context.Background()

		expected := []int{1, 2, 3, 4, 5, 6}
		for i, exp := range expected {
			val, err := multi.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %d, got %d", i, exp, val)
			}
		}

		_, err := multi.Read(ctx)
		if !errors.Is(err, io.EOF) {
			t.Errorf("Expected io.EOF, got %v", err)
		}
	})

	t.Run("ThreeReaders", func(t *testing.T) {
		r1 := OfSliceReader([]string{"a", "b"})
		r2 := OfSliceReader([]string{"c", "d"})
		r3 := OfSliceReader([]string{"e", "f"})

		multi := MultiReader(r1, r2, r3)
		ctx := context.Background()

		expected := []string{"a", "b", "c", "d", "e", "f"}
		var result []string

		for {
			val, err := multi.Read(ctx)
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("Read failed: %v", err)
			}
			result = append(result, val)
		}

		if len(result) != len(expected) {
			t.Errorf("Expected %d items, got %d", len(expected), len(result))
		}

		for i, v := range result {
			if v != expected[i] {
				t.Errorf("Item %d: expected %s, got %s", i, expected[i], v)
			}
		}
	})

	t.Run("EmptyReader", func(t *testing.T) {
		r1 := OfSliceReader([]int{1, 2})
		r2 := OfSliceReader([]int{})
		r3 := OfSliceReader([]int{3, 4})

		multi := MultiReader(r1, r2, r3)
		ctx := context.Background()

		expected := []int{1, 2, 3, 4}
		for i, exp := range expected {
			val, err := multi.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %d, got %d", i, exp, val)
			}
		}
	})

	t.Run("NestedMultiReader", func(t *testing.T) {
		r1 := OfSliceReader([]int{1, 2})
		r2 := OfSliceReader([]int{3, 4})
		r3 := OfSliceReader([]int{5, 6})

		multi1 := MultiReader(r1, r2)
		multi2 := MultiReader(multi1, r3)

		ctx := context.Background()
		expected := []int{1, 2, 3, 4, 5, 6}

		for i, exp := range expected {
			val, err := multi2.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %d, got %d", i, exp, val)
			}
		}
	})
}

// TestMultiWriter tests the MultiWriter function
func TestMultiWriter(t *testing.T) {
	t.Run("TwoWriters", func(t *testing.T) {
		stream1 := NewStream[int](3)
		stream2 := NewStream[int](3)

		multi := MultiWriter[int](stream1, stream2)
		ctx := context.Background()

		multi.Write(ctx, 1)
		multi.Write(ctx, 2)
		multi.Write(ctx, 3)

		// Check first stream
		val1, _ := stream1.Read(ctx)
		val2, _ := stream2.Read(ctx)

		if val1 != 1 || val2 != 1 {
			t.Errorf("Expected both to be 1, got %d and %d", val1, val2)
		}
	})

	t.Run("ThreeWriters", func(t *testing.T) {
		s1 := NewStream[string](2)
		s2 := NewStream[string](2)
		s3 := NewStream[string](2)

		multi := MultiWriter(s1, s2, s3)
		ctx := context.Background()

		multi.Write(ctx, "test")

		v1, _ := s1.Read(ctx)
		v2, _ := s2.Read(ctx)
		v3, _ := s3.Read(ctx)

		if v1 != "test" || v2 != "test" || v3 != "test" {
			t.Errorf("Expected all to be 'test', got %s, %s, %s", v1, v2, v3)
		}
	})

	t.Run("OneWriterClosed", func(t *testing.T) {
		s1 := NewStream[int](1)
		s2 := NewStream[int](1)

		s1.Close() // Close first stream

		multi := MultiWriter(s1, s2)
		ctx := context.Background()

		err := multi.Write(ctx, 42)
		if !errors.Is(err, ErrStreamClosed) {
			t.Errorf("Expected ErrStreamClosed, got %v", err)
		}
	})

	t.Run("BroadcastMultipleValues", func(t *testing.T) {
		streams := make([]*stream[int], 3)
		writers := make([]Writer[int], 3)

		for i := range streams {
			streams[i] = NewStream[int](5).(*stream[int])
			writers[i] = streams[i]
		}

		multi := MultiWriter(writers...)
		ctx := context.Background()

		values := []int{1, 2, 3, 4, 5}
		for _, v := range values {
			multi.Write(ctx, v)
		}

		// Verify all streams received all values
		for i, s := range streams {
			for j, expected := range values {
				val, err := s.Read(ctx)
				if err != nil {
					t.Fatalf("Stream %d, read %d failed: %v", i, j, err)
				}
				if val != expected {
					t.Errorf("Stream %d, read %d: expected %d, got %d", i, j, expected, val)
				}
			}
		}
	})
}

// TestMap tests the Map function
func TestMap(t *testing.T) {
	t.Run("IntToString", func(t *testing.T) {
		source := OfSliceReader([]int{1, 2, 3, 4, 5})
		mapped := Map(source, func(i int) string {
			return fmt.Sprintf("num_%d", i)
		})

		ctx := context.Background()
		expected := []string{"num_1", "num_2", "num_3", "num_4", "num_5"}

		for i, exp := range expected {
			val, err := mapped.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %s, got %s", i, exp, val)
			}
		}
	})

	t.Run("DoubleNumbers", func(t *testing.T) {
		source := OfSliceReader([]int{1, 2, 3})
		doubled := Map(source, func(i int) int {
			return i * 2
		})

		ctx := context.Background()
		expected := []int{2, 4, 6}

		for i, exp := range expected {
			val, err := doubled.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %d, got %d", i, exp, val)
			}
		}
	})

	t.Run("StringToUpper", func(t *testing.T) {
		source := OfSliceReader([]string{"hello", "world"})
		upper := Map(source, strings.ToUpper)

		ctx := context.Background()
		expected := []string{"HELLO", "WORLD"}

		for i, exp := range expected {
			val, err := upper.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %s, got %s", i, exp, val)
			}
		}
	})

	t.Run("ChainedMaps", func(t *testing.T) {
		source := OfSliceReader([]int{1, 2, 3})

		// Chain multiple transformations
		result := Map(
			Map(
				Map(source, func(i int) int { return i * 2 }),
				func(i int) int { return i + 10 },
			),
			func(i int) string { return fmt.Sprintf("result_%d", i) },
		)

		ctx := context.Background()
		expected := []string{"result_12", "result_14", "result_16"}

		for i, exp := range expected {
			val, err := result.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %s, got %s", i, exp, val)
			}
		}
	})

	t.Run("NilMapper", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for nil mapper")
			}
		}()

		source := OfSliceReader([]int{1, 2, 3})
		Map[int, int](source, nil)
	})

	t.Run("EmptyStream", func(t *testing.T) {
		source := OfSliceReader([]int{})
		mapped := Map(source, func(i int) string {
			return fmt.Sprintf("%d", i)
		})

		ctx := context.Background()
		_, err := mapped.Read(ctx)
		if !errors.Is(err, io.EOF) {
			t.Errorf("Expected io.EOF for empty stream, got %v", err)
		}
	})
}

// TestFilter tests the Filter function
func TestFilter(t *testing.T) {
	t.Run("FilterPositive", func(t *testing.T) {
		source := OfSliceReader([]int{-2, -1, 0, 1, 2, 3})
		positive := Filter(source, func(i int) bool {
			return i > 0
		})

		ctx := context.Background()
		expected := []int{1, 2, 3}

		for i, exp := range expected {
			val, err := positive.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %d, got %d", i, exp, val)
			}
		}

		_, err := positive.Read(ctx)
		if !errors.Is(err, io.EOF) {
			t.Errorf("Expected io.EOF, got %v", err)
		}
	})

	t.Run("FilterEven", func(t *testing.T) {
		source := OfSliceReader([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
		even := Filter(source, func(i int) bool {
			return i%2 == 0
		})

		ctx := context.Background()
		expected := []int{2, 4, 6, 8, 10}

		for i, exp := range expected {
			val, err := even.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %d, got %d", i, exp, val)
			}
		}
	})

	t.Run("FilterStrings", func(t *testing.T) {
		source := OfSliceReader([]string{"apple", "banana", "apricot", "cherry", "avocado"})
		startsWithA := Filter(source, func(s string) bool {
			return strings.HasPrefix(s, "a")
		})

		ctx := context.Background()
		expected := []string{"apple", "apricot", "avocado"}

		for i, exp := range expected {
			val, err := startsWithA.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %s, got %s", i, exp, val)
			}
		}
	})

	t.Run("FilterNone", func(t *testing.T) {
		source := OfSliceReader([]int{1, 2, 3})
		filtered := Filter(source, func(i int) bool {
			return i > 10 // No values match
		})

		ctx := context.Background()
		_, err := filtered.Read(ctx)
		if !errors.Is(err, io.EOF) {
			t.Errorf("Expected io.EOF when no values match, got %v", err)
		}
	})

	t.Run("FilterAll", func(t *testing.T) {
		source := OfSliceReader([]int{1, 2, 3, 4, 5})
		filtered := Filter(source, func(i int) bool {
			return true // All values match
		})

		ctx := context.Background()
		expected := []int{1, 2, 3, 4, 5}

		for i, exp := range expected {
			val, err := filtered.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %d, got %d", i, exp, val)
			}
		}
	})

	t.Run("NilPredicate", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for nil predicate")
			}
		}()

		source := OfSliceReader([]int{1, 2, 3})
		Filter(source, nil)
	})

	t.Run("CombineFilterAndMap", func(t *testing.T) {
		source := OfSliceReader([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})

		// Filter even numbers, then double them
		result := Map(
			Filter(source, func(i int) bool { return i%2 == 0 }),
			func(i int) int { return i * 2 },
		)

		ctx := context.Background()
		expected := []int{4, 8, 12, 16, 20}

		for i, exp := range expected {
			val, err := result.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %d, got %d", i, exp, val)
			}
		}
	})
}

// TestFlatMap tests the FlatMap function
func TestFlatMap(t *testing.T) {
	t.Run("StringToChars", func(t *testing.T) {
		source := OfSliceReader([]string{"ab", "cd", "ef"})
		chars := FlatMap(source, func(s string) Reader[rune] {
			return OfSliceReader([]rune(s))
		})

		ctx := context.Background()
		expected := []rune{'a', 'b', 'c', 'd', 'e', 'f'}

		for i, exp := range expected {
			val, err := chars.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %c, got %c", i, exp, val)
			}
		}

		_, err := chars.Read(ctx)
		if !errors.Is(err, io.EOF) {
			t.Errorf("Expected io.EOF, got %v", err)
		}
	})

	t.Run("ExpandNumbers", func(t *testing.T) {
		source := OfSliceReader([]int{1, 2, 3})
		expanded := FlatMap(source, func(i int) Reader[int] {
			// Each number expands to [n, n*10, n*100]
			return OfSliceReader([]int{i, i * 10, i * 100})
		})

		ctx := context.Background()
		expected := []int{1, 10, 100, 2, 20, 200, 3, 30, 300}

		for i, exp := range expected {
			val, err := expanded.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %d, got %d", i, exp, val)
			}
		}
	})

	t.Run("EmptyExpansions", func(t *testing.T) {
		source := OfSliceReader([]int{1, 2, 3})
		flattened := FlatMap(source, func(i int) Reader[int] {
			if i == 2 {
				return OfSliceReader([]int{}) // Empty expansion for 2
			}
			return OfSliceReader([]int{i})
		})

		ctx := context.Background()
		expected := []int{1, 3} // 2 is skipped due to empty expansion

		for i, exp := range expected {
			val, err := flattened.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %d, got %d", i, exp, val)
			}
		}
	})

	t.Run("NilMapper", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for nil mapper")
			}
		}()

		source := OfSliceReader([]int{1, 2, 3})
		FlatMap[int, int](source, nil)
	})

	t.Run("NestedLists", func(t *testing.T) {
		// Simulate nested lists
		type Group struct {
			items []int
		}

		groups := []Group{
			{items: []int{1, 2}},
			{items: []int{3, 4, 5}},
			{items: []int{6}},
		}

		source := OfSliceReader(groups)
		flattened := FlatMap(source, func(g Group) Reader[int] {
			return OfSliceReader(g.items)
		})

		ctx := context.Background()
		expected := []int{1, 2, 3, 4, 5, 6}

		for i, exp := range expected {
			val, err := flattened.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %d, got %d", i, exp, val)
			}
		}
	})

	t.Run("ChainFlatMapAndMap", func(t *testing.T) {
		source := OfSliceReader([]string{"a", "b"})

		// FlatMap to chars, then Map to uppercase
		result := Map(
			FlatMap(source, func(s string) Reader[rune] {
				return OfSliceReader([]rune(s + s)) // Double each char
			}),
			func(r rune) string {
				return strings.ToUpper(string(r))
			},
		)

		ctx := context.Background()
		expected := []string{"A", "A", "B", "B"}

		for i, exp := range expected {
			val, err := result.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %s, got %s", i, exp, val)
			}
		}
	})
}

// TestDistinct tests the Distinct function
func TestDistinct(t *testing.T) {
	t.Run("RemoveDuplicateInts", func(t *testing.T) {
		source := OfSliceReader([]int{1, 2, 3, 2, 4, 1, 5, 3})
		distinct := Distinct(source)

		ctx := context.Background()
		expected := []int{1, 2, 3, 4, 5}

		for i, exp := range expected {
			val, err := distinct.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %d, got %d", i, exp, val)
			}
		}

		_, err := distinct.Read(ctx)
		if !errors.Is(err, io.EOF) {
			t.Errorf("Expected io.EOF, got %v", err)
		}
	})

	t.Run("RemoveDuplicateStrings", func(t *testing.T) {
		source := OfSliceReader([]string{"apple", "banana", "apple", "cherry", "banana", "date"})
		distinct := Distinct(source)

		ctx := context.Background()
		expected := []string{"apple", "banana", "cherry", "date"}

		for i, exp := range expected {
			val, err := distinct.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %s, got %s", i, exp, val)
			}
		}
	})

	t.Run("NoDuplicates", func(t *testing.T) {
		source := OfSliceReader([]int{1, 2, 3, 4, 5})
		distinct := Distinct(source)

		ctx := context.Background()
		expected := []int{1, 2, 3, 4, 5}

		for i, exp := range expected {
			val, err := distinct.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %d, got %d", i, exp, val)
			}
		}
	})

	t.Run("AllDuplicates", func(t *testing.T) {
		source := OfSliceReader([]int{5, 5, 5, 5, 5})
		distinct := Distinct(source)

		ctx := context.Background()
		val, err := distinct.Read(ctx)
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
		if val != 5 {
			t.Errorf("Expected 5, got %d", val)
		}

		_, err = distinct.Read(ctx)
		if !errors.Is(err, io.EOF) {
			t.Errorf("Expected io.EOF after single distinct value, got %v", err)
		}
	})

	t.Run("EmptyStream", func(t *testing.T) {
		source := OfSliceReader([]int{})
		distinct := Distinct(source)

		ctx := context.Background()
		_, err := distinct.Read(ctx)
		if !errors.Is(err, io.EOF) {
			t.Errorf("Expected io.EOF for empty stream, got %v", err)
		}
	})

	t.Run("CombineWithFilter", func(t *testing.T) {
		source := OfSliceReader([]int{1, 2, 3, 2, 4, 1, 5, 3, 6, 4})

		// Filter even numbers, then remove duplicates
		result := Distinct(
			Filter(source, func(i int) bool { return i%2 == 0 }),
		)

		ctx := context.Background()
		expected := []int{2, 4, 6}

		for i, exp := range expected {
			val, err := result.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %d, got %d", i, exp, val)
			}
		}
	})
}

// TestComplexPipelines tests complex combinations of operators
func TestComplexPipelines(t *testing.T) {
	t.Run("FilterMapDistinct", func(t *testing.T) {
		// Complex pipeline: Filter -> Map -> Distinct
		source := OfSliceReader([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})

		pipeline := Distinct(
			Map(
				Filter(source, func(i int) bool { return i%2 == 0 }), // Even numbers
				func(i int) int { return i / 2 },                     // Divide by 2
			),
		)

		ctx := context.Background()
		expected := []int{1, 2, 3, 4, 5}

		for i, exp := range expected {
			val, err := pipeline.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %d, got %d", i, exp, val)
			}
		}
	})

	t.Run("FlatMapFilterMap", func(t *testing.T) {
		// FlatMap -> Filter -> Map
		source := OfSliceReader([]string{"abc", "def", "ghi"})

		pipeline := Map(
			Filter(
				FlatMap(source, func(s string) Reader[rune] {
					return OfSliceReader([]rune(s))
				}),
				func(r rune) bool { return r != 'd' && r != 'e' }, // Exclude 'd' and 'e'
			),
			func(r rune) string { return strings.ToUpper(string(r)) },
		)

		ctx := context.Background()
		expected := []string{"A", "B", "C", "F", "G", "H", "I"}

		for i, exp := range expected {
			val, err := pipeline.Read(ctx)
			if err != nil {
				t.Fatalf("Read %d failed: %v", i, err)
			}
			if val != exp {
				t.Errorf("Read %d: expected %s, got %s", i, exp, val)
			}
		}
	})

	t.Run("MultiReaderTeeFilter", func(t *testing.T) {
		// Multi-source pipeline with tee and filter
		r1 := OfSliceReader([]int{1, 2, 3})
		r2 := OfSliceReader([]int{4, 5, 6})

		combined := MultiReader(r1, r2)
		logStream := NewStream[int](6)

		tee := TeeReader(combined, logStream)
		filtered := Filter(tee, func(i int) bool { return i%2 == 1 })

		ctx := context.Background()
		expected := []int{1, 3, 5}
		var filteredR []int
		for {
			val, err := filtered.Read(ctx)
			if err != nil {
				if err == io.EOF {
					break
				}
				t.Fatalf("Read  failed: %v", err)
			}
			filteredR = append(filteredR, val)
		}
		if len(filteredR) != len(expected) {
			t.Errorf("Expected %d items, got %d", len(expected), len(filteredR))
		}
		for i, exp := range expected {
			if filteredR[i] != exp {
				t.Errorf("Read %d: expected %d, got %d", i, exp, filteredR[i])
			}
		}

		// Verify all values were logged
		logStream.Close()
		var logged []int
		for {
			val, err := logStream.Read(ctx)
			if err == io.EOF {
				break
			}
			logged = append(logged, val)
		}

		expectedLog := []int{1, 2, 3, 4, 5, 6}
		if len(logged) != len(expectedLog) {
			t.Errorf("Expected %d logged items, got %d", len(expectedLog), len(logged))
		}
	})
}

// TestConcurrentOperators tests concurrent safety of operators
func TestConcurrentOperators(t *testing.T) {
	t.Run("ConcurrentMap", func(t *testing.T) {
		const numReaders = 5
		const numItems = 100

		reader, writer := Pipe[int](10)
		ctx := context.Background()

		// Writer goroutine
		go func() {
			defer writer.(io.Closer).Close()
			for i := 0; i < numItems; i++ {
				writer.Write(ctx, i)
			}
		}()

		// Create mapped reader
		mapped := Map(reader, func(i int) int { return i * 2 })

		// Multiple concurrent readers
		var wg sync.WaitGroup
		counts := make([]atomic.Int32, numReaders)

		for i := 0; i < numReaders; i++ {
			wg.Add(1)
			idx := i
			go func() {
				defer wg.Done()
				for {
					_, err := mapped.Read(ctx)
					if err == io.EOF {
						return
					}
					if err != nil {
						t.Errorf("Reader %d: error: %v", idx, err)
						return
					}
					counts[idx].Add(1)
				}
			}()
		}

		wg.Wait()

		// Verify total reads
		var total int32
		for i := range counts {
			total += counts[i].Load()
		}

		if total != numItems {
			t.Errorf("Expected %d total reads, got %d", numItems, total)
		}
	})

	t.Run("ConcurrentFilter", func(t *testing.T) {
		reader, writer := Pipe[int](20)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		go func() {
			defer writer.(io.Closer).Close()
			for i := 0; i < 100; i++ {
				writer.Write(ctx, i)
			}
		}()

		filtered := Filter(reader, func(i int) bool { return i%2 == 0 })

		count := 0
		for {
			_, err := filtered.Read(ctx)
			if err == io.EOF {
				break
			}
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					t.Fatal("Test timed out")
				}
				t.Fatalf("Read error: %v", err)
			}
			count++
		}

		if count != 50 {
			t.Errorf("Expected 50 even numbers, got %d", count)
		}
	})
}

// Benchmark tests
func BenchmarkMap(b *testing.B) {
	source := OfSliceReader(make([]int, 1000))
	mapped := Map(source, func(i int) int { return i * 2 })
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mapped.Read(ctx)
	}
}

func BenchmarkFilter(b *testing.B) {
	source := OfSliceReader(make([]int, 1000))
	filtered := Filter(source, func(i int) bool { return i%2 == 0 })
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filtered.Read(ctx)
	}
}

func BenchmarkDistinct(b *testing.B) {
	data := make([]int, 1000)
	for i := range data {
		data[i] = i % 100 // Lots of duplicates
	}
	source := OfSliceReader(data)
	distinct := Distinct(source)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		distinct.Read(ctx)
	}
}
