package message

// Headers is an interface that defines methods for managing key-value pairs.
// It provides methods to retrieve and set values associated with specific keys.
type Headers interface {
	// Get retrieves the value associated with the given key.
	// It returns the value and a boolean indicating whether the key exists.
	Get(key string) (any, bool)

	// Set assigns a value to the specified key.
	// It returns the Headers interface to allow for method chaining.
	Set(key string, value any) Headers
}

func NewSimpleHeaders() Headers {
	return make(SimpleHeaders)
}

type SimpleHeaders map[string]any

func (s SimpleHeaders) Get(key string) (any, bool) {
	v, ok := s[key]
	return v, ok
}

func (s SimpleHeaders) Set(key string, value any) Headers {
	s[key] = value
	return s
}
