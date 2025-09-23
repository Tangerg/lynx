package strings

func IsQuoted(s string) bool {
	if len(s) < 2 {
		return false
	}

	start := s[0:1]
	end := s[len(s)-1:]

	return (start == "\"" && end == "\"") ||
		(start == "'" && end == "'")
}

func UnQuote(s string) string {
	if !IsQuoted(s) {
		return s
	}
	return s[1 : len(s)-1]
}
