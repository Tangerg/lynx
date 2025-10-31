package bufio

import (
	"bytes"
)

// dropCR drops a terminal \r from the data.
func dropCR(data []byte) []byte {
	if len(data) > 0 && data[len(data)-1] == '\r' {
		return data[0 : len(data)-1]
	}
	return data
}

// ScanLinesAllFormats is a custom split function for bufio.Scanner that returns each line of
// text, stripped of any trailing end-of-line marker ('\r', '\n', or "\r\n").
// The returned line may be empty.
// The end-of-line marker is not included in the returned line.
//
// Unlike the standard library's ScanLines function, this handles all three common
// line ending formats:
// - "\r\n" (Windows)
// - "\r" (old Mac OS 9 and earlier)
// - "\n" (Unix/Linux/Mac OS X and later)
//
// This is particularly useful for scenarios like SSE (Server-Sent Events) processing
// where data might come from various sources with different line ending conventions.
//
// Parameters:
//   - data: The input bytes to be scanned
//   - atEOF: A flag indicating whether the end of the input has been reached
//
// Returns:
//   - advance: The number of bytes to advance the input
//   - token: The token identified (line without EOL marker)
//   - err: Any error encountered during scanning
func ScanLinesAllFormats(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	n := bytes.IndexByte(data, '\n')
	r := bytes.IndexByte(data, '\r')

	// only \n
	if r == -1 && n >= 0 {
		return n + 1, dropCR(data[0:n]), nil
	}

	// only \r
	if n == -1 && r >= 0 {
		return r + 1, dropCR(data[0:r]), nil
	}

	// \r && \n
	if n >= 0 && r >= 0 {
		// \r\n
		if n == r+1 {
			return n + 1, dropCR(data[0:n]), nil
		}
		// \r...\n
		if n > r {
			return r + 1, dropCR(data[0:r]), nil
		}
		// \n...\r
		return n + 1, dropCR(data[0:n]), nil
	}

	// If we're at EOF, we have a final, non-terminated line. Return it.
	if atEOF {
		return len(data), dropCR(data), nil
	}

	// Request more data.
	return 0, nil, nil
}
