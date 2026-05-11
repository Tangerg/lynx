package anthropic

import "errors"

// Sentinel errors for the [anthropic] package's input-shape validators.
// Callers can match these with [errors.Is] to distinguish caller-side
// input errors from transport, SDK, or provider failures.
var (
	// ErrNilConfig is returned when a *Config validator receives nil.
	ErrNilConfig = errors.New("anthropic: config must not be nil")

	// ErrMissingApiKey is returned when a config supplies a nil
	// [model.ApiKey].
	ErrMissingApiKey = errors.New("anthropic: ApiKey is required")

	// ErrMissingDefaultOptions is returned when a *ModelConfig
	// supplies nil DefaultOptions.
	ErrMissingDefaultOptions = errors.New("anthropic: DefaultOptions is required")

	// ErrNilRequest is returned by [Api] methods when the SDK request
	// pointer is nil.
	ErrNilRequest = errors.New("anthropic: request must not be nil")
)
