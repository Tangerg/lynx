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
	// If we're at EOF and there's no data, return 0 to indicate no more tokens
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	// Find the position of the first '\n' character, returns -1 if not found
	n := bytes.IndexByte(data, '\n')
	// Find the position of the first '\r' character, returns -1 if not found
	r := bytes.IndexByte(data, '\r')

	// Handle the case when both \r and \n exist in the data
	if n >= 0 && r >= 0 {
		// Check if it's a Windows-style line ending "\r\n"
		if n == r+1 {
			// Advance past the '\n', drop the '\r' from the token
			return n + 1, dropCR(data[0:n]), nil
		}

		// For "\r...\n" or "\n...\r" patterns, use the earlier occurrence
		// min(n, r) gives us the position of whichever comes first
		i := min(n, r)
		// Advance past the first line ending character, drop any trailing '\r'
		return i + 1, dropCR(data[0:i]), nil
	}
	// Handle the case when only \r or only \n exists (not both)
	// max(n, r) returns the valid index (the other one is -1)
	if i := max(n, r); i >= 0 {
		// Advance past the line ending character, drop any trailing '\r'
		return i + 1, dropCR(data[0:i]), nil
	}

	// If we're at EOF, return the remaining data as the final line
	if atEOF {
		// No line ending characters exist in data at this point (both n and r are -1)
		// So no need to drop '\r'
		return len(data), data, nil
	}

	// Request more data by returning 0 advance with no token
	return 0, nil, nil
}
