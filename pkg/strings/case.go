package strings

import (
	"strings"
	"unicode"
)

func AsCamelCase(inputString string) CamelCase {
	return CamelCase(inputString)
}

type CamelCase string

func (c CamelCase) String() string {
	return string(c)
}

func (c CamelCase) SplitWith(transformFunc func(string) string) []string {
	if c == "" {
		return nil
	}

	var (
		splitParts    = make([]string, 0, len(c))
		inputRunes    = []rune(c)
		totalLength   = len(inputRunes)
		stringBuilder strings.Builder
	)

	for currentIndex := 0; currentIndex < totalLength; currentIndex++ {
		currentRune := inputRunes[currentIndex]
		stringBuilder.WriteRune(currentRune)

		if currentIndex < totalLength-1 {
			nextRune := inputRunes[currentIndex+1]

			// Check for letter to non-letter or non-letter to letter transition
			if (unicode.IsLetter(currentRune) && !unicode.IsLetter(nextRune)) ||
				(!unicode.IsLetter(currentRune) && unicode.IsLetter(nextRune)) {
				splitParts = append(splitParts, stringBuilder.String())
				stringBuilder.Reset()
				continue
			}

			// Check for lowercase to uppercase transition
			if unicode.IsLower(currentRune) && unicode.IsUpper(nextRune) {
				splitParts = append(splitParts, stringBuilder.String())
				stringBuilder.Reset()
				continue
			}

			// Check for uppercase sequence followed by lowercase
			if unicode.IsUpper(currentRune) && unicode.IsUpper(nextRune) &&
				currentIndex+2 < totalLength && unicode.IsLower(inputRunes[currentIndex+2]) {
				splitParts = append(splitParts, stringBuilder.String())
				stringBuilder.Reset()
				continue
			}
		}
	}

	// Add the remaining part if any
	if stringBuilder.Len() > 0 {
		splitParts = append(splitParts, stringBuilder.String())
	}

	// Apply transformation function if provided
	if transformFunc != nil {
		for partIndex := range splitParts {
			splitParts[partIndex] = transformFunc(splitParts[partIndex])
		}
	}

	return splitParts
}

func (c CamelCase) Split() []string {
	return c.SplitWith(nil)
}

func (c CamelCase) SplitToLower() []string {
	return c.SplitWith(strings.ToLower)
}

func (c CamelCase) SplitToUpper() []string {
	return c.SplitWith(strings.ToUpper)
}

func (c CamelCase) ToSnakeCase() SnakeCase {
	if c == "" {
		return ""
	}

	// Split camelCase into words
	camelWords := c.Split()

	// Filter and convert words to lowercase
	snakeWords := make([]string, 0, len(camelWords))
	for _, word := range camelWords {
		if word == "_" || word == "" {
			continue
		}
		snakeWords = append(snakeWords, strings.ToLower(word))
	}

	// Join words with underscores
	return AsSnakeCase(strings.Join(snakeWords, "_"))
}

func AsSnakeCase(inputString string) SnakeCase {
	return SnakeCase(inputString)
}

type SnakeCase string

func (s SnakeCase) String() string {
	return string(s)
}

func (s SnakeCase) SplitWith(transformFunc func(string) string) []string {
	if s == "" {
		return nil
	}

	// Split by underscore
	splitParts := strings.Split(s.String(), "_")

	// Apply transformation function if provided
	if transformFunc != nil {
		for partIndex, part := range splitParts {
			splitParts[partIndex] = transformFunc(part)
		}
	}

	return splitParts
}

func (s SnakeCase) Split() []string {
	return s.SplitWith(nil)
}

func (s SnakeCase) SplitToLower() []string {
	return s.SplitWith(strings.ToLower)
}

func (s SnakeCase) SplitToUpper() []string {
	return s.SplitWith(strings.ToUpper)
}

func (s SnakeCase) ToCamelCase() CamelCase {
	if s == "" {
		return ""
	}

	// Split and convert to lowercase
	lowercaseWords := s.SplitToLower()
	var camelBuilder strings.Builder

	// Build camelCase string
	for wordIndex, lowercaseWord := range lowercaseWords {
		// First word remains lowercase
		if wordIndex == 0 {
			camelBuilder.WriteString(lowercaseWord)
			continue
		}

		// Skip empty words
		if lowercaseWord == "" {
			continue
		}

		// Capitalize first letter of subsequent words
		if len(lowercaseWord) == 1 {
			camelBuilder.WriteString(strings.ToUpper(lowercaseWord))
		} else {
			camelBuilder.WriteString(strings.ToUpper(lowercaseWord[:1]))
			camelBuilder.WriteString(lowercaseWord[1:])
		}
	}

	return AsCamelCase(camelBuilder.String())
}
