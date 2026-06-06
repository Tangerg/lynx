package model

import "encoding/json"

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

// maskStars is the fixed middle of a masked key. Fixed-width on purpose: the
// rendering must never reveal the real key length, so the star run does not
// scale with it.
const maskStars = "****"

// maskEndsMinLen is the shortest key that still shows its first and last two
// characters. Shorter keys are masked whole, so a tiny key never has most of
// itself exposed.
const maskEndsMinLen = 9

// MaskAPIKey redacts a credential for display — the reusable form behind every
// log line, JSON field, or wire payload that must show a key without leaking
// it. The middle is a fixed-width star run, so the output never reveals the key
// length, and it carries no "api_key=" label (callers add their own context):
//
//	""                    -> ""
//	len < maskEndsMinLen  -> "****"
//	len ≥ maskEndsMinLen  -> "ab****yz" (first 2, last 2)
func MaskAPIKey(value string) string {
	if value == "" {
		return ""
	}
	if len(value) < maskEndsMinLen {
		return maskStars
	}
	return value[:2] + maskStars + value[len(value)-2:]
}

// masked renders this key's value through [MaskAPIKey] for the Stringer /
// JSON leak-safe paths.
func (k *staticAPIKey) masked() string { return MaskAPIKey(k.value) }
