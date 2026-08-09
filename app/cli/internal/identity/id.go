package identity

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// New returns an unpredictable process-independent identity with a stable
// diagnostic prefix.
func New(prefix string) (string, error) {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", fmt.Errorf("generate %s identity: %w", prefix, err)
	}
	return prefix + "_" + hex.EncodeToString(entropy[:]), nil
}
