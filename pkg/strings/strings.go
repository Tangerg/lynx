package strings

// IsQuoted checks if a string is wrapped with matching quotes (either double or single).
// A string is considered quoted if:
//   - It has at least 2 characters
//   - The first and last characters are both double quotes ("), OR
//   - The first and last characters are both single quotes (')
//
// This function only checks the first and last characters, it does not validate
// if quotes are properly escaped or balanced within the string.
//
// Parameters:
//   - s: The string to check
//
// Returns:
//   - true if the string starts and ends with matching quotes
//   - false otherwise (including empty strings, single characters, or mismatched quotes)
//
// Examples:
//
//	IsQuoted(`"hello"`)      // true  - double quoted
//	IsQuoted(`'hello'`)      // true  - single quoted
//	IsQuoted(`"hello'`)      // false - mismatched quotes
//	IsQuoted(`hello`)        // false - not quoted
//	IsQuoted(`"`)            // false - single character
//	IsQuoted(`""`)           // true  - empty string with quotes
//	IsQuoted(``)             // false - empty string
//	IsQuoted(`"it's"`)       // true  - double quoted (inner quote ignored)
func IsQuoted(s string) bool {
	// Quick check: a quoted string must have at least 2 characters (opening and closing quotes)
	if len(s) < 2 {
		return false
	}

	// Extract the first character (opening quote candidate)
	start := s[0:1]
	// Extract the last character (closing quote candidate)
	end := s[len(s)-1:]

	// Check if both first and last characters are matching quotes
	// Either both double quotes or both single quotes
	return (start == "\"" && end == "\"") ||
		(start == "'" && end == "'")
}

// UnQuote removes the surrounding quotes from a string if it is quoted.
// This function uses IsQuoted to determine if the string has matching quotes,
// and if so, returns the string with the first and last characters removed.
//
// If the string is not quoted (according to IsQuoted), the original string
// is returned unchanged.
//
// Parameters:
//   - s: The string to unquote
//
// Returns:
//   - The string without surrounding quotes if it was quoted
//   - The original string if it was not quoted
//
// Note:
//   - This function does not handle escaped quotes within the string
//   - For nested quotes like `""hello""`, only the outer quotes are removed
//   - Empty quoted strings like `""` or `”` will return an empty string
//
// Examples:
//
//	UnQuote(`"hello"`)       // "hello"  - removes double quotes
//	UnQuote(`'hello'`)       // "hello"  - removes single quotes
//	UnQuote(`"hello'`)       // `"hello'` - mismatched, returns unchanged
//	UnQuote(`hello`)         // "hello"  - not quoted, returns unchanged
//	UnQuote(`""`)            // ""       - empty quoted string
//	UnQuote(`"it's fine"`)   // "it's fine" - inner quote preserved
//	UnQuote(`""hello""`)     // `"hello"` - only outer quotes removed
func UnQuote(s string) string {
	// Check if the string has matching quotes
	if !IsQuoted(s) {
		// Not quoted, return as-is
		return s
	}

	// Remove the first and last characters (the quotes)
	// This creates a substring from index 1 to len(s)-1 (exclusive)
	return s[1 : len(s)-1]
}
