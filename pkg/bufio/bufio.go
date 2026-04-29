package bufio

import "bytes"

// dropCR strips a trailing '\r' from data.
func dropCR(data []byte) []byte {
	if len(data) > 0 && data[len(data)-1] == '\r' {
		return data[:len(data)-1]
	}
	return data
}

// ScanLinesAllFormats is a bufio.SplitFunc that splits on "\n", "\r",
// or "\r\n". The returned token excludes the line terminator and may
// be empty.
//
// Unlike bufio.ScanLines, it accepts classic Mac "\r" terminators in
// addition to Unix "\n" and Windows "\r\n", so the same scanner can
// process inputs from any platform.
//
// Example:
//
//	sc := bufio.NewScanner(r)
//	sc.Split(xbufio.ScanLinesAllFormats)
//	for sc.Scan() {
//	    fmt.Println(sc.Text())
//	}
func ScanLinesAllFormats(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	n := bytes.IndexByte(data, '\n')
	r := bytes.IndexByte(data, '\r')

	switch {
	case n >= 0 && r >= 0:
		// "\r\n" pair: consume both, return everything before \r.
		if n == r+1 {
			return n + 1, dropCR(data[:n]), nil
		}
		// Otherwise consume the earlier terminator.
		i := min(n, r)
		return i + 1, dropCR(data[:i]), nil
	case n >= 0 || r >= 0:
		i := max(n, r)
		return i + 1, dropCR(data[:i]), nil
	case atEOF:
		// Final line without terminator.
		return len(data), data, nil
	default:
		// Need more input.
		return 0, nil, nil
	}
}
