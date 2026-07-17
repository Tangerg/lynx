// Package panicerr converts recovered panic values into ordinary errors at
// executable extension boundaries.
package panicerr

import "fmt"

// New prefixes a recovered panic value with message. Error values remain in
// the unwrap chain; other panic values retain their formatted representation.
func New(message string, recovered any) error {
	if cause, ok := recovered.(error); ok {
		return fmt.Errorf("%s: %w", message, cause)
	}
	return fmt.Errorf("%s: %v", message, recovered)
}
