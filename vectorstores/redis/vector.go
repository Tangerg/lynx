package redis

import (
	"encoding/binary"
	stdmath "math"
)

const float32ByteWidth = 4

func float32sToBytes(values []float32) []byte {
	buf := make([]byte, len(values)*float32ByteWidth)
	for i, v := range values {
		binary.LittleEndian.PutUint32(buf[i*float32ByteWidth:], stdmath.Float32bits(v))
	}
	return buf
}
