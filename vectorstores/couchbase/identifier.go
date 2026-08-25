package couchbase

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
)

var identifierPatternWithDash = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

func validateIdentifiersWithDash(provider string, fields map[string]string) error {
	return validateIdentifierFields(identifierPatternWithDash, provider, fields)
}

func validateIdentifierFields(pattern *regexp.Regexp, provider string, fields map[string]string) error {
	for _, name := range slices.Sorted(maps.Keys(fields)) {
		value := fields[name]
		if !pattern.MatchString(value) {
			return fmt.Errorf("%s: %s=%q must match %s", provider, name, value, pattern)
		}
	}
	return nil
}
