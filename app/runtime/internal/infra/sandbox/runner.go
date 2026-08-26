package sandbox

import (
	"bytes"
	"fmt"
)

const maxCommandOutputBytes = 256 << 10

type limitedBuffer struct {
	buffer  bytes.Buffer
	dropped int
}

func (l *limitedBuffer) Write(data []byte) (int, error) {
	available := maxCommandOutputBytes - l.buffer.Len()
	if available > 0 {
		_, _ = l.buffer.Write(data[:min(available, len(data))])
	}
	if len(data) > available {
		l.dropped += len(data) - max(available, 0)
	}
	return len(data), nil
}

func (l *limitedBuffer) BytesWithMarker() []byte {
	out := bytes.Clone(l.buffer.Bytes())
	if l.dropped == 0 {
		return out
	}
	return fmt.Appendf(out, "\n... [%d bytes truncated] ...\n", l.dropped)
}
