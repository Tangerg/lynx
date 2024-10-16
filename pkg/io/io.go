package io

import (
	"io"
)

// ReadAll reads data from an io.Reader into a buffer of a specified size
// until an error or io.EOF is encountered, and returns the data it read.
// If the bufferSize is less than or equal to zero, a default size of 512 bytes is used.
// A successful call returns err == nil, not err == io.EOF. Because ReadAll is
// defined to read from the reader until io.EOF, it does not treat an io.EOF from Read
// as an error to be reported.
func ReadAll(r io.Reader, bufferSize int) ([]byte, error) {
	if bufferSize <= 0 {
		bufferSize = 512
	}
	buffer := make([]byte, 0, bufferSize)
	for {
		n, err := r.Read(buffer[len(buffer):cap(buffer)])
		buffer = buffer[:len(buffer)+n]
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return buffer, err
		}

		if len(buffer) == cap(buffer) {
			buffer = append(buffer, 0)[:len(buffer)]
		}
	}
}
