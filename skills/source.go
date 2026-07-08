package skills

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
)

// SkillFile is the required metadata file at the root of every skill
// directory.
const SkillFile = "SKILL.md"

// Source is the read-only repository that lists and loads skills. Its two
// operations mirror the first progressive-disclosure levels, so a consumer
// pulls in only as much as a task needs:
//
//   - List         — name + description for every skill (level 1)
//   - Load         — one skill's full instructions (level 2)
//
// The interface lives here (consumer side) so callers depend on the
// capability, not on a filesystem layout: a real directory, an embedded FS, a
// remote store, or a test fake all satisfy it.
type Source interface {
	List(ctx context.Context) ([]Summary, error)
	Load(ctx context.Context, name string) (*Skill, error)
}

// ResourceSource is a [Source] that can also open bundled files under a skill
// directory. Consumers that only need progressive-disclosure levels 1 and 2
// depend on [Source]; consumers that need references/assets/scripts ask for
// this narrower extension.
type ResourceSource interface {
	Source
	OpenResource(ctx context.Context, name, resource string) (fs.File, error)
}

var _ Source = (*fsSource)(nil)
var _ ResourceSource = (*fsSource)(nil)

var errNilSource = errors.New("skills: source must not be nil")

// fsSource implements [Source] over any fs.FS whose top-level entries are skill
// directories (each holding a SKILL.md). Reads are lazy and per-call, so
// edits on the backing filesystem are picked up without a refresh step.
type fsSource struct {
	fsys fs.FS
}

// NewFS returns a [ResourceSource] backed by fsys.
func NewFS(fsys fs.FS) ResourceSource {
	return &fsSource{fsys: fsys}
}

// Dir returns a [ResourceSource] backed by the directory rooted at root.
func Dir(root string) ResourceSource {
	return &fsSource{fsys: os.DirFS(root)}
}

// List returns a summary for every valid skill directory, sorted by name.
// Entries that are not directories, lack a SKILL.md, or fail validation are
// skipped rather than failing the whole listing. A missing root directory is
// not an error — it just means there are no skills yet (so a source pointed at
// a not-yet-created ~/.lyra/skills lists empty rather than failing).
func (f *fsSource) List(_ context.Context) ([]Summary, error) {
	entries, err := fs.ReadDir(f.fsys, ".")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
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
func (f *fsSource) Load(_ context.Context, name string) (*Skill, error) {
	return f.load(name)
}

func (f *fsSource) load(name string) (*Skill, error) {
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

// OpenResource opens a file bundled under a skill (e.g.
// references/REFERENCE.md, scripts/run.py). The resource path is resolved
// relative to the skill directory and must stay within it; traversal out of
// the directory is rejected with [ErrResourcePath].
func (f *fsSource) OpenResource(_ context.Context, name, resource string) (fs.File, error) {
	if err := validName(name); err != nil {
		return nil, err
	}
	full := path.Join(name, resource)
	if full == name || !strings.HasPrefix(full, name+"/") || !fs.ValidPath(full) {
		return nil, fmt.Errorf("%w: %q", ErrResourcePath, resource)
	}
	file, err := f.fsys.Open(full)
	if err != nil {
		return nil, fmt.Errorf("skills: open resource %q/%q: %w", name, resource, err)
	}
	return file, nil
}

// ReadResource reads and closes a bundled skill resource from src.
func ReadResource(ctx context.Context, src ResourceSource, name, resource string) ([]byte, error) {
	if src == nil {
		return nil, errNilSource
	}
	file, err := src.OpenResource(ctx, name, resource)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(file)
	closeErr := file.Close()
	if err != nil {
		return nil, fmt.Errorf("skills: read resource %q/%q: %w", name, resource, err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("skills: close resource %q/%q: %w", name, resource, closeErr)
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
