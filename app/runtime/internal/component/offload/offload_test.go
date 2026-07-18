package offload

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPlaceholderRoundTrip(t *testing.T) {
	body := strings.Repeat("x", 500)
	ph := Placeholder(body, "ABC234XYZ7", "read_tool_result", 100)

	if len(ph) >= len(body) {
		t.Fatalf("placeholder (%d) not smaller than body (%d)", len(ph), len(body))
	}
	if !strings.HasPrefix(ph, "xxx") || !strings.HasSuffix(ph, "xxx") {
		t.Fatalf("placeholder dropped the head/tail preview: %q", ph)
	}
	for _, want := range []string{"500 bytes", "read_tool_result", "ABC234XYZ7"} {
		if !strings.Contains(ph, want) {
			t.Errorf("placeholder missing %q:\n%s", want, ph)
		}
	}
	id, ok := ID(ph)
	if !ok || id != "ABC234XYZ7" {
		t.Fatalf("ID(placeholder) = (%q, %v), want (ABC234XYZ7, true)", id, ok)
	}
}

func TestIDIgnoresOrdinaryResults(t *testing.T) {
	for _, s := range []string{
		"",
		"just some tool output",
		`{"id":"lowercase-not-base32"}`, // not preceded by the offload marker
		`here is a json blob {"id":"ABC234"} fyi`, // has an id object but no "bytes offloaded" anchor
	} {
		if _, ok := ID(s); ok {
			t.Errorf("ID(%q) matched, want no match", s)
		}
	}
}

func TestPlaceholderRuneSafe(t *testing.T) {
	// A body full of multibyte runes: the head/tail cuts must not split one.
	body := strings.Repeat("é", 500) // 2 bytes each = 1000 bytes
	ph := Placeholder(body, "ABCDE234", "read_tool_result", 100)
	if !utf8.ValidString(ph) {
		t.Fatal("placeholder split a multibyte rune (invalid UTF-8)")
	}
	if id, ok := ID(ph); !ok || id != "ABCDE234" {
		t.Fatalf("ID = (%q,%v)", id, ok)
	}
}
