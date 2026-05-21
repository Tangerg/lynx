package stream

import (
	"context"
	"io"
	"slices"
)

// OfSliceReader returns a Reader that emits the elements of items in
// order, then io.EOF. The stream is buffered to avoid blocking on the
// initial loads.
func OfSliceReader[T any](items []T) Reader[T] {
	s := NewStream[T](len(items))
	ctx := context.Background()
	for _, v := range items {
		_ = s.Write(ctx, v)
	}
	_ = s.Close()
	return s
}

// OfChannelReader drains every value immediately available on c
// (without blocking) and returns a Reader over the snapshot. Values
// arriving after the call are not observed.
func OfChannelReader[T any](c <-chan T) Reader[T] {
	out := make([]T, 0)
	for {
		select {
		case v, ok := <-c:
			if !ok {
				return OfSliceReader(out)
			}
			out = append(out, v)
		default:
			return OfSliceReader(out)
		}
	}
}

// Pipe returns a connected (Reader, Writer) sharing the same stream.
// Closing the writer's underlying [Stream] signals io.EOF to the
// reader.
//
// Example:
//
//	r, w := stream.Pipe[int]()
//	go func() { defer w.(io.Closer).Close(); w.Write(ctx, 1) }()
//	v, _ := r.Read(ctx)
func Pipe[T any](sizes ...int) (Reader[T], Writer[T]) {
	s := NewStream[T](sizes...)
	return s, s
}

// teeReader reads from r and writes each successful value to w.
type teeReader[T any] struct {
	r Reader[T]
	w Writer[T]
}

// Read reads from the source and forwards the value to the destination
// before returning.
func (t *teeReader[T]) Read(ctx context.Context) (v T, err error) {
	v, err = t.r.Read(ctx)
	if err != nil {
		return
	}
	err = t.w.Write(ctx, v)
	return
}

// TeeReader returns a Reader that copies each successfully read value
// to w. Read errors short-circuit; write errors are returned together
// with the value.
func TeeReader[T any](r Reader[T], w Writer[T]) Reader[T] {
	return &teeReader[T]{r: r, w: w}
}

// multiReader reads through readers in order, exhausting each before
// moving to the next.
type multiReader[T any] struct {
	readers []Reader[T]
}

// Read implements [Reader.Read]. Adjacent multiReaders are flattened
// to keep the dispatch O(1) even when MultiReader is nested.
func (m *multiReader[T]) Read(ctx context.Context) (v T, err error) {
	for len(m.readers) > 0 {
		if len(m.readers) == 1 {
			if mr, ok := m.readers[0].(*multiReader[T]); ok {
				m.readers = mr.readers
				continue
			}
		}
		v, err = m.readers[0].Read(ctx)
		if err == io.EOF {
			m.readers = m.readers[1:]
			continue
		}
		return
	}
	return v, io.EOF
}

// MultiReader concatenates readers into a single Reader.
func MultiReader[T any](readers ...Reader[T]) Reader[T] {
	return &multiReader[T]{readers: slices.Clone(readers)}
}

// multiWriter fans writes out to a list of writers; the first error
// stops the broadcast.
type multiWriter[T any] struct {
	writers []Writer[T]
}

// Write implements [Writer.Write].
func (m *multiWriter[T]) Write(ctx context.Context, v T) error {
	for _, w := range m.writers {
		if err := w.Write(ctx, v); err != nil {
			return err
		}
	}
	return nil
}

// MultiWriter returns a Writer that broadcasts each value to writers
// in order, returning the first error encountered.
func MultiWriter[T any](writers ...Writer[T]) Writer[T] {
	return &multiWriter[T]{writers: slices.Clone(writers)}
}

// mapperReader applies fn to each value read from r.
type mapperReader[T, U any] struct {
	r  Reader[T]
	fn func(T) U
}

// Read implements [Reader.Read].
func (m *mapperReader[T, U]) Read(ctx context.Context) (u U, err error) {
	t, err := m.r.Read(ctx)
	if err != nil {
		return
	}
	return m.fn(t), nil
}

// Map returns a Reader that applies fn to each value read from r. fn
// must not be nil.
//
// Example:
//
//	r := stream.OfSliceReader([]int{1, 2, 3})
//	s := stream.Map(r, func(n int) string { return strconv.Itoa(n) })
func Map[T, U any](r Reader[T], fn func(T) U) Reader[U] {
	if fn == nil {
		panic("stream: nil mapper")
	}
	return &mapperReader[T, U]{r: r, fn: fn}
}

// filterReader keeps only values that satisfy pred.
type filterReader[T any] struct {
	r    Reader[T]
	pred func(T) bool
}

// Read implements [Reader.Read]. It drops values where pred is false.
func (f *filterReader[T]) Read(ctx context.Context) (v T, err error) {
	for {
		v, err = f.r.Read(ctx)
		if err != nil {
			return
		}
		if f.pred(v) {
			return
		}
	}
}

// Filter returns a Reader that yields only the values for which pred
// returns true. pred must not be nil.
func Filter[T any](r Reader[T], pred func(T) bool) Reader[T] {
	if pred == nil {
		panic("stream: nil predicate")
	}
	return &filterReader[T]{r: r, pred: pred}
}

// flatMapReader expands each source value into a Reader and emits its
// values before consuming the next source value.
type flatMapReader[T, U any] struct {
	r       Reader[T]
	fn      func(T) Reader[U]
	current Reader[U]
}

// Read implements [Reader.Read].
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
		t, err1 := f.r.Read(ctx)
		if err1 != nil {
			return v, err1
		}
		f.current = f.fn(t)
	}
}

// FlatMap returns a Reader that maps each source value through fn into
// a sub-Reader and concatenates the results in order. fn must not be
// nil.
func FlatMap[T, U any](r Reader[T], fn func(T) Reader[U]) Reader[U] {
	if fn == nil {
		panic("stream: nil mapper")
	}
	return &flatMapReader[T, U]{r: r, fn: fn}
}

// distinctReader emits each unique value from r exactly once. Memory
// grows with the number of distinct values; use only on bounded
// streams.
type distinctReader[T comparable] struct {
	r    Reader[T]
	seen map[T]struct{}
}

// Read implements [Reader.Read].
func (d *distinctReader[T]) Read(ctx context.Context) (v T, err error) {
	for {
		v, err = d.r.Read(ctx)
		if err != nil {
			return
		}
		if _, ok := d.seen[v]; !ok {
			d.seen[v] = struct{}{}
			return
		}
	}
}

// Distinct returns a Reader that drops duplicate values, keeping the
// first occurrence in order. T must be comparable.
func Distinct[T comparable](r Reader[T]) Reader[T] {
	return &distinctReader[T]{r: r, seen: make(map[T]struct{})}
}
