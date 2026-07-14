// Package secret contains presentation-safe helpers for runtime credentials.
package secret

const (
	maskStars      = "****"
	maskEndsMinLen = 9
)

// Mask returns a fixed-width redaction suitable for logs and wire payloads.
// Empty values remain empty; short values reveal nothing; longer values keep
// only their first and last two bytes.
func Mask(value string) string {
	if value == "" {
		return ""
	}
	if len(value) < maskEndsMinLen {
		return maskStars
	}
	return value[:2] + maskStars + value[len(value)-2:]
}
