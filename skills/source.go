package skills

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
)

// SkillFile is the required metadata file at the root of every skill
// directory.
const SkillFile = "SKILL.md"

// Source is the read-only repository the skill tool reads from. Its three
// operations mirror the spec's progressive-disclosure levels, so a consumer
// pulls in only as much as a task needs:
//
//   - List         — name + description for every skill (level 1)
//   - Load         — one skill's full instructions (level 2)
//   - LoadResource — a bundled file under the skill, on demand (level 3)
//
// The interface lives here (consumer side) so callers depend on the
// capability, not on a filesystem layout: a real directory, an embedded FS, a
// remote store, or a test fake all satisfy it.
type Source interface {
	List(ctx context.Context) ([]Summary, error)
	Load(ctx context.Context, name string) (*Skill, error)
	LoadResource(ctx context.Context, name, resource string) ([]byte, error)
}

var _ Source = (*FS)(nil)

// FS implements [Source] over any fs.FS whose top-level entries are skill
// directories (each holding a SKILL.md). Reads are lazy and per-call, so
// edits on the backing filesystem are picked up without a refresh step.
type FS struct {
	fsys fs.FS
}

// NewFS adapts an fs.FS into a [Source].
func NewFS(fsys fs.FS) *FS {
	return &FS{fsys: fsys}
}

// Dir is the convenience constructor over a real directory path.
func Dir(root string) *FS {
	return &FS{fsys: os.DirFS(root)}
}

// List returns a summary for every valid skill directory, sorted by name.
// Entries that are not directories, lack a SKILL.md, or fail validation are
// skipped rather than failing the whole listing.
func (f *FS) List(_ context.Context) ([]Summary, error) {
	entries, err := fs.ReadDir(f.fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("skills: list: %w", err)
	}
	summaries := make([]Summary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sk, err := f.load(entry.Name())
		if err != nil {
			continue // not a valid skill directory — skip it
		}
		summaries = append(summaries, sk.Summary())
	}
	slices.SortFunc(summaries, func(a, b Summary) int {
		return strings.Compare(a.Name, b.Name)
	})
	return summaries, nil
}

// Load reads, parses, and validates one skill by directory name.
func (f *FS) Load(_ context.Context, name string) (*Skill, error) {
	return f.load(name)
}

func (f *FS) load(name string) (*Skill, error) {
	if err := validName(name); err != nil {
		return nil, err
	}
	data, err := fs.ReadFile(f.fsys, path.Join(name, SkillFile))
	if err != nil {
		return nil, fmt.Errorf("skills: load %q: %w", name, err)
	}
	fm, body, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("skills: load %q: %w", name, err)
	}
	if err := fm.Validate(); err != nil {
		return nil, fmt.Errorf("skills: load %q: %w", name, err)
	}
	if fm.Name != name {
		return nil, fmt.Errorf("%w: frontmatter %q vs directory %q", ErrNameMismatch, fm.Name, name)
	}
	return &Skill{Frontmatter: fm, Body: body}, nil
}

// LoadResource returns the contents of a file bundled under a skill (e.g.
// references/REFERENCE.md, scripts/run.py). The resource path is resolved
// relative to the skill directory and must stay within it; traversal out of
// the directory is rejected with [ErrResourcePath].
func (f *FS) LoadResource(_ context.Context, name, resource string) ([]byte, error) {
	if err := validName(name); err != nil {
		return nil, err
	}
	full := path.Join(name, resource)
	if full == name || !strings.HasPrefix(full, name+"/") || !fs.ValidPath(full) {
		return nil, fmt.Errorf("%w: %q", ErrResourcePath, resource)
	}
	data, err := fs.ReadFile(f.fsys, full)
	if err != nil {
		return nil, fmt.Errorf("skills: load resource %q/%q: %w", name, resource, err)
	}
	return data, nil
}

// validName guards that a skill name is a single path element, so it cannot
// escape the repository root via slashes or "..".
func validName(name string) error {
	if name == "" {
		return ErrNameEmpty
	}
	if name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("%w: %q", ErrNameInvalid, name)
	}
	return nil
}
