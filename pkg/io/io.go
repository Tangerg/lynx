package io

import (
	"io"
	"iter"

	"github.com/Tangerg/lynx/pkg/slices"
)

// ReadAll reads all data from an io.Reader into a dynamically growing buffer.
//
// The function reads data until io.EOF is encountered or an error occurs.
// The buffer starts with the specified capacity and grows automatically as needed.
//
// Parameters:
//   - r: The io.Reader to read from
//   - bufSize: Optional initial buffer capacity (default: 512 bytes)
//     If multiple values are provided, only the first is used.
//     If the value is 0 or not provided, defaults to 512 bytes.
//     Negative values cause a panic.
//
// Returns:
//   - []byte: All data read from the reader
//   - error: Any error encountered except io.EOF (which is treated as success)
//
// Note: Unlike io.ReadAll, this function allows customizing the initial buffer capacity,
// which can improve performance when the approximate data size is known.
func ReadAll(r io.Reader, bufSize ...int) ([]byte, error) {
	size := slices.FirstOr(bufSize, 0)
	if size < 0 {
		panic("buffer capacity must not be negative")
	}
	if size == 0 {
		size = 512
	}

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

// Read returns an iterator that yields chunks of data read from an io.Reader.
//
// Each iteration reads data into a new buffer with the specified capacity.
// The iterator terminates when an error occurs or the caller breaks the loop.
//
// Parameters:
//   - r: The io.Reader to read from
//   - bufSize: Optional buffer capacity for each read operation (default: 512 bytes)
//     If multiple values are provided, only the first is used.
//     If the value is 0 or not provided, defaults to 512 bytes.
//     Negative values cause a panic.
//
// Returns:
//   - iter.Seq2[[]byte, error]: An iterator that yields (buffer, error) pairs.
//     io.EOF is converted to nil error to indicate successful completion.
//     Other errors are yielded once before terminating the iterator.
//
// Note: A new buffer is allocated for each iteration to prevent memory aliasing issues.
// The caller should process or copy the data before the next iteration if needed.
func Read(r io.Reader, bufSize ...int) iter.Seq2[[]byte, error] {
	size := slices.FirstOr(bufSize, 0)
	if size < 0 {
		panic("buffer capacity must not be negative")
	}
	if size == 0 {
		size = 512
	}

	return func(yield func([]byte, error) bool) {
		for {
			buf := make([]byte, 0, size)
			n, err := r.Read(buf[len(buf):cap(buf)])
			buf = buf[:len(buf)+n]

			if err == io.EOF {
				err = nil
			}

			if err != nil {
				yield(buf, err)
				return
			}

			if !yield(buf, nil) {
				return
			}
		}
	}
}
