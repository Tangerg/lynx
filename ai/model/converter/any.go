package converter

// adapterConverter is an internal type that adapts any StructuredConverter[T]
// to StructuredConverter[any] by wrapping the original converter and performing
// type erasure on the conversion result.
//
// This adapter enables treating converters with different concrete return types
// uniformly as converters that return interface{} (any), which is useful for
// scenarios where you need to store multiple converters in a collection or
// handle them polymorphically.
type adapterConverter struct {
	// format stores the format instructions from the wrapped converter
	format string
	// convertFunc is the wrapped conversion function that performs the actual
	// conversion and returns the result as any type
	convertFunc func(raw string) (any, error)
}

// GetFormat returns the format instructions that were captured from the
// original wrapped converter. This ensures that the adapter preserves
// the same formatting requirements as the original converter.
func (c *adapterConverter) GetFormat() string {
	return c.format
}

// Convert performs the conversion by delegating to the wrapped convertFunc
// and returns the result as any type. Any errors from the underlying
// converter are propagated unchanged.
func (c *adapterConverter) Convert(raw string) (any, error) {
	return c.convertFunc(raw)
}

// AsAny creates a type-erased adapter that wraps any StructuredConverter[T]
// and converts it to a StructuredConverter[any]. This generic function provides
// a flexible way to adapt converters of any concrete type to the any interface.
//
// The adapter preserves the original converter's format instructions and
// conversion behavior while changing only the return type from T to any.
//
// Type parameters:
//   - T: The concrete type that the original converter produces
//
// Parameters:
//   - converter: The original StructuredConverter[T] to be adapted
//
// Returns:
//   - A StructuredConverter[any] that wraps the original converter
//
// Example:
//
//	userConverter := NewJSONConverter[User]()
//	anyConverter := AsAny(userConverter)
//	result, err := anyConverter.Convert(`{"name":"John","age":30}`)
//	// result is of type any, but contains a User struct
func AsAny[T any](converter StructuredConverter[T]) StructuredConverter[any] {
	return &adapterConverter{
		format: converter.GetFormat(),
		convertFunc: func(raw string) (any, error) {
			result, err := converter.Convert(raw)
			return result, err
		},
	}
}

// ListAsAny creates a StructuredConverter[any] that parses comma-separated
// values and returns the result as any type (specifically []string cast to any).
//
// This is a convenience function equivalent to AsAny(NewListConverter()).
// The returned converter will parse input like "apple, banana, cherry" into
// a []string and return it as any.
//
// Returns:
//   - A StructuredConverter[any] configured for parsing comma-separated lists
//
// Example:
//
//	converter := ListAsAny()
//	result, err := converter.Convert("apple, banana, cherry")
//	// result is []string{"apple", "banana", "cherry"} as any
func ListAsAny() StructuredConverter[any] {
	return AsAny(NewListConverter())
}

// MapAsAny creates a StructuredConverter[any] that parses JSON objects
// and returns the result as any type (specifically map[string]any cast to any).
//
// This is a convenience function equivalent to AsAny(NewMapConverter()).
// The returned converter will parse JSON object input into a map[string]any
// and return it as any.
//
// Returns:
//   - A StructuredConverter[any] configured for parsing JSON objects into maps
//
// Example:
//
//	converter := MapAsAny()
//	result, err := converter.Convert(`{"name":"John","age":30}`)
//	// result is map[string]any{"name":"John", "age":30} as any
func MapAsAny() StructuredConverter[any] {
	return AsAny(NewMapConverter())
}

// JSONAsAny creates a StructuredConverter[any] that parses JSON data
// and returns the result as any type. The underlying converter uses
// NewJSONConverter[any](), which means it can parse any valid JSON
// structure (objects, arrays, primitives) into the corresponding Go types.
//
// This is a convenience function equivalent to AsAny(NewJSONConverter[any]()).
// The returned converter will parse JSON input into the appropriate Go type
// (map[string]any for objects, []any for arrays, etc.) and return it as any.
//
// Returns:
//   - A StructuredConverter[any] configured for parsing any JSON structure
//
// Example:
//
//	converter := JSONAsAny()
//	result, err := converter.Convert(`{"users":[{"name":"John"},{"name":"Jane"}]}`)
//	// result is map[string]any with nested structures as any
func JSONAsAny() StructuredConverter[any] {
	return AsAny(NewJSONConverter[any]())
}

// JSONAsAnyOf creates a StructuredConverter[any] that parses JSON data
// into a specific type T and then returns it as any. This function provides
// type-safe JSON parsing while maintaining compatibility with the any interface.
//
// This is a convenience function equivalent to AsAny(NewJSONConverter[T]()).
// The returned converter will parse JSON input according to the structure
// of type T, validate the JSON schema compliance, and return the parsed
// T value cast to any.
//
// Type parameters:
//   - T: The specific Go type to parse the JSON into (e.g., User, Product, etc.)
//
// Returns:
//   - A StructuredConverter[any] configured for parsing JSON into type T
//
// Example:
//
//	type User struct {
//	    Name string `json:"name"`
//	    Age  int    `json:"age"`
//	}
//
//	converter := JSONAsAnyOf[User]()
//	result, err := converter.Convert(`{"name":"John","age":30}`)
//	// result is User{Name:"John", Age:30} as any
//
//	// Type assertion can be used to get back the concrete type
//	if user, ok := result.(User); ok {
//	    fmt.Printf("User: %s, Age: %d", user.Name, user.Age)
//	}
func JSONAsAnyOf[T any]() StructuredConverter[any] {
	return AsAny(NewJSONConverter[T]())
}
