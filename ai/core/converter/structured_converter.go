package converter

// StructuredConverter is a generic interface that combines the functionalities of both
// FormatProvider and Converter interfaces. It is parameterized with a type T, which can
// be any type as indicated by the `any` constraint. This interface requires implementing
// types to provide both a method for converting a raw string into a value of type T
// (from the Converter interface) and a method for retrieving a format string
// (from the FormatProvider interface). This allows for structured conversion processes
// that are aware of specific formatting requirements.
type StructuredConverter[T any] interface {
	FormatProvider
	Converter[T]
}
