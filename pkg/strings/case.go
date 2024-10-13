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

	for i := 1; i < length-1; i++ {
		prev := runes[i-1]
		cur := runes[i]
		next := runes[i+1]

		sb.WriteRune(prev)

		if unicode.IsLower(prev) &&
			unicode.IsUpper(cur) {

			parts = append(parts, sb.String())
			sb.Reset()
			continue
		}
		if unicode.IsUpper(prev) &&
			unicode.IsUpper(cur) &&
			unicode.IsLower(next) {

			parts = append(parts, sb.String())
			sb.Reset()
			continue
		}

		if i == length-2 {
			sb.WriteRune(cur)
			sb.WriteRune(next)
			parts = append(parts, sb.String())
			sb.Reset()
		}
	}

	if predicate != nil {
		for i, _ := range parts {
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
	return strings.Join(CamelCaseSplitToLower(source), "_")
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
