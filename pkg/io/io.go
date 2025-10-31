package io

import (
	"io"

	"github.com/Tangerg/lynx/pkg/slices"
)

// ReadAll reads all data from an io.Reader into a dynamically growing buffer.
//
// The function reads data until io.EOF is encountered or an error occurs.
// The buffer starts with the specified size and grows automatically as needed.
//
// Parameters:
//   - r: The io.Reader to read from
//   - bufferSize: Optional initial buffer size (default: 512 bytes)
//     If multiple values are provided, only the first is used.
//     If the value is 0 or not provided, defaults to 512 bytes.
//     Negative values cause a panic.
//
// Returns:
//   - []byte: All data read from the reader
//   - error: Any error encountered except io.EOF (which is treated as success)
//
// Note: Unlike io.ReadAll, this function allows customizing the initial buffer size,
// which can improve performance when the approximate data size is known.
func ReadAll(r io.Reader, bufferSize ...int) ([]byte, error) {
	size := slices.FirstOr(bufferSize, 0)
	if size < 0 {
		panic("buffer size cannot be negative")
	}
	if size == 0 {
		size = 512
	}

	buffer := make([]byte, 0, size)
	for {
		// Expand buffer capacity if needed before reading
		if len(buffer) == cap(buffer) {
			// Double the capacity by appending a zero byte and truncating
			buffer = append(buffer, 0)[:len(buffer)]
		}

		// Read into the available capacity
		n, err := r.Read(buffer[len(buffer):cap(buffer)])
		buffer = buffer[:len(buffer)+n]

		if err != nil {
			// io.EOF indicates successful completion, not an error
			if err == io.EOF {
				err = nil
			}
			return buffer, err
		}
	}
}
