package main

import (
	"strings"
	"testing"
)

func TestTruncateOutput_NoTruncationWhenShort(t *testing.T) {
	in := "a\nb\nc"
	got := truncateOutput(in, 5)
	if got != in {
		t.Errorf("got %q, want %q", got, in)
	}
}

func TestTruncateOutput_DropsTrailingNewline(t *testing.T) {
	got := truncateOutput("a\nb\n", 5)
	if got != "a\nb" {
		t.Errorf("trailing newline should be trimmed; got %q", got)
	}
}

func TestTruncateOutput_TruncatesWithFooter(t *testing.T) {
	in := "line1\nline2\nline3\nline4\nline5\nline6"
	got := truncateOutput(in, 2)

	if !strings.HasPrefix(got, "line1\nline2\n") {
		t.Errorf("first two lines missing: %q", got)
	}
	if !strings.Contains(got, "+4 more") {
		t.Errorf("footer missing the +4 marker: %q", got)
	}
	if !strings.Contains(got, "--verbose") {
		t.Errorf("footer should hint --verbose: %q", got)
	}
	if strings.Contains(got, "line3") {
		t.Errorf("truncated content leaked: %q", got)
	}
}

func TestTruncateOutput_Empty(t *testing.T) {
	if got := truncateOutput("", 5); got != "" {
		t.Errorf("empty input → %q, want empty", got)
	}
	if got := truncateOutput("\n\n", 5); got != "" {
		t.Errorf("whitespace-only input → %q, want empty", got)
	}
}

func TestShortSessionID(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"abc", "abc"},
		{"abcdefgh", "abcdefgh"},
		{"12345678-90ab-cdef", "12345678"},
	}
	for _, c := range cases {
		if got := shortSessionID(c.in); got != c.want {
			t.Errorf("shortSessionID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
