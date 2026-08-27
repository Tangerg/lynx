package skills

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/samber/lo"
)

// SkillFile is the required metadata file at the root of every skill
// directory.
const SkillFile = "SKILL.md"

// Repository is a read-only Agent Skills repository backed by an [fs.FS].
// Reads are lazy and per-call, so changes to the backing filesystem are
// visible without a refresh operation.
type Repository struct {
	fsys fs.FS
}

var _ Source = (*Repository)(nil)
var _ ResourceSource = (*Repository)(nil)

func NewRepository(fsys fs.FS) (*Repository, error) {
	if lo.IsNil(fsys) {
		return nil, ErrNilFilesystem
	}
	return &Repository{fsys: fsys}, nil
}

func NewDirectoryRepository(root string) *Repository {
	return &Repository{fsys: rootedFS(root)}
}

func (r *Repository) validate() error {
	if r == nil || lo.IsNil(r.fsys) {
		return ErrNilFilesystem
	}
	return nil
}

// List returns a summary for every valid skill directory, sorted by name.
// Invalid skill entries are skipped. Repository access failures are returned.
// A missing root directory is treated as an empty repository.
func (r *Repository) List(ctx context.Context) ([]Summary, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}
	if err := contextError(ctx, "list"); err != nil {
		return nil, err
	}
	entries, err := fs.ReadDir(r.fsys, ".")
	if ctxErr := contextError(ctx, "list"); ctxErr != nil {
		return nil, ctxErr
	}
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("skills: list: %w", err)
	}

	summaries := make([]Summary, 0, len(entries))
	for _, entry := range entries {
		if err := contextError(ctx, "list"); err != nil {
			return nil, err
		}
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if ValidateName(name) != nil {
			continue
		}
		skill, err := r.load(ctx, name)
		if err != nil {
			if ctxErr := contextError(ctx, "list"); ctxErr != nil {
				return nil, ctxErr
			}
			if errors.Is(err, fs.ErrNotExist) || errors.Is(err, ErrInvalidSkill) {
				continue
			}
			return nil, fmt.Errorf("skills: list: %w", err)
		}
		summaries = append(summaries, skill.Summary())
	}
	slices.SortFunc(summaries, func(a, b Summary) int {
		return strings.Compare(a.Name, b.Name)
	})
	return summaries, nil
}

// Load reads, parses, and validates one skill by directory name.
func (r *Repository) Load(ctx context.Context, name string) (*Skill, error) {
	return r.load(ctx, name)
}

func (r *Repository) load(ctx context.Context, name string) (*Skill, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	operation := fmt.Sprintf("load %q", name)
	if err := contextError(ctx, operation); err != nil {
		return nil, err
	}
	data, readErr := fs.ReadFile(r.fsys, name+"/"+SkillFile)
	if err := contextError(ctx, operation); err != nil {
		return nil, err
	}
	if readErr != nil {
		return nil, fmt.Errorf("skills: load %q: %w", name, readErr)
	}

	skill, err := Parse(data)
	if ctxErr := contextError(ctx, operation); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		return nil, invalidSkill(name, err)
	}
	if skill.Name != name {
		return nil, invalidSkill(name, fmt.Errorf(
			"%w: frontmatter %q vs directory %q",
			ErrNameMismatch,
			skill.Name,
			name,
		))
	}
	return skill, nil
}

// OpenResource opens a file bundled under a skill. The resource path is
// resolved relative to the skill directory. Lexical traversal is rejected;
// repositories returned by [NewDirectoryRepository] also reject symlink escapes.
func (r *Repository) OpenResource(ctx context.Context, name, resource string) (fs.File, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	if err := validateResourcePath(resource); err != nil {
		return nil, err
	}
	operation := fmt.Sprintf("open resource %q/%q", name, resource)
	if err := contextError(ctx, operation); err != nil {
		return nil, err
	}

	var file fs.File
	var err error
	if confined, ok := r.fsys.(confinedResourceFS); ok {
		file, err = confined.openInDir(name, resource)
	} else {
		file, err = r.fsys.Open(name + "/" + resource)
	}
	if err != nil {
		err = fmt.Errorf("skills: open resource %q/%q: %w", name, resource, err)
	}
	return checkedResourceFile(ctx, operation, name, resource, file, err)
}

func invalidSkill(name string, cause error) error {
	return fmt.Errorf("%w %q: %w", ErrInvalidSkill, name, cause)
}
