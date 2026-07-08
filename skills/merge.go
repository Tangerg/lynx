package skills

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

// Merge layers several resource sources into one. Earlier sources take precedence:
// on a name collision the first source that has the skill wins, so callers
// express precedence by order (e.g. a project source before a global one).
//
// nil sources are dropped. Merge of a single source returns it unchanged;
// Merge of none yields an empty source (List returns nothing, Load reports
// not found).
func Merge(sources ...ResourceSource) ResourceSource {
	kept := make([]ResourceSource, 0, len(sources))
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
	sources []ResourceSource
}

var _ Source = (*merged)(nil)
var _ ResourceSource = (*merged)(nil)

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

// Load returns the skill from the first source that has it. Missing skills are
// skipped; malformed skills return immediately so a broken higher-precedence
// copy is not silently masked by a lower one.
func (m *merged) Load(ctx context.Context, name string) (*Skill, error) {
	return firstOK(m.sources, name, func(src ResourceSource) (*Skill, error) {
		return src.Load(ctx, name)
	})
}

// OpenResource opens the resource from the first source that has it.
func (m *merged) OpenResource(ctx context.Context, name, resource string) (fs.File, error) {
	return firstOK(m.sources, name, func(src ResourceSource) (fs.File, error) {
		return src.OpenResource(ctx, name, resource)
	})
}

// firstOK returns the first source's successful result. Only not-exist errors
// fall through to lower-precedence sources; every other error is authoritative.
func firstOK[T any](sources []ResourceSource, name string, op func(ResourceSource) (T, error)) (T, error) {
	var zero T
	var errs []error
	for _, src := range sources {
		got, err := op(src)
		if err == nil {
			return got, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return zero, err
		}
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		return zero, fmt.Errorf("skills: %q not found: no sources", name)
	}
	return zero, errors.Join(errs...)
}
