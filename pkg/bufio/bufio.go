package bufio

import (
	"bytes"
)

func ScanLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
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
