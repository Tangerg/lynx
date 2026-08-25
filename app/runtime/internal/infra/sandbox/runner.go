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

func (b *limitedBuffer) Write(data []byte) (int, error) {
	available := maxCommandOutputBytes - b.buffer.Len()
	if available > 0 {
		_, _ = b.buffer.Write(data[:min(available, len(data))])
	}
	if len(data) > available {
		b.dropped += len(data) - max(available, 0)
	}
	return len(data), nil
}

func (b *limitedBuffer) BytesWithMarker() []byte {
	out := bytes.Clone(b.buffer.Bytes())
	if b.dropped == 0 {
		return out
	}
	return fmt.Appendf(out, "\n... [%d bytes truncated] ...\n", b.dropped)
}
