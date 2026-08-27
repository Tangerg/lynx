package redis

import (
	"encoding/binary"
	stdmath "math"
)

// float32sToBytes serializes a vector into the little-endian FLOAT32
// blob RediSearch expects.
func float32sToBytes(values []float32) []byte {
	buf := make([]byte, len(values)*4)
	for i, v := range values {
		binary.LittleEndian.PutUint32(buf[i*4:], stdmath.Float32bits(v))
	}
	return buf
}
