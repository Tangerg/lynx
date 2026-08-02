// Package streamio contains the bounded-read primitive shared by streaming
// model adapters.
package streamio

import (
	"io"
	"iter"
)

const chunkSize = 16 * 1024

// Read returns an iterator over independently owned chunks read from reader.
func Read(reader io.Reader) iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		for {
			buffer := make([]byte, chunkSize)
			read, err := reader.Read(buffer)
			eof := err == io.EOF
			if eof {
				err = nil
			}
			if read > 0 || err != nil {
				if !yield(buffer[:read], err) {
					return
				}
			}
			if eof || err != nil {
				return
			}
		}
	}
}
