package strings

// IsQuoted reports whether s starts and ends with the same quote
// character, " or '. It examines only the first and last bytes; nested
// or escaped quotes inside s are not considered.
//
// Strings shorter than two bytes return false.
func IsQuoted(s string) bool {
	if len(s) < 2 {
		return false
	}
	first, last := s[0], s[len(s)-1]
	return first == last && (first == '"' || first == '\'')
}

// UnQuote returns s without its surrounding matching quotes, as
// recognized by [IsQuoted]. If s is not quoted, it is returned
// unchanged. UnQuote does not process escape sequences within s.
//
// Example:
//
//	UnQuote(`"hello"`) // "hello"
//	UnQuote(`'a'`)     // "a"
//	UnQuote(`hello`)   // "hello" (unchanged)
func UnQuote(s string) string {
	if !IsQuoted(s) {
		return s
	}
	return s[1 : len(s)-1]
}
