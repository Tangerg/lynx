package skills

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Merge layers several sources into one. Earlier sources take precedence:
// on a name collision the first source that has the skill wins, so callers
// express precedence by order (e.g. a project source before a global one).
//
// nil sources are dropped. Merge of a single source returns it unchanged;
// Merge of none yields an empty source (List returns nothing, Load reports
// not found).
func Merge(sources ...Source) Source {
	kept := make([]Source, 0, len(sources))
	for _, s := range sources {
		if s != nil {
			kept = append(kept, s)
		}
	}
	if len(kept) == 1 {
		return kept[0]
	}
	return &merged{sources: kept}
}

// merged is the [Merge] result: a Source that fans each operation across its
// backing sources in precedence order.
type merged struct {
	sources []Source
}

var _ Source = (*merged)(nil)

// List unions every source's summaries, keeping the first occurrence of each
// name (precedence by source order) and sorting the result by name.
func (m *merged) List(ctx context.Context) ([]Summary, error) {
	var out []Summary
	seen := make(map[string]struct{})
	for _, src := range m.sources {
		summaries, err := src.List(ctx)
		if err != nil {
			return nil, err
		}
		for _, s := range summaries {
			if _, dup := seen[s.Name]; dup {
				continue // a higher-precedence source already provided this name
			}
			seen[s.Name] = struct{}{}
			out = append(out, s)
		}
	}
	slices.SortFunc(out, func(a, b Summary) int {
		return strings.Compare(a.Name, b.Name)
	})
	return out, nil
}

// Load returns the skill from the first source that has it. A source that
// lacks the skill (or whose copy fails to parse/validate) is skipped, so a
// valid lower-precedence copy still wins over a broken higher one.
func (m *merged) Load(ctx context.Context, name string) (*Skill, error) {
	return firstOK(m.sources, name, func(src Source) (*Skill, error) {
		return src.Load(ctx, name)
	})
}

// LoadResource reads the resource from the first source that can serve it,
// with the same skip-on-failure semantics as Load.
func (m *merged) LoadResource(ctx context.Context, name, resource string) ([]byte, error) {
	return firstOK(m.sources, name, func(src Source) ([]byte, error) {
		return src.LoadResource(ctx, name, resource)
	})
}

// firstOK returns the first source's successful result. When every source
// fails it joins ALL of their errors — so a real failure in an early source
// isn't masked by a later source's not-found (a not-found-style error when
// there are no sources at all).
func firstOK[T any](sources []Source, name string, op func(Source) (T, error)) (T, error) {
	var zero T
	var errs []error
	for _, src := range sources {
		got, err := op(src)
		if err == nil {
			return got, nil
		}
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		return zero, fmt.Errorf("skills: %q not found: no sources", name)
	}
	return zero, errors.Join(errs...)
}
