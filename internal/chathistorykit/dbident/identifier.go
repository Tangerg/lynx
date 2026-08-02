// Package dbident validates database identifiers interpolated into statements.
package dbident

import "regexp"

const patternText = `^[A-Za-z_][A-Za-z0-9_]*$`

var pattern = regexp.MustCompile(patternText)

// Valid reports whether value is safe to interpolate as an unquoted identifier.
func Valid(value string) bool {
	return pattern.MatchString(value)
}
