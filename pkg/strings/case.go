package strings

import (
	"strings"
	"unicode"
)

func AsCamelCase(s string) CamelCase {
	return CamelCase(s)
}

type CamelCase string

func (c CamelCase) String() string {
	return string(c)
}

func (c CamelCase) SplitWith(predicate func(string) string) []string {
	if c == "" {
		return nil
	}

	var (
		parts  = make([]string, 0, len(c))
		runes  = []rune(c)
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
	words := c.Split()
	newWords := make([]string, 0, len(words))
	for _, word := range words {
		if word == "_" || word == "" {
			continue
		}
		newWords = append(newWords, strings.ToLower(word))
	}
	return AsSnakeCase(strings.Join(newWords, "_"))
}

func AsSnakeCase(s string) SnakeCase {
	return SnakeCase(s)
}

type SnakeCase string

func (s SnakeCase) String() string {
	return string(s)
}

func (s SnakeCase) SplitWith(predicate func(string) string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s.String(), "_")

	if predicate != nil {
		for i, part := range parts {
			parts[i] = predicate(part)
		}
	}

	return parts
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
	toLower := s.SplitToLower()
	var sb strings.Builder
	for i, lower := range toLower {
		if i == 0 {
			sb.WriteString(lower)
			continue
		}
		if lower == "" {
			continue
		}
		if len(lower) == 1 {
			sb.WriteString(strings.ToUpper(lower))
		} else {
			sb.WriteString(strings.ToUpper(lower[:1]))
			sb.WriteString(lower[1:])
		}
	}
	return AsCamelCase(sb.String())
}
