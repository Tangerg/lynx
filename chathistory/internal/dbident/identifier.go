// Package dbident validates database identifiers interpolated into statements.
package dbident

import "regexp"

// Pattern is the conservative unquoted-identifier shape shared by history
// backends before interpolating caller-supplied names into database statements.
const Pattern = `^[A-Za-z_][A-Za-z0-9_]*$`

var pattern = regexp.MustCompile(Pattern)

// Valid reports whether value is safe to interpolate as an unquoted identifier.
func Valid(value string) bool {
	return pattern.MatchString(value)
}
