// Package fuzzy ranks candidates against what someone has typed.
//
// The matching is what people expect from a command palette: every character of the
// pattern has to appear in the candidate, in order, and how well it scores depends on
// where. A match at the start of a word counts for more than one in the middle of
// one, characters that run together count for more than characters spread out, and a
// pattern typed in the same case as the candidate counts for a little more than one
// that was not.
//
// It answers in byte offsets rather than character counts, because what asks is
// almost always about to draw the candidate with the matched characters picked out,
// and drawing walks a string by offset.
package fuzzy

import (
	"cmp"
	"slices"
	"unicode"
	"unicode/utf8"
)

// What a match is worth. The numbers matter only relative to each other: a word-start
// match is worth twice a plain one, a run of two adjacent characters beats the same
// two characters a word apart, and a gap costs enough to break ties without being
// able to push a real match below a worse one.
const (
	perCharacter     = 16
	bonusWordStart   = 32
	bonusConsecutive = 24
	bonusSameCase    = 8
	penaltyGap       = 3

	// bonusStart is what beginning the candidate is worth on top of beginning a word.
	// Without it "status_line" and "test_status" answer "st" equally well, and they do
	// not: the thing that starts with what was typed is the thing that was meant.
	bonusStart = 24
)

// Match is how well a candidate answered a pattern.
type Match struct {
	// Score is meaningful only against other scores for the same pattern. Higher is
	// better, and a match is never worth less than one, so that "matched at all" and
	// "did not match" cannot be confused.
	Score int
	// At is the byte offset in the candidate of each character the pattern matched,
	// ascending. An offset can fall inside a grapheme cluster — a pattern character
	// can match a combining mark — so something highlighting the matches should ask
	// whether a cluster contains an offset rather than begins at one.
	At []int
}

// Score matches pattern against candidate, case-insensitively, and reports whether
// it matched at all. An empty pattern matches everything, with nothing highlighted.
//
// # What it does not do
//
// It is not a full alignment search. Where a pattern character could bind to several
// places, this tries the first one and every one that begins a word, and keeps the
// best of those — which is what makes "st" find the "status" in "test_status" instead
// of settling for the "st" in "test". A placement that is better for some other
// reason, in a candidate with no word boundary to hint at it, can still be missed.
// That is a deliberate stopping point: candidates are palette-sized, patterns are
// short, and the alternative is a scoring matrix per candidate.
func Score(pattern, candidate string) (Match, bool) {
	if pattern == "" {
		return Match{}, true
	}
	first, _ := utf8.DecodeRuneInString(pattern)
	best, found := Match{}, false
	for _, from := range anchors(candidate, first) {
		if m, ok := scoreFrom(pattern, candidate, from); ok && (!found || m.Score > best.Score) {
			best, found = m, true
		}
	}
	return best, found
}

// anchors are the offsets worth starting a match at: the first place the pattern's
// first character appears, and every later place it appears at the start of a word.
//
// Anywhere else is not worth trying. A placement is better than an earlier one
// because it collects the word-start bonus, so the places that could win are the
// places that would collect it.
func anchors(candidate string, first rune) []int {
	var out []int
	prev := rune(-1)
	for at, r := range candidate {
		if folds(r, first) && (len(out) == 0 || at == 0 || opensWord(prev)) {
			out = append(out, at)
		}
		prev = r
	}
	return out
}

// scoreFrom binds each pattern character to the first place it appears at or after
// the last one, starting the search at from.
func scoreFrom(pattern, candidate string, from int) (Match, bool) {
	at := make([]int, 0, utf8.RuneCountInString(pattern))
	score := 0

	// index counts characters rather than bytes, so that the gap between two matched
	// characters is measured in what a reader sees.
	index, matchedAt := 0, -1
	prev := rune(-1)
	if from > 0 {
		prev, _ = utf8.DecodeLastRuneInString(candidate[:from])
	}

	rest := pattern
	for off, r := range candidate[from:] {
		if rest == "" {
			break
		}
		p, size := utf8.DecodeRuneInString(rest)
		if folds(r, p) {
			score += perCharacter
			if from+off == 0 || opensWord(prev) {
				score += bonusWordStart
			}
			if from+off == 0 {
				score += bonusStart
			}
			switch {
			case matchedAt < 0:
			case index == matchedAt+1:
				score += bonusConsecutive
			default:
				score -= penaltyGap * (index - matchedAt - 1)
			}
			if r == p {
				score += bonusSameCase
			}
			at = append(at, from+off)
			matchedAt = index
			rest = rest[size:]
		}
		prev = r
		index++
	}
	if rest != "" {
		return Match{}, false
	}
	return Match{Score: max(score, 1), At: at}, true
}

// folds reports whether two characters are the same ignoring case.
func folds(a, b rune) bool { return a == b || unicode.ToLower(a) == unicode.ToLower(b) }

// opensWord reports whether a character makes what follows it the start of a word.
//
// Separators only, not a case change: treating a capital as a word start would score
// "SQL" as three words, and the point of the bonus is to find the beginning of
// something a person would name.
func opensWord(prev rune) bool {
	switch prev {
	case ' ', '\t', '_', '-', '.', '/', ':':
		return true
	}
	return prev < 0
}

// Ranked is one candidate that matched, named by its position in what was searched.
//
// It is an index rather than the string so that a caller ranking its own items —
// files with a kind, commands with a description — gets back something it can look
// its own item up with, instead of having to match strings back up afterwards.
type Ranked struct {
	Index int
	Match Match
}

// Filter scores every candidate and returns those that matched, best first.
//
// Candidates that score the same stay in the order they were given, so a caller that
// sorted its items meaningfully keeps that order for ties — which is where a caller's
// own idea of importance belongs.
func Filter(pattern string, candidates []string) []Ranked {
	out := make([]Ranked, 0, len(candidates))
	for i, candidate := range candidates {
		if m, ok := Score(pattern, candidate); ok {
			out = append(out, Ranked{Index: i, Match: m})
		}
	}
	slices.SortStableFunc(out, func(a, b Ranked) int {
		return cmp.Compare(b.Match.Score, a.Match.Score)
	})
	return out
}
