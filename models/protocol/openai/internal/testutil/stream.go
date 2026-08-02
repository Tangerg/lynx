package testutil

import (
	"iter"
)

// Collect drains an iter.Seq2[T, error] iterator into a slice. The
// iteration stops on the first non-nil error, which is returned along
// with whatever was yielded so far.
//
// This is the canonical helper for streaming-test assertions: spin up
// a mock SSE server, call model.Stream(ctx, req), Collect the result,
// then assert on the slice + final error.
func Collect[T any](seq iter.Seq2[T, error]) ([]T, error) {
	var out []T
	for v, err := range seq {
		if err != nil {
			return out, err
		}
		out = append(out, v)
	}
	return out, nil
}

// CollectN drains at most n items from the iterator. Use this for
// cancellation tests — break early to verify the iterator's stop
// function tears down the upstream connection cleanly.
func CollectN[T any](seq iter.Seq2[T, error], n int) ([]T, error) {
	var out []T
	var i int
	for v, err := range seq {
		if err != nil {
			return out, err
		}
		out = append(out, v)
		i++
		if i >= n {
			return out, nil
		}
	}
	return out, nil
}
