package strings

import (
	"strings"
	"unicode"
)

// AsCamelCase creates a CamelCase type from a string.
// This is a convenience constructor function that wraps a string into the CamelCase type.
//
// Parameters:
//   - inputString: The string to convert to CamelCase type
//
// Returns:
//   - A CamelCase instance wrapping the input string
//
// Example:
//
//	camel := AsCamelCase("getUserName")
//	parts := camel.Split() // ["get", "User", "Name"]
func AsCamelCase(s string) CamelCase {
	return CamelCase(s)
}

// CamelCase represents a string in camelCase or PascalCase format.
// It provides methods to split and convert the camelCase string into different formats.
//
// CamelCase format examples:
//   - "getUserName" (lowerCamelCase)
//   - "GetUserName" (UpperCamelCase/PascalCase)
//   - "HTTPServer" (with abbreviations)
//   - "user123Name" (with numbers)
type CamelCase string

// String returns the underlying string value of the CamelCase.
// This implements the fmt.Stringer interface.
//
// Returns:
//   - The string representation of the CamelCase
func (c CamelCase) String() string {
	return string(c)
}

// SplitWith splits the camelCase string into separate words and applies an optional
// transformation function to each word.
//
// The splitting logic handles:
//   - Transitions between letters and non-letters (e.g., "user123" -> ["user", "123"])
//   - Transitions from lowercase to uppercase (e.g., "userName" -> ["user", "Name"])
//   - Uppercase sequences followed by lowercase (e.g., "HTTPServer" -> ["HTTP", "Server"])
//
// Parameters:
//   - transformFunc: Optional function to transform each split part.
//     Pass nil to skip transformation.
//
// Returns:
//   - A slice of strings representing the split words.
//     Returns nil if the input is empty.
//
// Example:
//
//	camel := AsCamelCase("getUserHTTPResponse")
//	parts := camel.SplitWith(strings.ToLower)
//	// Result: ["get", "user", "http", "response"]
func (c CamelCase) SplitWith(transformFunc func(string) string) []string {
	if c == "" {
		return nil
	}

	var (
		parts = make([]string, 0, len(c))
		runes = []rune(c)
		n     = len(runes)
		sb    strings.Builder
	)

	for i := 0; i < n; i++ {
		r := runes[i]
		sb.WriteRune(r)

		if i < n-1 {
			next := runes[i+1]

			// Rule 1: letter/non-letter transition
			if (unicode.IsLetter(r) && !unicode.IsLetter(next)) ||
				(!unicode.IsLetter(r) && unicode.IsLetter(next)) {
				parts = append(parts, sb.String())
				sb.Reset()
				continue
			}

			// Rule 2: lowercase to uppercase
			if unicode.IsLower(r) && unicode.IsUpper(next) {
				parts = append(parts, sb.String())
				sb.Reset()
				continue
			}

			// Rule 3: uppercase sequence before lowercase (e.g. "HTTPServer")
			if unicode.IsUpper(r) && unicode.IsUpper(next) &&
				i+2 < n && unicode.IsLower(runes[i+2]) {
				parts = append(parts, sb.String())
				sb.Reset()
				continue
			}
		}
	}

	if sb.Len() > 0 {
		parts = append(parts, sb.String())
	}

	if transformFunc != nil {
		for i := range parts {
			parts[i] = transformFunc(parts[i])
		}
	}

	return parts
}

// Split splits the camelCase string into separate words without transformation.
// This is equivalent to calling SplitWith(nil).
//
// Returns:
//   - A slice of strings representing the split words
//
// Example:
//
//	camel := AsCamelCase("getUserName")
//	parts := camel.Split() // ["get", "User", "Name"]
func (c CamelCase) Split() []string {
	return c.SplitWith(nil)
}

// SplitToLower splits the camelCase string and converts each word to lowercase.
// This is equivalent to calling SplitWith(strings.ToLower).
//
// Returns:
//   - A slice of lowercase strings representing the split words
//
// Example:
//
//	camel := AsCamelCase("getUserName")
//	parts := camel.SplitToLower() // ["get", "user", "name"]
func (c CamelCase) SplitToLower() []string {
	return c.SplitWith(strings.ToLower)
}

// SplitToUpper splits the camelCase string and converts each word to uppercase.
// This is equivalent to calling SplitWith(strings.ToUpper).
//
// Returns:
//   - A slice of uppercase strings representing the split words
//
// Example:
//
//	camel := AsCamelCase("getUserName")
//	parts := camel.SplitToUpper() // ["GET", "USER", "NAME"]
func (c CamelCase) SplitToUpper() []string {
	return c.SplitWith(strings.ToUpper)
}

// ToSnakeCase converts the camelCase string to snake_case format.
// The conversion process:
//  1. Splits the camelCase string into words
//  2. Filters out underscores and empty strings
//  3. Converts each word to lowercase
//  4. Joins words with underscores
//
// Returns:
//   - A SnakeCase instance representing the converted string
//
// Example:
//
//	camel := AsCamelCase("getUserHTTPResponse")
//	snake := camel.ToSnakeCase() // "get_user_http_response"
func (c CamelCase) ToSnakeCase() SnakeCase {
	if c == "" {
		return ""
	}

	words := c.Split()
	snakeWords := make([]string, 0, len(words))
	for _, w := range words {
		if w == "_" || w == "" {
			continue
		}
		snakeWords = append(snakeWords, strings.ToLower(w))
	}

	// Join words with underscores
	return AsSnakeCase(strings.Join(snakeWords, "_"))
}

// AsSnakeCase creates a SnakeCase type from a string.
// This is a convenience constructor function that wraps a string into the SnakeCase type.
//
// Parameters:
//   - inputString: The string to convert to SnakeCase type
//
// Returns:
//   - A SnakeCase instance wrapping the input string
//
// Example:
//
//	snake := AsSnakeCase("get_user_name")
//	parts := snake.Split() // ["get", "user", "name"]
func AsSnakeCase(s string) SnakeCase {
	return SnakeCase(s)
}

// SnakeCase represents a string in snake_case format.
// It provides methods to split and convert the snake_case string into different formats.
//
// Snake_case format examples:
//   - "get_user_name"
//   - "http_request_handler"
//   - "user_123_id"
type SnakeCase string

// String returns the underlying string value of the SnakeCase.
// This implements the fmt.Stringer interface.
//
// Returns:
//   - The string representation of the SnakeCase
func (s SnakeCase) String() string {
	return string(s)
}

// SplitWith splits the snake_case string by underscores and applies an optional
// transformation function to each word.
//
// Parameters:
//   - transformFunc: Optional function to transform each split part.
//     Pass nil to skip transformation.
//
// Returns:
//   - A slice of strings representing the split words.
//     Returns nil if the input is empty.
//
// Note:
//   - Consecutive underscores will result in empty string elements in the output
//   - Leading or trailing underscores will result in empty string elements
//
// Example:
//
//	snake := AsSnakeCase("get_user_name")
//	parts := snake.SplitWith(strings.ToUpper)
//	// Result: ["GET", "USER", "NAME"]
func (s SnakeCase) SplitWith(transformFunc func(string) string) []string {
	if s == "" {
		return nil
	}

	parts := strings.Split(s.String(), "_")

	if transformFunc != nil {
		for i, part := range parts {
			parts[i] = transformFunc(part)
		}
	}

	return parts
}

// Split splits the snake_case string by underscores without transformation.
// This is equivalent to calling SplitWith(nil).
//
// Returns:
//   - A slice of strings representing the split words
//
// Example:
//
//	snake := AsSnakeCase("get_user_name")
//	parts := snake.Split() // ["get", "user", "name"]
func (s SnakeCase) Split() []string {
	return s.SplitWith(nil)
}

// SplitToLower splits the snake_case string and converts each word to lowercase.
// This is equivalent to calling SplitWith(strings.ToLower).
//
// Returns:
//   - A slice of lowercase strings representing the split words
//
// Example:
//
//	snake := AsSnakeCase("GET_USER_NAME")
//	parts := snake.SplitToLower() // ["get", "user", "name"]
func (s SnakeCase) SplitToLower() []string {
	return s.SplitWith(strings.ToLower)
}

// SplitToUpper splits the snake_case string and converts each word to uppercase.
// This is equivalent to calling SplitWith(strings.ToUpper).
//
// Returns:
//   - A slice of uppercase strings representing the split words
//
// Example:
//
//	snake := AsSnakeCase("get_user_name")
//	parts := snake.SplitToUpper() // ["GET", "USER", "NAME"]
func (s SnakeCase) SplitToUpper() []string {
	return s.SplitWith(strings.ToUpper)
}

// ToCamelCase converts the snake_case string to camelCase format.
// The conversion process:
//  1. Splits the snake_case string by underscores
//  2. Converts all words to lowercase
//  3. Keeps the first word lowercase
//  4. Capitalizes the first letter of subsequent non-empty words
//  5. Joins all words together
//
// Returns:
//   - A CamelCase instance representing the converted string
//
// Note:
//   - Empty words (from consecutive underscores) are skipped
//   - The first word is always lowercase (lowerCamelCase format)
//
// Example:
//
//	snake := AsSnakeCase("get_user_http_response")
//	camel := snake.ToCamelCase() // "getUserHttpResponse"
func (s SnakeCase) ToCamelCase() CamelCase {
	if s == "" {
		return ""
	}

	words := s.SplitToLower()
	var sb strings.Builder

	for i, word := range words {
		if i == 0 {
			sb.WriteString(word)
			continue
		}
		if word == "" {
			continue
		}
		if len(word) == 1 {
			sb.WriteString(strings.ToUpper(word))
		} else {
			sb.WriteString(strings.ToUpper(word[:1]))
			sb.WriteString(word[1:])
		}
	}

	return AsCamelCase(sb.String())
}
