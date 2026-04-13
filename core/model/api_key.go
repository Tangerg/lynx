package model

import (
	"fmt"
	"strings"
)

// ApiKey provides a consistent interface for accessing API keys in LLM model clients.
// Supports both static long-lived keys and dynamic short-lived keys that require periodic refresh.
//
// Common use cases:
// - Static keys (OpenAI, Anthropic): Never change during application lifecycle
// - Dynamic keys (GCP Vertex AI): Short-lived tokens refreshed from service accounts
// - No authentication: Local or public models that don't require credentials
//
// Model clients call Get() before each request to ensure valid authentication.
// Implementations handle caching and refresh logic internally.
type ApiKey interface {
	// Get returns a currently valid API key for authentication.
	// Callers should not cache the result - call this method whenever an API key is needed.
	// Implementations must ensure the returned key is not expired.
	//
	// Returns empty string if no authentication is required.
	Get() string
}

var (
	_ ApiKey = (*apiKey)(nil)
)

// apiKey implements ApiKey for static API keys that never change.
// Thread-safe for concurrent access as all fields are immutable after construction.
// Caches string representation for efficient logging and debugging.
type apiKey struct {
	apiKey      string // Immutable API key value
	stringCache string // Cached masked representation for logging
}

// NewApiKey creates an ApiKey for static keys that don't require refresh.
// Accepts any string value including empty strings for no-auth scenarios.
//
// Examples:
//
//	NewApiKey("sk-1234567890abcdef")  // OpenAI-style key
//	NewApiKey("")                      // No authentication
//	NewApiKey("test-key")             // Development key
func NewApiKey(key string) ApiKey {
	s := &apiKey{
		apiKey: key,
	}
	s.stringCache = s.string()
	return s
}

// Get returns the static API key value provided during construction.
// Always returns immediately without network calls or refresh logic.
func (s *apiKey) Get() string {
	return s.apiKey
}

// string generates a masked representation of the API key for safe logging.
// Masking strategy based on key length:
// - Empty: "api_key=<empty>"
// - Short (≤10 chars): "api_key=" + asterisks
// - Long (>10 chars): Shows first 2 and last 2 chars with asterisks between
//
// Examples:
//
//	"" → "api_key=<empty>"
//	"abc" → "api_key=***"
//	"sk-1234567890" → "api_key=sk******90"
func (s *apiKey) string() string {
	if s.apiKey == "" {
		return "api_key=<empty>"
	}
	if len(s.apiKey) <= 10 {
		return "api_key=" + strings.Repeat("*", len(s.apiKey))
	}
	return fmt.Sprintf("api_key=%s%s%s",
		s.apiKey[:2],
		strings.Repeat("*", len(s.apiKey)-4),
		s.apiKey[len(s.apiKey)-2:])
}

// String returns a cached masked representation safe for logging and debugging.
// Implements fmt.Stringer interface for safe use in log statements without credential exposure.
// String representation is computed once and cached for performance.
func (s *apiKey) String() string {
	return s.stringCache
}
