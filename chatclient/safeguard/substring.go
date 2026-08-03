package safeguard

import (
	"context"
	"fmt"
	"strings"
)

// SubstringOptions controls matching and disclosure. Case-insensitive
// matching is the default. HideMatch prevents a configured term from entering
// UnsafeError or OnBlock.
type SubstringOptions struct {
	CaseSensitive bool
	HideMatch     bool
}

// SubstringMatcher is an immutable stdlib matcher for small term sets.
type SubstringMatcher struct {
	terms   []substringTerm
	options SubstringOptions
}

type substringTerm struct {
	display string
	match   string
}

// NewSubstringMatcher validates, trims, de-duplicates, and snapshots terms.
// At least one non-empty term is required.
func NewSubstringMatcher(terms []string, options SubstringOptions) (*SubstringMatcher, error) {
	cleaned := make([]substringTerm, 0, len(terms))
	seen := make(map[string]struct{}, len(terms))
	for _, term := range terms {
		display := strings.TrimSpace(term)
		if display == "" {
			continue
		}
		match := display
		if !options.CaseSensitive {
			match = strings.ToLower(match)
		}
		if _, duplicate := seen[match]; duplicate {
			continue
		}
		seen[match] = struct{}{}
		cleaned = append(cleaned, substringTerm{display: display, match: match})
	}
	if len(cleaned) == 0 {
		return nil, fmt.Errorf("%w: at least one non-empty substring is required", ErrInvalidConfig)
	}
	return &SubstringMatcher{terms: cleaned, options: options}, nil
}

// Match reports the first configured term contained in text.
func (m *SubstringMatcher) Match(ctx context.Context, text string) (Match, error) {
	if err := ctx.Err(); err != nil {
		return Match{}, err
	}
	if text == "" {
		return Match{}, nil
	}
	haystack := text
	if !m.options.CaseSensitive {
		haystack = strings.ToLower(haystack)
	}
	for _, term := range m.terms {
		if !strings.Contains(haystack, term.match) {
			continue
		}
		if m.options.HideMatch {
			return Match{Found: true}, nil
		}
		return Match{Term: term.display, Found: true}, nil
	}
	return Match{}, nil
}
