package stream

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewStream(t *testing.T) {
	tests := []struct {
		name         string
		sizes        []int
		wantBuffered bool
		wantCapacity int
	}{
		{
			name:         "unbuffered stream - no args",
			sizes:        nil,
			wantBuffered: false,
			wantCapacity: 0,
		},
		{
			name:         "unbuffered stream - zero size",
			sizes:        []int{0},
			wantBuffered: false,
			wantCapacity: 0,
		},
		{
			name:         "unbuffered stream - negative size",
			sizes:        []int{-1},
			wantBuffered: false,
			wantCapacity: 0,
		},
		{
			name:         "buffered stream - size 1",
			sizes:        []int{1},
			wantBuffered: true,
			wantCapacity: 1,
		},
		{
			name:         "buffered stream - size 10",
			sizes:        []int{10},
			wantBuffered: true,
			wantCapacity: 10,
		},
		{
			name:         "multiple sizes - use first",
			sizes:        []int{5, 10, 15},
			wantBuffered: true,
			wantCapacity: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStream[int](tt.sizes...)
			impl := s.(*stream[int])

			gotCapacity := cap(impl.value)
			if gotCapacity != tt.wantCapacity {
				t.Errorf("cap(value) = %d, want %d", gotCapacity, tt.wantCapacity)
			}

			if err := s.Close(); err != nil {
				t.Errorf("Close() error = %v", err)
			}
		})
	}
}

func TestStreamWriteRead(t *testing.T) {
	tests := []struct {
		name   string
		buffer int
		values []int
	}{
		{
			name:   "unbuffered single value",
			buffer: 0,
			values: []int{42},
		},
		{
			name:   "unbuffered multiple values",
			buffer: 0,
			values: []int{1, 2, 3, 4, 5},
		},
		{
			name:   "buffered single value",
			buffer: 10,
			values: []int{42},
		},
		{
			name:   "buffered multiple values",
			buffer: 10,
			values: []int{1, 2, 3, 4, 5},
		},
		{
			name:   "buffered exact capacity",
			buffer: 3,
			values: []int{1, 2, 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStream[int](tt.buffer)
			ctx := context.Background()

			var wg sync.WaitGroup
			wg.Add(2)

			go func() {
				defer wg.Done()
				for _, v := range tt.values {
					if err := s.Write(ctx, v); err != nil {
						t.Errorf("Write(%d) error = %v", v, err)
						return
					}
				}
				s.Close()
			}()

			go func() {
				defer wg.Done()
				for _, want := range tt.values {
					got, err := s.Read(ctx)
					if err != nil {
						t.Errorf("Read() error = %v", err)
						return
					}
					if got != want {
						t.Errorf("Read() = %d, want %d", got, want)
					}
				}
			}()

			wg.Wait()
		})
	}
}

func TestStreamReadContextCancel(t *testing.T) {
	s := NewStream[int](0)
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.Read(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Read() error = %v, want context.Canceled", err)
	}
}

func TestStreamReadContextTimeout(t *testing.T) {
	s := NewStream[int](0)
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := s.Read(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Read() error = %v, want context.DeadlineExceeded", err)
	}
}

func TestStreamReadFromClosedStream(t *testing.T) {
	s := NewStream[int](0)
	s.Close()

	ctx := context.Background()
	_, err := s.Read(ctx)
	if !errors.Is(err, io.EOF) {
		t.Errorf("Read() error = %v, want io.EOF", err)
	}
}

func TestStreamReadBufferedAfterClose(t *testing.T) {
	s := NewStream[int](3)
	ctx := context.Background()

	s.Write(ctx, 1)
	s.Write(ctx, 2)
	s.Write(ctx, 3)
	s.Close()

	for i := 1; i <= 3; i++ {
		got, err := s.Read(ctx)
		if err != nil {
			t.Fatalf("Read() error = %v, want nil", err)
		}
		if got != i {
			t.Errorf("Read() = %d, want %d", got, i)
		}
	}

	_, err := s.Read(ctx)
	if !errors.Is(err, io.EOF) {
		t.Errorf("Read() after consuming buffer error = %v, want io.EOF", err)
	}
}

func TestStreamWriteContextCancel(t *testing.T) {
	s := NewStream[int](0)
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.Write(ctx, 42)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Write() error = %v, want context.Canceled", err)
	}
}

func TestStreamWriteContextTimeout(t *testing.T) {
	s := NewStream[int](0)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := s.Write(ctx, 42)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Write() error = %v, want context.DeadlineExceeded", err)
	}
	s.Close()
}

func TestStreamWriteToClosedStream(t *testing.T) {
	s := NewStream[int](0)
	s.Close()

	ctx := context.Background()
	err := s.Write(ctx, 42)
	if !errors.Is(err, ErrStreamClosed) {
		t.Errorf("Write() error = %v, want ErrStreamClosed", err)
	}
}

func TestStreamWriteBlocksOnFullBuffer(t *testing.T) {
	s := NewStream[int](2)
	defer s.Close()

	ctx := context.Background()

	s.Write(ctx, 1)
	s.Write(ctx, 2)

	writeDone := make(chan bool)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		err := s.Write(ctx, 3)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Write() error = %v, want context.DeadlineExceeded", err)
		}
		close(writeDone)
	}()

	<-writeDone
}

func TestStreamClose(t *testing.T) {
	s := NewStream[int](0)

	if err := s.Close(); err != nil {
		t.Errorf("first Close() error = %v, want nil", err)
	}

	if err := s.Close(); !errors.Is(err, ErrStreamClosed) {
		t.Errorf("second Close() error = %v, want ErrStreamClosed", err)
	}
}

func TestStreamCloseConcurrent(t *testing.T) {
	s := NewStream[int](0)

	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errs := make([]error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			errs[idx] = s.Close()
		}(i)
	}

	wg.Wait()

	nilCount := 0
	closedCount := 0

	for _, err := range errs {
		if err == nil {
			nilCount++
		} else if errors.Is(err, ErrStreamClosed) {
			closedCount++
		} else {
			t.Errorf("unexpected error: %v", err)
		}
	}

	if nilCount < 1 {
		t.Errorf("expected at least one nil error, got %d", nilCount)
	}
}

func TestStreamConcurrentWriteRead(t *testing.T) {
	s := NewStream[int](5)
	ctx := context.Background()

	const numValues = 100
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < numValues; i++ {
			if err := s.Write(ctx, i); err != nil {
				t.Errorf("Write(%d) error = %v", i, err)
				return
			}
		}
		s.Close()
	}()

	received := make([]int, 0, numValues)
	var mu sync.Mutex

	go func() {
		defer wg.Done()
		for {
			val, err := s.Read(ctx)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Errorf("Read() error = %v", err)
				return
			}
			mu.Lock()
			received = append(received, val)
			mu.Unlock()
		}
	}()

	wg.Wait()

	if len(received) != numValues {
		t.Errorf("received %d values, want %d", len(received), numValues)
	}

	for i, v := range received {
		if v != i {
			t.Errorf("received[%d] = %d, want %d", i, v, i)
		}
	}
}

func TestStreamMultipleReaders(t *testing.T) {
	s := NewStream[int](10)
	ctx := context.Background()

	const numValues = 100
	const numReaders = 3

	var wg sync.WaitGroup
	wg.Add(numReaders + 1)

	go func() {
		defer wg.Done()
		for i := 0; i < numValues; i++ {
			if err := s.Write(ctx, i); err != nil {
				t.Errorf("Write(%d) error = %v", i, err)
				return
			}
		}
		s.Close()
	}()

	var totalRead atomic.Int64

	for r := 0; r < numReaders; r++ {
		go func(readerID int) {
			defer wg.Done()
			count := 0
			for {
				_, err := s.Read(ctx)
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Errorf("Reader %d: Read() error = %v", readerID, err)
					return
				}
				count++
			}
			totalRead.Add(int64(count))
		}(r)
	}

	wg.Wait()

	if totalRead.Load() != numValues {
		t.Errorf("total read = %d, want %d", totalRead.Load(), numValues)
	}
}

func TestStreamGenericTypes(t *testing.T) {
	t.Run("string type", func(t *testing.T) {
		s := NewStream[string](1)
		defer s.Close()

		ctx := context.Background()
		want := "hello"

		if err := s.Write(ctx, want); err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		got, err := s.Read(ctx)
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}

		if got != want {
			t.Errorf("Read() = %q, want %q", got, want)
		}
	})

	t.Run("struct type", func(t *testing.T) {
		type data struct {
			ID   int
			Name string
		}

		s := NewStream[data](1)
		defer s.Close()

		ctx := context.Background()
		want := data{ID: 1, Name: "test"}

		if err := s.Write(ctx, want); err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		got, err := s.Read(ctx)
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}

		if got != want {
			t.Errorf("Read() = %+v, want %+v", got, want)
		}
	})

	t.Run("pointer type", func(t *testing.T) {
		s := NewStream[*int](1)
		defer s.Close()

		ctx := context.Background()
		val := 42
		want := &val

		if err := s.Write(ctx, want); err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		got, err := s.Read(ctx)
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}

		if got != want {
			t.Errorf("Read() = %p, want %p", got, want)
		}
	})
}

func TestStreamZeroValueOnError(t *testing.T) {
	s := NewStream[int](0)
	s.Close()

	ctx := context.Background()
	got, err := s.Read(ctx)

	if err == nil {
		t.Fatal("Read() error = nil, want error")
	}

	if got != 0 {
		t.Errorf("Read() returned %d, want zero value (0)", got)
	}
}

func TestStreamWriteAfterPartialClose(t *testing.T) {
	s := NewStream[int](2)
	ctx := context.Background()

	s.Write(ctx, 1)

	go func() {
		time.Sleep(10 * time.Millisecond)
		s.Close()
	}()

	time.Sleep(5 * time.Millisecond)

	err := s.Write(ctx, 2)
	if err != nil && !errors.Is(err, ErrStreamClosed) {
		t.Errorf("Write() error = %v", err)
	}
}

func BenchmarkStreamUnbuffered(b *testing.B) {
	s := NewStream[int](0)
	defer s.Close()

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			go s.Write(ctx, 1)
			s.Read(ctx)
		}
	})
}

func BenchmarkStreamBuffered(b *testing.B) {
	s := NewStream[int](100)
	defer s.Close()

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			go s.Write(ctx, 1)
			s.Read(ctx)
		}
	})
}

func BenchmarkStreamWriteOnly(b *testing.B) {
	s := NewStream[int](1000)
	defer s.Close()

	ctx := context.Background()

	go func() {
		for {
			if _, err := s.Read(ctx); err != nil {
				return
			}
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Write(ctx, i)
	}
}

func BenchmarkStreamReadOnly(b *testing.B) {
	s := NewStream[int](1000)
	defer s.Close()

	ctx := context.Background()

	go func() {
		for i := 0; i < b.N; i++ {
			s.Write(ctx, i)
		}
		s.Close()
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Read(ctx)
	}
}
