package fuzzy

import (
	"slices"
	"testing"
)

func TestAPatternHasToBeASubsequence(t *testing.T) {
	for _, tc := range []struct {
		pattern, candidate string
		matches            bool
	}{
		{"", "anything", true},
		{"abc", "abc", true},
		{"abc", "a-b-c", true},
		{"abc", "acb", false},
		{"abcd", "abc", false},
		{"ABC", "abc", true},
		{"abc", "ABC", true},
		{"a", "", false},
	} {
		m, ok := Score(tc.pattern, tc.candidate)
		if ok != tc.matches {
			t.Errorf("Score(%q, %q) matched = %v, want %v", tc.pattern, tc.candidate, ok, tc.matches)
		}
		if ok && tc.pattern != "" && m.Score < 1 {
			t.Errorf("Score(%q, %q) = %d, want a match to be worth something",
				tc.pattern, tc.candidate, m.Score)
		}
	}
}

func TestMatchedOffsetsAreWhereTheCharactersAre(t *testing.T) {
	// They are byte offsets because what asks is about to draw the candidate with them
	// picked out, and drawing walks a string by offset.
	m, ok := Score("hé", "chèz hér")
	if !ok {
		t.Fatal("no match")
	}
	for _, at := range m.At {
		if at >= len("chèz hér") {
			t.Fatalf("offset %d is past the candidate", at)
		}
	}
	if got := "chèz hér"[m.At[0]:]; got[:1] != "h" {
		t.Fatalf("first offset points at %q, want the h", got[:1])
	}
	if !slices.IsSorted(m.At) {
		t.Fatalf("offsets %v are not ascending", m.At)
	}
}

func TestAWordStartBeatsTheMiddleOfAWord(t *testing.T) {
	// The case that makes a palette feel right or wrong: typing "st" should find the
	// status, not the st in test.
	m, ok := Score("st", "test_status")
	if !ok {
		t.Fatal("no match")
	}
	if got := m.At[0]; got != 5 {
		t.Fatalf("matched at %v, want the s that begins status, at offset 5", m.At)
	}
}

func TestAnEarlierPlacementWinsWhenNothingBeatsIt(t *testing.T) {
	// The greedy answer is the right one when there is no later word to prefer.
	m, ok := Score("ts", "test")
	if !ok {
		t.Fatal("no match")
	}
	if want := []int{0, 2}; !slices.Equal(m.At, want) {
		t.Fatalf("matched at %v, want %v", m.At, want)
	}
}

func TestCharactersThatRunTogetherScoreBest(t *testing.T) {
	together, _ := Score("ab", "xxab")
	apart, _ := Score("ab", "xaxxb")
	if together.Score <= apart.Score {
		t.Fatalf("run together = %d, spread out = %d, want the run to win",
			together.Score, apart.Score)
	}
}

func TestAWordStartScoresBest(t *testing.T) {
	start, _ := Score("s", "a_start")
	middle, _ := Score("s", "abcsdef")
	if start.Score <= middle.Score {
		t.Fatalf("word start = %d, middle = %d, want the word start to win",
			start.Score, middle.Score)
	}
}

func TestTheSameCaseIsWorthALittleMore(t *testing.T) {
	same, _ := Score("S", "Session")
	other, _ := Score("s", "Session")
	if same.Score <= other.Score {
		t.Fatalf("same case = %d, other case = %d", same.Score, other.Score)
	}
}

func TestSeparatorsAreWhatStartsAWordAndNotCase(t *testing.T) {
	// Treating a capital as a word start would score "SQL" as three words, and the
	// bonus is meant to find the beginning of something a person would name.
	for _, prev := range []rune{' ', '\t', '_', '-', '.', '/', ':'} {
		if !opensWord(prev) {
			t.Errorf("%q does not open a word", prev)
		}
	}
	for _, prev := range []rune{'a', 'Z', '0', '(', '\''} {
		if opensWord(prev) {
			t.Errorf("%q opens a word", prev)
		}
	}
}

func TestFilterRanksBestFirst(t *testing.T) {
	candidates := []string{"unrelated", "test_status", "status_line", "stats"}
	got := Filter("st", candidates)
	if len(got) != 3 {
		t.Fatalf("%d matched, want 3 (%q must not)", len(got), "unrelated")
	}
	if best := candidates[got[0].Index]; best != "status_line" && best != "stats" {
		t.Fatalf("best = %q, want one that begins with the pattern", best)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Match.Score < got[i].Match.Score {
			t.Fatalf("results are not ordered: %v", got)
		}
	}
}

func TestFilterKeepsTheOrderItWasGivenForTies(t *testing.T) {
	// Which is where a caller's own idea of importance belongs: it sorted its items
	// before asking, and equal scores must not undo that.
	candidates := []string{"ab", "ab", "ab"}
	got := Filter("ab", candidates)
	if len(got) != 3 {
		t.Fatalf("%d matched, want 3", len(got))
	}
	for i, r := range got {
		if r.Index != i {
			t.Fatalf("tie order = %d at %d, want the order it was given", r.Index, i)
		}
	}
}

func TestAnEmptyPatternMatchesEverythingAndHighlightsNothing(t *testing.T) {
	m, ok := Score("", "anything")
	if !ok || len(m.At) != 0 || m.Score != 0 {
		t.Fatalf("Score(\"\", …) = %+v, %v", m, ok)
	}
	if got := Filter("", []string{"a", "b"}); len(got) != 2 {
		t.Fatalf("%d matched an empty pattern, want everything", len(got))
	}
}

func TestNothingMatchesNothing(t *testing.T) {
	if got := Filter("zz", []string{"a", "b"}); len(got) != 0 {
		t.Fatalf("%d matched, want none", len(got))
	}
}
