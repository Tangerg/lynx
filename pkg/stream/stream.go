package stream

import (
	"context"
	"errors"
	"io"
	"sync"

	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

// ErrStreamClosed is returned by [Writer.Write] and [io.Closer.Close]
// when the stream is already closed.
var ErrStreamClosed = errors.New("stream: already closed")

// Reader reads values of type T from a stream.
type Reader[T any] interface {
	// Read blocks until a value is available, ctx is done, or the
	// stream is closed. It returns io.EOF when the stream is closed
	// and drained.
	Read(ctx context.Context) (T, error)
}

// Writer writes values of type T to a stream.
type Writer[T any] interface {
	// Write blocks until the value is accepted, ctx is done, or the
	// stream is closed. It returns [ErrStreamClosed] for writes to a
	// closed stream.
	Write(ctx context.Context, v T) error
}

// Stream is a bidirectional, closeable channel-backed stream.
type Stream[T any] interface {
	Reader[T]
	Writer[T]
	io.Closer
}

// stream is the channel-based implementation of [Stream]. The mutex
// makes Close wait for in-flight writes so closing the channel never
// races with a send.
type stream[T any] struct {
	value  chan T
	mu     sync.RWMutex
	closed bool
}

// Read implements [Reader.Read].
func (s *stream[T]) Read(ctx context.Context) (v T, err error) {
	select {
	case <-ctx.Done():
		return v, ctx.Err()
	case val, ok := <-s.value:
		if !ok {
			return v, io.EOF
		}
		return val, nil
	}
}

// Write implements [Writer.Write].
func (s *stream[T]) Write(ctx context.Context, v T) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return ErrStreamClosed
	}
	select {
	case <-ctx.Done():
		s.mu.RUnlock()
		return ctx.Err()
	case s.value <- v:
		s.mu.RUnlock()
		return nil
	}
}

// Close marks the stream closed and closes the underlying channel.
// Subsequent calls return [ErrStreamClosed]. In-flight writes finish
// before the channel is closed; subsequent reads drain buffered data
// then observe io.EOF.
func (s *stream[T]) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrStreamClosed
	}
	s.closed = true
	close(s.value)
	return nil
}

// IsClosed reports whether [Close] has been called. Callers should
// still handle [ErrStreamClosed] from Write rather than relying on
// this snapshot.
func (s *stream[T]) IsClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}

// NewStream returns a new [Stream] with optional buffer size. Only the
// first size is used; negative values become 0 (unbuffered).
//
// Example:
//
//	s := stream.NewStream[int](32)
//	defer s.Close()
//	go producer(s)
//	for {
//	    v, err := s.Read(ctx)
//	    if errors.Is(err, io.EOF) {
//	        return
//	    }
//	    handle(v)
//	}
func NewStream[T any](sizes ...int) Stream[T] {
	size, _ := pkgSlices.First(sizes)
	if size < 0 {
		size = 0
	}
	return &stream[T]{value: make(chan T, size)}
}
