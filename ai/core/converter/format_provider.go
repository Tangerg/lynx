package converter

// FormatProvider is an interface that defines a method for retrieving a format string.
// The GetFormat method returns a string that represents a specific format. This interface
// can be implemented by any type that needs to provide a format string, allowing for
// flexible and interchangeable formatting logic.
type FormatProvider interface {
	GetFormat() string
}
