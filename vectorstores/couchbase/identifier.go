package couchbase

import (
	"fmt"
	"regexp"
)

var identifierPatternWithDash = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

type identifier string

func (i identifier) validate(field string) error {
	if !identifierPatternWithDash.MatchString(string(i)) {
		return fmt.Errorf("couchbase: %s=%q must match %s", field, i, identifierPatternWithDash)
	}
	return nil
}
