package converter

// adapterConverter adapts any StructuredConverter[T] to StructuredConverter[any]
// by wrapping the original converter and performing type erasure.
type adapterConverter struct {
	format      string
	convertFunc func(raw string) (any, error)
}

// GetFormat returns the format instructions from the wrapped converter.
func (c *adapterConverter) GetFormat() string {
	return c.format
}

// Convert delegates to the wrapped convertFunc and returns the result as any.
func (c *adapterConverter) Convert(raw string) (any, error) {
	return c.convertFunc(raw)
}

// AsAny wraps any StructuredConverter[T] and converts it to StructuredConverter[any].
// The adapter preserves format instructions and conversion behavior while changing
// the return type from T to any.
func AsAny[T any](converter StructuredConverter[T]) StructuredConverter[any] {
	return &adapterConverter{
		format: converter.GetFormat(),
		convertFunc: func(raw string) (any, error) {
			result, err := converter.Convert(raw)
			return result, err
		},
	}
}

// ListAsAny creates a StructuredConverter[any] that parses comma-separated values.
// Equivalent to AsAny(NewListConverter()).
func ListAsAny() StructuredConverter[any] {
	return AsAny(NewListConverter())
}

// MapAsAny creates a StructuredConverter[any] that parses JSON objects into maps.
// Equivalent to AsAny(NewMapConverter()).
func MapAsAny() StructuredConverter[any] {
	return AsAny(NewMapConverter())
}

// JSONAsAnyOf creates a StructuredConverter[any] that parses JSON into type T
// and returns it as any. Equivalent to AsAny(NewJSONConverter[T]()).
func JSONAsAnyOf[T any]() StructuredConverter[any] {
	return AsAny(NewJSONConverter[T]())
}
