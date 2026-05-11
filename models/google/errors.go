package google

import "errors"

// Sentinel errors for the [google] package's input-shape validators.
// Callers can match these with [errors.Is] to distinguish caller-side
// input errors from transport, SDK, or provider failures.
var (
	// ErrNilConfig is returned when a *Config validator receives nil.
	ErrNilConfig = errors.New("google: config must not be nil")

	// ErrMissingApiKey is returned when a config supplies a nil
	// [model.ApiKey].
	ErrMissingApiKey = errors.New("google: ApiKey is required")

	// ErrMissingDefaultOptions is returned when a *ModelConfig
	// supplies nil DefaultOptions.
	ErrMissingDefaultOptions = errors.New("google: DefaultOptions is required")
)
