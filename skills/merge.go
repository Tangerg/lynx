package skills

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

// Merge layers several resource sources into one. Earlier sources take
// precedence: on a name collision the first source that has the skill wins, so
// callers express precedence by order (e.g. a project source before a global
// one). The winning source owns the complete skill bundle; missing resources
// do not fall through to a lower-precedence copy with the same name.
//
// Nil and typed-nil sources are dropped. Merge of a single source returns it
// unchanged; Merge of none yields an empty source (List returns nothing, Load
// reports not found).
func Merge(sources ...ResourceSource) ResourceSource {
	kept := make([]ResourceSource, 0, len(sources))
	for _, s := range sources {
		if !isNil(s) {
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
	if err := contextError(ctx, "list"); err != nil {
		return nil, err
	}
	var out []Summary
	seen := make(map[string]struct{})
	for _, src := range m.sources {
		if err := contextError(ctx, "list"); err != nil {
			return nil, err
		}
		summaries, err := src.List(ctx)
		if ctxErr := contextError(ctx, "list"); ctxErr != nil {
			return nil, ctxErr
		}
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
	if err := validateName(name); err != nil {
		return nil, err
	}
	_, skill, err := m.find(ctx, name, fmt.Sprintf("load %q", name))
	return skill, err
}

// OpenResource opens a resource from the source that owns the winning skill.
// A lower-precedence copy must never contribute files to a higher-precedence
// skill with the same name.
func (m *merged) OpenResource(ctx context.Context, name, resource string) (fs.File, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	if err := validateResourcePath(resource); err != nil {
		return nil, err
	}
	operation := fmt.Sprintf("open resource %q/%q", name, resource)
	src, _, err := m.find(ctx, name, operation)
	if err != nil {
		return nil, err
	}
	file, err := src.OpenResource(ctx, name, resource)
	return checkedResourceFile(ctx, operation, name, resource, file, err)
}

// find returns the first source that owns name and the skill it loaded. Only
// not-exist errors fall through; malformed higher-precedence skills remain
// authoritative rather than being silently shadowed by a lower source.
func (m *merged) find(ctx context.Context, name, operation string) (ResourceSource, *Skill, error) {
	if err := contextError(ctx, operation); err != nil {
		return nil, nil, err
	}
	var errs []error
	for _, src := range m.sources {
		if err := contextError(ctx, operation); err != nil {
			return nil, nil, err
		}
		skill, err := src.Load(ctx, name)
		if ctxErr := contextError(ctx, operation); ctxErr != nil {
			return nil, nil, ctxErr
		}
		if err == nil {
			return src, skill, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, nil, err
		}
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		return nil, nil, fmt.Errorf("skills: skill %q: %w", name, fs.ErrNotExist)
	}
	return nil, nil, errors.Join(errs...)
}
