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
func AsCamelCase(inputString string) CamelCase {
	return CamelCase(inputString)
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
		// Pre-allocate slice with capacity equal to input length
		// This is an upper bound estimate to reduce allocations
		splitParts = make([]string, 0, len(c))
		// Convert string to runes to properly handle Unicode characters
		inputRunes = []rune(c)
		// Total number of runes in the input
		totalLength = len(inputRunes)
		// Builder to efficiently construct each word
		stringBuilder strings.Builder
	)

	// Iterate through each rune in the input
	for currentIndex := 0; currentIndex < totalLength; currentIndex++ {
		currentRune := inputRunes[currentIndex]
		stringBuilder.WriteRune(currentRune)

		// Check if we need to split at the current position
		if currentIndex < totalLength-1 {
			nextRune := inputRunes[currentIndex+1]

			// Rule 1: Split on letter to non-letter or non-letter to letter transition
			// Examples: "user123" -> ["user", "123"], "123user" -> ["123", "user"]
			if (unicode.IsLetter(currentRune) && !unicode.IsLetter(nextRune)) ||
				(!unicode.IsLetter(currentRune) && unicode.IsLetter(nextRune)) {
				splitParts = append(splitParts, stringBuilder.String())
				stringBuilder.Reset()
				continue
			}

			// Rule 2: Split on lowercase to uppercase transition
			// Example: "userName" -> ["user", "Name"]
			if unicode.IsLower(currentRune) && unicode.IsUpper(nextRune) {
				splitParts = append(splitParts, stringBuilder.String())
				stringBuilder.Reset()
				continue
			}

			// Rule 3: Split on uppercase sequence followed by lowercase
			// This handles abbreviations like "HTTPServer" -> ["HTTP", "Server"]
			// We split before the last uppercase letter if it's followed by lowercase
			if unicode.IsUpper(currentRune) && unicode.IsUpper(nextRune) &&
				currentIndex+2 < totalLength && unicode.IsLower(inputRunes[currentIndex+2]) {
				splitParts = append(splitParts, stringBuilder.String())
				stringBuilder.Reset()
				continue
			}
		}
	}

	// Add the remaining part if any characters are left in the builder
	if stringBuilder.Len() > 0 {
		splitParts = append(splitParts, stringBuilder.String())
	}

	// Apply transformation function to each part if provided
	if transformFunc != nil {
		for partIndex := range splitParts {
			splitParts[partIndex] = transformFunc(splitParts[partIndex])
		}
	}

	return splitParts
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

	// Split camelCase into words
	camelWords := c.Split()

	// Filter and convert words to lowercase
	snakeWords := make([]string, 0, len(camelWords))
	for _, word := range camelWords {
		// Skip underscores and empty strings
		if word == "_" || word == "" {
			continue
		}
		snakeWords = append(snakeWords, strings.ToLower(word))
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
func AsSnakeCase(inputString string) SnakeCase {
	return SnakeCase(inputString)
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

	// Split by underscore
	splitParts := strings.Split(s.String(), "_")

	// Apply transformation function to each part if provided
	if transformFunc != nil {
		for partIndex, part := range splitParts {
			splitParts[partIndex] = transformFunc(part)
		}
	}

	return splitParts
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

	// Split and convert to lowercase
	lowercaseWords := s.SplitToLower()
	var camelBuilder strings.Builder

	// Build camelCase string
	for wordIndex, lowercaseWord := range lowercaseWords {
		// First word remains lowercase (lowerCamelCase convention)
		if wordIndex == 0 {
			camelBuilder.WriteString(lowercaseWord)
			continue
		}

		// Skip empty words (from consecutive underscores)
		if lowercaseWord == "" {
			continue
		}

		// Capitalize first letter of subsequent words
		if len(lowercaseWord) == 1 {
			// Handle single character words
			camelBuilder.WriteString(strings.ToUpper(lowercaseWord))
		} else {
			// Capitalize first letter, keep rest lowercase
			camelBuilder.WriteString(strings.ToUpper(lowercaseWord[:1]))
			camelBuilder.WriteString(lowercaseWord[1:])
		}
	}

	return AsCamelCase(camelBuilder.String())
}
