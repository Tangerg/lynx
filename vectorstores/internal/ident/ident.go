package ident

import (
	"fmt"
	"regexp"
)

// Pattern matches the standard SQL unquoted identifier shape — a
// leading letter or underscore followed by letters, digits, or
// underscores.
var Pattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// PatternWithDash adds `-` to [Pattern]. Couchbase bucket / scope /
// collection / index names allow hyphens, and Vespa namespaces /
// schemas are routinely hyphenated.
var PatternWithDash = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

// Check validates every (name, value) pair in fields against
// [Pattern]. The pkg prefix lets the wrapped error name the store
// providing the failing identifier.
func Check(pkg string, fields map[string]string) error {
	return checkWith(Pattern, pkg, fields)
}

// CheckWithDash is the hyphen-friendly counterpart to [Check].
func CheckWithDash(pkg string, fields map[string]string) error {
	return checkWith(PatternWithDash, pkg, fields)
}

func checkWith(pat *regexp.Regexp, pkg string, fields map[string]string) error {
	for name, value := range fields {
		if !pat.MatchString(value) {
			return fmt.Errorf("%s: %s=%q must match %s", pkg, name, value, pat)
		}
	}
	return nil
}
