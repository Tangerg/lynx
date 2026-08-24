package gitprocess

import (
	"strings"
	"testing"
)

func TestBoundedTextDrainsAndMarksOverflow(t *testing.T) {
	text := boundedText{limit: 4}
	written, err := text.Write([]byte("abcdef"))
	if err != nil || written != 6 {
		t.Fatalf("Write = %d, %v; want full drain", written, err)
	}
	if got := text.String(); !strings.HasPrefix(got, "abcd") || !strings.Contains(got, "truncated") {
		t.Fatalf("bounded stderr = %q, want retained prefix and marker", got)
	}
}
