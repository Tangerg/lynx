// Package secrets provides application-level handling for credential values
// that may cross a presentation or diagnostic boundary.
package secrets

const (
	maskStars      = "****"
	maskEndsMinLen = 9
)

// Mask redacts a secret for logs and presentation. Empty values remain empty;
// short values reveal nothing; longer values retain only their first and last
// two bytes.
func Mask(value string) string {
	if value == "" {
		return ""
	}
	if len(value) < maskEndsMinLen {
		return maskStars
	}
	return value[:2] + maskStars + value[len(value)-2:]
}
