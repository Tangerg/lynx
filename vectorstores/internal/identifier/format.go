package identifier

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
)

// Format defines the safe ASCII identifier grammar accepted at a provider
// boundary.
type Format uint8

const (
	// Strict allows a leading letter or underscore followed by letters,
	// digits, or underscores.
	Strict Format = iota
	// WithDash extends [Strict] by allowing dashes after the first character.
	WithDash
)

var (
	strictPattern   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	withDashPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)
)

// Match reports whether value conforms to the format.
func (format Format) Match(value string) bool {
	return format.pattern().MatchString(value)
}

// Validate checks named fields in deterministic name order.
func (format Format) Validate(provider string, fields map[string]string) error {
	pattern := format.pattern()
	for _, name := range slices.Sorted(maps.Keys(fields)) {
		value := fields[name]
		if !pattern.MatchString(value) {
			return fmt.Errorf("%s: %s=%q must match %s", provider, name, value, pattern)
		}
	}
	return nil
}

func (format Format) pattern() *regexp.Regexp {
	switch format {
	case Strict:
		return strictPattern
	case WithDash:
		return withDashPattern
	default:
		panic(fmt.Sprintf("identifier: unknown format %d", format))
	}
}
