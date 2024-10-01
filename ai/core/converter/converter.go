package converter

// Converter is a generic interface that defines a method for converting a raw string
// into a value of type T. The type T can be any type, as indicated by the `any` constraint.
// The Convert method takes a raw string as input and returns a value of type T along with
// an error. If the conversion is successful, the error will be nil; otherwise, it will
// contain information about what went wrong during the conversion process.
type Converter[T any] interface {
	Convert(raw string) (T, error)
}
