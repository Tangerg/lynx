// Package requestid creates collision-resistant identifiers for idempotent run
// requests.
package requestid

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// New returns an unpredictable process-independent request identity.
func New() (string, error) {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", fmt.Errorf("generate request identity: %w", err)
	}
	return "req_" + hex.EncodeToString(entropy[:]), nil
}
