package sandbox

import (
	"io"
	"testing"
)

type repeatingReader struct{}

func (repeatingReader) Read(data []byte) (int, error) {
	for index := range data {
		data[index] = 'x'
	}
	return len(data), nil
}

func TestLimitedBufferCannotBypassWriteLimitThroughReaderFrom(t *testing.T) {
	const inputBytes = 2 * maxCommandOutputBytes

	var output limitedBuffer
	written, err := io.Copy(&output, io.LimitReader(repeatingReader{}, inputBytes))
	if err != nil {
		t.Fatal(err)
	}
	if written != inputBytes {
		t.Fatalf("io.Copy wrote %d bytes; want %d", written, inputBytes)
	}
	if output.Len() != maxCommandOutputBytes {
		t.Fatalf("limited buffer retained %d bytes; want %d", output.Len(), maxCommandOutputBytes)
	}
	if output.dropped != maxCommandOutputBytes {
		t.Fatalf("limited buffer dropped %d bytes; want %d", output.dropped, maxCommandOutputBytes)
	}
}
