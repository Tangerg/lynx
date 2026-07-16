// Package extension owns the namespace/name policy for protocol extension maps.
package extension

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/Tangerg/lynx/core/metadata"
)

// Set validates key, encodes value, and stores it in target.
func Set(target *metadata.Map, key string, value any) error {
	if target == nil {
		return errors.New("nil extension map")
	}
	if !validKey(key) {
		return fmt.Errorf("key %q must use namespace/name", key)
	}
	return target.Set(key, value)
}

// Validate verifies extension keys and encoded values.
func Validate(values metadata.Map) error {
	for key := range values {
		if !validKey(key) {
			return fmt.Errorf("key %q must use namespace/name", key)
		}
	}
	return values.Validate()
}

func validKey(key string) bool {
	if strings.Count(key, "/") != 1 {
		return false
	}
	namespace, name, _ := strings.Cut(key, "/")
	return validSegment(namespace) && validSegment(name)
}

func validSegment(segment string) bool {
	if segment == "" {
		return false
	}
	for _, r := range segment {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}
