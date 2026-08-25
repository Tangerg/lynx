package cassandra

import (
	"strconv"
	"strings"
)

func formatVectorLiteral(vector []float32) string {
	var text strings.Builder
	text.Grow(len(vector) * 8)
	text.WriteByte('[')
	for i, value := range vector {
		if i > 0 {
			text.WriteByte(',')
		}
		text.WriteString(strconv.FormatFloat(float64(value), 'f', -1, 32))
	}
	text.WriteByte(']')
	return text.String()
}
