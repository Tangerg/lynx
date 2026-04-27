package io

import (
	"io"
	"iter"

	"github.com/Tangerg/lynx/pkg/slices"
)

// defaultBufSize is used when the caller does not provide bufSize or
// passes 0.
const defaultBufSize = 512

// ReadAll reads from r until EOF and returns the data. It mirrors
// io.ReadAll but accepts an optional initial buffer capacity, which
// can avoid reallocations when the size is roughly known.
//
// Only the first value of bufSize is used. A negative value panics.
//
// Example:
//
//	body, err := io.ReadAll(resp.Body, 16*1024)
func ReadAll(r io.Reader, bufSize ...int) ([]byte, error) {
	size := bufferSize(bufSize)
	buf := make([]byte, 0, size)
	for {
		n, err := r.Read(buf[len(buf):cap(buf)])
		buf = buf[:len(buf)+n]
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return buf, err
		}
		if len(buf) == cap(buf) {
			buf = append(buf, 0)[:len(buf)]
		}
	}
}

// Read returns an iterator that yields successive chunks read from r.
// Each iteration allocates a fresh buffer of size bufSize so that the
// previous chunk remains valid after the iterator advances. EOF
// terminates iteration without yielding an error.
//
// Only the first value of bufSize is used. A negative value panics.
//
// Example:
//
//	for chunk, err := range io.Read(resp.Body, 16*1024) {
//	    if err != nil {
//	        return err
//	    }
//	    process(chunk)
//	}
func Read(r io.Reader, bufSize ...int) iter.Seq2[[]byte, error] {
	size := bufferSize(bufSize)
	return func(yield func([]byte, error) bool) {
		for {
			buf := make([]byte, size)
			n, err := r.Read(buf)
			eof := err == io.EOF
			if eof {
				err = nil
			}
			if n > 0 || err != nil {
				if !yield(buf[:n], err) {
					return
				}
			}
			if eof || err != nil {
				return
			}
		}
	}
}

// bufferSize resolves the optional bufSize variadic into a positive
// buffer capacity, panicking on negative input.
func bufferSize(bufSize []int) int {
	size := slices.FirstOr(bufSize, 0)
	if size < 0 {
		panic("io: buffer capacity must not be negative")
	}
	if size == 0 {
		return defaultBufSize
	}
	return size
}
