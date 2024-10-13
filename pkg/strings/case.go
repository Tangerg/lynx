package strings

import (
	"strings"
	"unicode"
)

func CamelCaseSplitWith(source string, predicate func(string) string) []string {
	if source == "" {
		return nil
	}

	var (
		parts  = make([]string, 0, len(source))
		runes  = []rune(source)
		length = len(runes)
		sb     strings.Builder
	)

	for i := 0; i < length; i++ {
		cur := runes[i]
		sb.WriteRune(cur)

		if i < length-1 {
			next := runes[i+1]

			if (unicode.IsLetter(cur) && !unicode.IsLetter(next)) ||
				(!unicode.IsLetter(cur) && unicode.IsLetter(next)) {
				parts = append(parts, sb.String())
				sb.Reset()
				continue
			}

			if unicode.IsLower(cur) && unicode.IsUpper(next) {
				parts = append(parts, sb.String())
				sb.Reset()
				continue
			}

			if unicode.IsUpper(cur) && unicode.IsUpper(next) && i+2 < length && unicode.IsLower(runes[i+2]) {
				parts = append(parts, sb.String())
				sb.Reset()
				continue
			}
		}
	}

	if sb.Len() > 0 {
		parts = append(parts, sb.String())
	}

	if predicate != nil {
		for i := range parts {
			parts[i] = predicate(parts[i])
		}
	}

	return parts
}
func CamelCaseSplit(source string) []string {
	return CamelCaseSplitWith(source, nil)
}
func CamelCaseSplitToLower(source string) []string {
	return CamelCaseSplitWith(source, strings.ToLower)
}
func CamelCaseSplitToUpper(source string) []string {
	return CamelCaseSplitWith(source, strings.ToUpper)
}
func CamelCaseToSnakeCase(source string) string {
	if source == "" {
		return ""
	}
	words := CamelCaseSplitWith(source, nil)
	newWords := make([]string, 0, len(words))
	for _, word := range words {
		if word == "_" || word == "" {
			continue
		}
		newWords = append(newWords, strings.ToLower(word))
	}
	return strings.Join(newWords, "_")
}

func SnakeCaseSplitWith(source string, predicate func(string) string) []string {
	if source == "" {
		return nil
	}
	parts := strings.Split(source, "_")

	if predicate != nil {
		for i, part := range parts {
			parts[i] = predicate(part)
		}
	}

	return parts
}
func SnakeCaseSplit(source string) []string {
	return SnakeCaseSplitWith(source, nil)
}
func SnakeCaseSplitToLower(source string) []string {
	return SnakeCaseSplitWith(source, strings.ToLower)
}
func SnakeCaseSplitToUpper(source string) []string {
	return SnakeCaseSplitWith(source, strings.ToUpper)
}
func SnakeCaseToCamelCase(source string) string {
	if source == "" {
		return ""
	}
	toLower := SnakeCaseSplitToLower(source)
	var sb strings.Builder
	for i, s := range toLower {
		if i == 0 {
			sb.WriteString(s)
			continue
		}
		if s == "" {
			continue
		}
		if len(s) == 1 {
			sb.WriteString(strings.ToUpper(s))
		} else {
			sb.WriteString(strings.ToUpper(s[:1]))
			sb.WriteString(s[1:])
		}
	}
	return sb.String()
}
