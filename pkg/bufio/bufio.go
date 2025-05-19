package bufio

import (
	"bytes"
)

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

	if i := bytes.IndexByte(data, '\r'); i >= 0 {
		if i+1 < len(data) && data[i+1] == '\n' {
			return i + 2, data[:i], nil
		}
		return i + 1, data[:i], nil
	}

	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return i + 1, data[:i], nil
	}

	if atEOF {
		if len(data) > 0 && data[len(data)-1] == '\r' {
			return len(data), data[:len(data)-1], nil
		}
		return len(data), data, nil
	}

	return 0, nil, nil
}
