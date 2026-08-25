// Package vectorliteral formats float32 vectors for database protocols that
// accept bracketed numeric literals.
package vectorliteral

import (
	"strconv"
	"strings"
)

// Format renders vector as a compact bracketed float32 literal.
func Format(vector []float32) string {
	var text strings.Builder
	text.Grow(len(vector) * 8)
	text.WriteByte('[')
	for index, value := range vector {
		if index > 0 {
			text.WriteByte(',')
		}
		text.WriteString(strconv.FormatFloat(float64(value), 'f', -1, 32))
	}
	text.WriteByte(']')
	return text.String()
}
