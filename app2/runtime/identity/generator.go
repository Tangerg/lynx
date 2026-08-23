// Package identity creates opaque, sortable-enough product identities without
// leaking storage or transport concerns into domain aggregates.
package identity

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

type Generator struct{}

func (Generator) New(prefix string) (string, error) {
	if prefix == "" {
		return "", fmt.Errorf("identity: prefix is required")
	}
	bytes := make([]byte, 18)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("identity: random source: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(bytes), nil
}
