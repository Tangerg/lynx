package converter

// FormatProvider provides instructions for how the output of a
// language generative should be formatted.
type FormatProvider interface {
	// GetFormat returns a string containing instructions for how the output of a language
	// generative should be formatted.
	GetFormat() string
}

// Converter converts the (raw) LLM output into a structured responses of type T.
type Converter[T any] interface {
	// Convert converts the raw LLM output string into a structured response of type T.
	Convert(raw string) (T, error)
}

// StructuredOutputConverter converts the (raw) LLM output into a structured responses of type T. The
// FormatProvider.GetFormat() method should provide the LLM prompt description of
// the desired format.
type StructuredOutputConverter[T any] interface {
	FormatProvider
	Converter[T]
}
