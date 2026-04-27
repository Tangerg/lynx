package strings

import (
	"strings"
	"unicode"
)

// CamelCase is a string in camelCase or PascalCase form. The methods on
// CamelCase split the value into its constituent words.
//
// Recognized boundaries:
//   - between letters and non-letters ("user123" → "user", "123")
//   - lower-to-upper transitions ("userName" → "user", "Name")
//   - end of an upper-case run before a lower-case rune
//     ("HTTPServer" → "HTTP", "Server")
type CamelCase string

// AsCamelCase wraps s as a [CamelCase].
func AsCamelCase(s string) CamelCase { return CamelCase(s) }

// String returns the underlying string.
func (c CamelCase) String() string { return string(c) }

// SplitWith splits c at word boundaries and applies fn to each part.
// If fn is nil the parts are returned unchanged. Empty input yields
// nil.
func (c CamelCase) SplitWith(fn func(string) string) []string {
	if c == "" {
		return nil
	}
	runes := []rune(c)
	n := len(runes)
	parts := make([]string, 0, n)
	var sb strings.Builder

	for i, r := range runes {
		sb.WriteRune(r)
		if i == n-1 {
			break
		}
		next := runes[i+1]
		switch {
		case unicode.IsLetter(r) != unicode.IsLetter(next):
			// letter↔non-letter
		case unicode.IsLower(r) && unicode.IsUpper(next):
			// camelCase hump
		case unicode.IsUpper(r) && unicode.IsUpper(next) &&
			i+2 < n && unicode.IsLower(runes[i+2]):
			// end of an UPPER run before a lower (HTTPServer)
		default:
			continue
		}
		parts = append(parts, sb.String())
		sb.Reset()
	}
	if sb.Len() > 0 {
		parts = append(parts, sb.String())
	}
	if fn != nil {
		for i := range parts {
			parts[i] = fn(parts[i])
		}
	}
	return parts
}

// Split splits c at word boundaries with no transformation.
func (c CamelCase) Split() []string { return c.SplitWith(nil) }

// SplitToLower splits c and lower-cases each word.
func (c CamelCase) SplitToLower() []string { return c.SplitWith(strings.ToLower) }

// SplitToUpper splits c and upper-cases each word.
func (c CamelCase) SplitToUpper() []string { return c.SplitWith(strings.ToUpper) }

// ToSnakeCase returns the snake_case form of c. Adjacent runs of
// underscores in the source are collapsed; empty parts are skipped.
//
// Example:
//
//	AsCamelCase("getUserHTTPResponse").ToSnakeCase() // "get_user_http_response"
func (c CamelCase) ToSnakeCase() SnakeCase {
	if c == "" {
		return ""
	}
	words := c.Split()
	out := make([]string, 0, len(words))
	for _, w := range words {
		if w == "" || w == "_" {
			continue
		}
		out = append(out, strings.ToLower(w))
	}
	return AsSnakeCase(strings.Join(out, "_"))
}

// SnakeCase is a string in snake_case form. Words are separated by
// underscore characters.
type SnakeCase string

// AsSnakeCase wraps s as a [SnakeCase].
func AsSnakeCase(s string) SnakeCase { return SnakeCase(s) }

// String returns the underlying string.
func (s SnakeCase) String() string { return string(s) }

// SplitWith splits s on underscores and applies fn to each part.
// Consecutive or leading/trailing underscores produce empty entries.
// Empty input returns nil.
func (s SnakeCase) SplitWith(fn func(string) string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(string(s), "_")
	if fn != nil {
		for i, p := range parts {
			parts[i] = fn(p)
		}
	}
	return parts
}

// Split splits s on underscores with no transformation.
func (s SnakeCase) Split() []string { return s.SplitWith(nil) }

// SplitToLower splits s and lower-cases each word.
func (s SnakeCase) SplitToLower() []string { return s.SplitWith(strings.ToLower) }

// SplitToUpper splits s and upper-cases each word.
func (s SnakeCase) SplitToUpper() []string { return s.SplitWith(strings.ToUpper) }

// ToCamelCase returns the camelCase form of s. The first word is kept
// lower-case (lowerCamelCase); subsequent words are title-cased. Empty
// segments are skipped.
//
// Example:
//
//	AsSnakeCase("get_user_http_response").ToCamelCase() // "getUserHttpResponse"
func (s SnakeCase) ToCamelCase() CamelCase {
	if s == "" {
		return ""
	}
	words := s.SplitToLower()
	var sb strings.Builder
	for i, w := range words {
		if w == "" {
			continue
		}
		if i == 0 {
			sb.WriteString(w)
			continue
		}
		sb.WriteString(strings.ToUpper(w[:1]))
		sb.WriteString(w[1:])
	}
	return AsCamelCase(sb.String())
}
