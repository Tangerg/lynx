package maintenance

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitizeTitle(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"plain", "Fix the login bug", "Fix the login bug"},
		{"surrounding double quotes", `"Refactor Auth Module"`, "Refactor Auth Module"},
		{"backticks", "`Wire OAuth Flow`", "Wire OAuth Flow"},
		{"trailing period", "Add Dark Mode.", "Add Dark Mode"},
		{"first line only", "Improve Streaming\n(notes about other stuff)", "Improve Streaming"},
		{"surrounding whitespace", "  Tidy Imports  ", "Tidy Imports"},
		{"blank", "   \n  ", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sanitizeTitle(c.in); got != c.want {
				t.Fatalf("sanitizeTitle(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestSanitizeTitleCapsRunes(t *testing.T) {
	got := sanitizeTitle(strings.Repeat("word ", 40)) // ~200 chars, > titleMaxRunes
	if n := utf8.RuneCountInString(got); n > titleMaxRunes {
		t.Fatalf("title not capped to %d runes: got %d (%q)", titleMaxRunes, n, got)
	}
}
