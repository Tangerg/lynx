package model

import (
	"fmt"
	"strings"
)

// ApiKey represents an interface for accessing API keys that may need to be
// refreshed at regular intervals. Some model providers leverage short-lived
// API keys which must be renewed using another credential. For example, a GCP
// service account can be exchanged for an API key to call Vertex AI.
//
// Model clients use the ApiKey interface to get an API key before they make
// any request to the model provider. Implementations of this interface can
// cache the API key and perform a key refresh when it is required.
//
// This interface provides a consistent way to handle different types of API
// key scenarios:
//   - Static long-lived keys (like OpenAI, Anthropic)
//   - Dynamic short-lived keys that require periodic refresh
//   - No authentication scenarios for local or public models
type ApiKey interface {
	// Get returns an API key to use for making requests. Users of this method
	// should NOT cache the returned API key, instead call this method whenever
	// you need an API key. Implementors of this method MUST ensure that the
	// returned key is not expired.
	//
	// Returns:
	//   - A valid API key string for authentication
	//   - An empty string if no authentication is required
	Get() string
}

var (
	_ ApiKey = (*apiKey)(nil)
)

// apiKey is a simple implementation of ApiKey that holds an immutable
// API key value. This implementation is suitable for cases where the API key
// is static and does not need to be refreshed or rotated.
//
// This implementation caches the string representation for performance reasons,
// as the String() method may be called frequently during logging and debugging.
// The cached representation masks sensitive information while preserving some
// useful details for troubleshooting.
//
// Thread Safety: This implementation is thread-safe for read operations as
// all fields are set during construction and never modified afterward.
type apiKey struct {
	apiKey      string
	stringCache string
}

// NewApiKey creates a new ApiKey instance with the provided API key value.
// This implementation uses apiKey internally which is suitable for
// static API keys that do not require refresh or rotation.
//
// The function accepts any string value, including empty strings, making it
// flexible for various authentication scenarios.
//
// Parameters:
//   - apiKey: The API key string. Can be empty for scenarios where no
//     authentication is required.
//
// Returns:
//   - An ApiKey implementation that will consistently return the provided value
//
// Example usage:
//
//	key := NewApiKey("sk-1234567890abcdef")  // For OpenAI-style keys
//	emptyKey := NewApiKey("")                // For no authentication
//	shortKey := NewApiKey("abc")             // For short test keys
func NewApiKey(key string) ApiKey {
	s := &apiKey{
		apiKey: key,
	}
	s.stringCache = s.string()
	return s
}

// Get returns the API key value. This implementation always returns the same
// static value that was provided during construction.
//
// Unlike implementations that refresh tokens, this method is lightweight and
// always returns immediately without any network calls or complex logic.
//
// Returns:
//   - The exact API key string that was provided to NewApiKey()
func (s *apiKey) Get() string {
	return s.apiKey
}

// string generates a safe string representation of the API key that masks
// sensitive information while preserving some useful details for debugging.
//
// The masking strategy varies based on the API key length:
//   - Empty keys: Returns "api_key=<empty>"
//   - Short keys (≤4 chars): Returns "api_key=" followed by asterisks
//   - Long keys (>4 chars): Shows first 2 and last 2 characters with asterisks in between
//
// Examples:
//   - "" → "api_key=<empty>"
//   - "abc" → "api_key=***"
//   - "abcd" → "api_key=****"
//   - "sk-1234567890" → "api_key=sk******90"
//   - "very-long-api-key-here" → "api_key=ve***************re"
//
// This approach provides enough information for debugging (key format, length)
// while preventing accidental exposure of sensitive credentials in logs.
func (s *apiKey) string() string {
	if s.apiKey == "" {
		return "api_key=<empty>"
	}
	if len(s.apiKey) <= 4 {
		return "api_key=" + strings.Repeat("*", len(s.apiKey))
	}
	return fmt.Sprintf("api_key=%s%s%s",
		s.apiKey[:2],
		strings.Repeat("*", len(s.apiKey)-4),
		s.apiKey[len(s.apiKey)-2:])
}

// String returns a cached string representation of the API key with sensitive
// information masked. This method is safe to use in logging and debugging
// contexts as it does not expose the actual API key value.
//
// The string representation is computed once during construction and cached
// for performance, making subsequent calls very efficient.
//
// This method implements the fmt.Stringer interface, allowing the ApiKey to be
// safely used in logging statements, error messages, and debugging output
// without risk of credential leakage.
//
// Examples of output:
//
//	fmt.Println(NewApiKey("sk-1234567890"))     // Output: api_key=sk******90
//	fmt.Println(NewApiKey(""))                  // Output: api_key=<empty>
//	fmt.Println(NewApiKey("abc"))               // Output: api_key=***
//
// Returns:
//   - A masked string representation safe for logging and display
func (s *apiKey) String() string {
	return s.stringCache
}
