package maintenance

import (
	"slices"
	"testing"
)

func TestParseMemoryFactsCanonicalizesModelMarkdown(t *testing.T) {
	got := parseMemoryFacts("```\n- keep this\n* keep this\n1. numbered\nplain\nNO_FACTS\n```")
	want := []string{"- keep this", "- numbered", "- plain"}
	if !slices.Equal(got, want) {
		t.Fatalf("parsed facts = %#v, want %#v", got, want)
	}
}
