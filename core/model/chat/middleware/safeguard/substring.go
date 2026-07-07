package safeguard

import (
	"context"
	"strings"
)

// SubstringMatcherOptions configures [NewSubstringMatcher]. All
// fields default to false (the zero value) so a bare
// `SubstringMatcherOptions{}` produces the safe, verbose default.
type SubstringMatcherOptions struct {
	// CaseSensitive controls whether matching honors letter case.
	// Default false (terms compared after [strings.ToLower]).
	CaseSensitive bool

	// HideMatch suppresses the matched term in the resulting
	// [ErrUnsafeContent]. Default false: the matched term is
	// disclosed (useful for debugging). Set true when the term list
	// is sensitive (e.g. internal keyword detector you don't want
	// echoed back to callers).
	HideMatch bool
}

// NewSubstringMatcher is the stdlib-backed default [Matcher] —
// returns a hit when any term is contained in the scanned text.
// Empty / whitespace terms are dropped at construction time so a
// caller's "" sentinel never matches the universe.
//
// Performance: O(N×len(text)) per scan, no allocations beyond the
// terms slice itself. For large term lists or hot paths, plug in a
// dedicated multi-string matcher (Aho-Corasick, regex set) via the
// [Matcher] interface — substring is the safe-by-default minimum.
func NewSubstringMatcher(terms []string, opts ...SubstringMatcherOptions) Matcher {
	var opt SubstringMatcherOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	cleaned := make([]string, 0, len(terms))
	for _, t := range terms {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if !opt.CaseSensitive {
			t = strings.ToLower(t)
		}
		cleaned = append(cleaned, t)
	}
	return &substringMatcher{terms: cleaned, opts: opt}
}

type substringMatcher struct {
	terms []string
	opts  SubstringMatcherOptions
}

func (m *substringMatcher) Match(_ context.Context, text string) (string, bool) {
	if len(m.terms) == 0 || text == "" {
		return "", false
	}
	hay := text
	if !m.opts.CaseSensitive {
		hay = strings.ToLower(hay)
	}
	for _, t := range m.terms {
		if strings.Contains(hay, t) {
			if m.opts.HideMatch {
				return "", true
			}
			return t, true
		}
	}
	return "", false
}
