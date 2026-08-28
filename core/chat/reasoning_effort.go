package chat

import (
	"errors"
	"strings"
)

// ReasoningEffort is a provider-neutral reasoning intensity selected from a
// model's advertised values. It is intentionally open rather than a fixed enum:
// the selected model owns its accepted vocabulary.
type ReasoningEffort string

// Validate rejects values whose identity would change under trimming. Empty is
// valid and asks the provider adapter to use the selected model's default.
func (r ReasoningEffort) Validate() error {
	if r != ReasoningEffort(strings.TrimSpace(string(r))) {
		return errors.New("reasoning effort must not have surrounding whitespace")
	}
	return nil
}
