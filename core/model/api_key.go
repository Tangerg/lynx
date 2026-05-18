package model

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ApiKey hides the credential a model client uses to authenticate.
// It exists so static keys (OpenAI, Anthropic) and dynamic short-lived
// tokens (GCP Vertex AI) share one call site: clients invoke [ApiKey.Get]
// before each request and let the implementation handle caching, refresh,
// or "no auth needed".
//
// Implementations must:
//   - return a currently valid key from [ApiKey.Get] (never expired);
//   - be safe for concurrent use;
//   - return "" when no authentication is required.
type ApiKey interface {
	// Get returns a currently valid API key. Callers MUST NOT cache the
	// result — call this before each request so dynamic implementations
	// can refresh transparently. Returns "" when no authentication is
	// required.
	Get() string
}

var _ ApiKey = (*staticApiKey)(nil)

// staticApiKey holds an immutable key value and a pre-computed masked form
// for safe logging. All fields are set at construction; nothing mutates,
// so the value is trivially safe for concurrent reads.
type staticApiKey struct {
	value      string
	maskedView string
}

// NewApiKey wraps a fixed credential as an [ApiKey]. Pass "" for endpoints
// that do not require authentication.
//
// Example:
//
//	key := NewApiKey("sk-1234567890abcdef")  // OpenAI-style key
//	key := NewApiKey("")                      // local model, no auth
//
//	// In a request:
//	req.Header.Set("Authorization", "Bearer "+key.Get())
func NewApiKey(value string) ApiKey {
	k := &staticApiKey{value: value}
	k.maskedView = maskApiKey(value)
	return k
}

// Get returns the immutable key supplied at construction.
func (k *staticApiKey) Get() string {
	return k.value
}

// String returns the masked representation, suitable for logs.
// Implements [fmt.Stringer] so the value never leaks accidentally via
// "%v" or "%s" formatting.
func (k *staticApiKey) String() string {
	return k.maskedView
}

// MarshalJSON emits the masked representation so the secret cannot
// leak through a structured logger / config dumper that JSON-encodes
// a containing struct. Stringer alone is bypassed by json.Marshal,
// which inspects unexported fields directly.
func (k *staticApiKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.maskedView)
}

// maskApiKey renders a credential as "api_key=<masked>" without revealing
// the secret. It chooses one of three shapes by length:
//
//	""              -> "api_key=<empty>"
//	len ≤ 10        -> "api_key=" + asterisks
//	len > 10        -> "api_key=ab****yz" (first 2, last 2)
func maskApiKey(value string) string {
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
