package model

import (
	"encoding/json"
	"fmt"
	"strings"
)

// APIKey hides the credential a model client uses to authenticate.
// It exists so static keys (OpenAI, Anthropic) and dynamic short-lived
// tokens (GCP Vertex AI) share one call site: clients invoke [APIKey.Get]
// before each request and let the implementation handle caching, refresh,
// or "no auth needed".
//
// Implementations must:
//   - return a currently valid key from [APIKey.Get] (never expired);
//   - be safe for concurrent use;
//   - return "" when no authentication is required.
type APIKey interface {
	// Get returns a currently valid API key. Callers MUST NOT cache the
	// result — call this before each request so dynamic implementations
	// can refresh transparently. Returns "" when no authentication is
	// required.
	Get() string
}

var _ APIKey = (*staticAPIKey)(nil)

// staticAPIKey holds an immutable key value. The value is set at
// construction and never mutates, so it is trivially safe for concurrent
// reads; the masked form for logging is derived on demand by [staticAPIKey.masked].
type staticAPIKey struct {
	value string
}

// NewAPIKey wraps a fixed credential as an [APIKey]. Pass "" for endpoints
// that do not require authentication.
//
// Example:
//
//	key := NewAPIKey("sk-1234567890abcdef")  // OpenAI-style key
//	key := NewAPIKey("")                      // local model, no auth
//
//	// In a request:
//	req.Header.Set("Authorization", "Bearer "+key.Get())
func NewAPIKey(value string) APIKey {
	return &staticAPIKey{value: value}
}

// Get returns the immutable key supplied at construction.
func (k *staticAPIKey) Get() string {
	return k.value
}

// String returns the masked representation, suitable for logs.
// Implements [fmt.Stringer] so the value never leaks accidentally via
// "%v" or "%s" formatting.
func (k *staticAPIKey) String() string {
	return k.masked()
}

// MarshalJSON emits the masked representation so the secret cannot
// leak through a structured logger / config dumper that JSON-encodes
// a containing struct. Stringer alone is bypassed by json.Marshal,
// which inspects unexported fields directly.
func (k *staticAPIKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.masked())
}

// masked renders the key as "api_key=<masked>" without revealing the
// secret. It chooses one of three shapes by length:
//
//	""              -> "api_key=<empty>"
//	len ≤ 10        -> "api_key=" + asterisks
//	len > 10        -> "api_key=ab****yz" (first 2, last 2)
func (k *staticAPIKey) masked() string {
	value := k.value
	if value == "" {
		return "api_key=<empty>"
	}

	if len(value) <= 10 {
		return "api_key=" + strings.Repeat("*", len(value))
	}

	return fmt.Sprintf("api_key=%s%s%s",
		value[:2],
		strings.Repeat("*", len(value)-4),
		value[len(value)-2:],
	)
}
