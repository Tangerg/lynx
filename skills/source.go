package skills

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
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
// remote store, or a test fake all satisfy it. Implementations must honor ctx
// cancellation and return an error matching context.Canceled or
// context.DeadlineExceeded.
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

var (
	errNilFS           = errors.New("skills: filesystem must not be nil")
	errNilSource       = errors.New("skills: source must not be nil")
	errNilResourceFile = errors.New("resource source returned a nil file without an error")
)

// fsSource implements [Source] over any fs.FS whose top-level entries are skill
// directories (each holding a SKILL.md). Reads are lazy and per-call, so
// edits on the backing filesystem are picked up without a refresh step.
type fsSource struct {
	fsys fs.FS
}

// NewFS returns a [ResourceSource] backed by fsys.
//
// NewFS trusts the confinement semantics of fsys. Use [Dir] for an operating
// system directory that must reject symbolic links escaping its root. A nil or
// typed-nil filesystem produces a source whose operations return an error.
func NewFS(fsys fs.FS) ResourceSource {
	if isNil(fsys) {
		fsys = errorFS{err: errNilFS}
	}
	return &fsSource{fsys: fsys}
}

func isNil(value any) bool {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return true
	}
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

// Dir returns a [ResourceSource] backed by the directory rooted at root.
// Skill metadata opens are confined to root; resource opens are additionally
// confined to the selected skill directory, including symbolic-link
// resolution.
func Dir(root string) ResourceSource {
	return &fsSource{fsys: rootedFS(root)}
}

// rootedFS confines each open to its root without retaining a directory
// descriptor between calls. os.OpenInRoot also prevents symbolic links from
// escaping root, which os.DirFS deliberately does not guarantee.
type rootedFS string

func (f rootedFS) Open(name string) (fs.File, error) {
	return os.OpenInRoot(string(f), name)
}

// confinedResourceFS is the stronger capability used by [Dir]. Generic fs.FS
// values are trusted to implement their own confinement; a rooted directory
// can additionally anchor resource symlink resolution at the selected skill,
// not merely at the repository root.
type confinedResourceFS interface {
	openInDir(dir, name string) (fs.File, error)
}

func (f rootedFS) openInDir(dir, name string) (fs.File, error) {
	root, err := os.OpenRoot(string(f))
	if err != nil {
		return nil, err
	}
	sub, err := root.OpenRoot(filepath.FromSlash(dir))
	if err != nil {
		return nil, errors.Join(err, root.Close())
	}
	file, err := sub.Open(filepath.FromSlash(name))
	closeErr := errors.Join(sub.Close(), root.Close())
	if err != nil {
		return nil, errors.Join(err, closeErr)
	}
	if closeErr != nil {
		return nil, errors.Join(closeErr, file.Close())
	}
	return file, nil
}

// errorFS turns constructor misuse into an ordinary operation error instead
// of a nil-interface panic.
type errorFS struct {
	err error
}

func (f errorFS) Open(string) (fs.File, error) {
	return nil, f.err
}

// List returns a summary for every valid skill directory, sorted by name.
// Entries that are not directories, lack a SKILL.md, have an invalid directory
// name, or contain an invalid skill are skipped rather than failing the whole
// listing. Repository access failures are returned; they are not mistaken for
// invalid entries. A missing root directory is not an error — it just means
// there are no skills yet (so a source pointed at a not-yet-created
// ~/.lyra/skills lists empty rather than failing).
func (f *fsSource) List(ctx context.Context) ([]Summary, error) {
	if err := contextError(ctx, "list"); err != nil {
		return nil, err
	}
	entries, err := fs.ReadDir(f.fsys, ".")
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
		if validateName(name) != nil {
			continue
		}
		sk, err := f.load(ctx, name)
		if err != nil {
			if ctxErr := contextError(ctx, "list"); ctxErr != nil {
				return nil, ctxErr
			}
			if errors.Is(err, fs.ErrNotExist) || errors.Is(err, ErrInvalidSkill) {
				continue
			}
			return nil, fmt.Errorf("skills: list: %w", err)
		}
		summaries = append(summaries, sk.Summary())
	}
	slices.SortFunc(summaries, func(a, b Summary) int {
		return strings.Compare(a.Name, b.Name)
	})
	return summaries, nil
}

// Load reads, parses, and validates one skill by directory name.
func (f *fsSource) Load(ctx context.Context, name string) (*Skill, error) {
	return f.load(ctx, name)
}

func (f *fsSource) load(ctx context.Context, name string) (*Skill, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	operation := fmt.Sprintf("load %q", name)
	if err := contextError(ctx, operation); err != nil {
		return nil, err
	}
	data, readErr := fs.ReadFile(f.fsys, name+"/"+SkillFile)
	if err := contextError(ctx, operation); err != nil {
		return nil, err
	}
	if readErr != nil {
		return nil, fmt.Errorf("skills: load %q: %w", name, readErr)
	}
	fm, body, parseErr := Parse(data)
	if err := contextError(ctx, operation); err != nil {
		return nil, err
	}
	if parseErr != nil {
		return nil, invalidSkill(name, parseErr)
	}
	validationErr := fm.Validate()
	if err := contextError(ctx, operation); err != nil {
		return nil, err
	}
	if validationErr != nil {
		return nil, invalidSkill(name, validationErr)
	}
	if fm.Name != name {
		return nil, invalidSkill(name, fmt.Errorf("%w: frontmatter %q vs directory %q", ErrNameMismatch, fm.Name, name))
	}
	if err := contextError(ctx, operation); err != nil {
		return nil, err
	}
	return &Skill{Frontmatter: fm, Body: body}, nil
}

func invalidSkill(name string, cause error) error {
	return fmt.Errorf("%w %q: %w", ErrInvalidSkill, name, cause)
}

// OpenResource opens a file bundled under a skill (e.g.
// references/REFERENCE.md, scripts/run.py). The resource path is resolved
// relative to the skill directory. Lexical traversal out of the directory is
// rejected with [ErrResourcePath]; sources returned by [Dir] also reject
// symbolic links that resolve outside the selected skill directory.
func (f *fsSource) OpenResource(ctx context.Context, name, resource string) (fs.File, error) {
	if err := validateName(name); err != nil {
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
	if confined, ok := f.fsys.(confinedResourceFS); ok {
		file, err = confined.openInDir(name, resource)
	} else {
		file, err = f.fsys.Open(name + "/" + resource)
	}
	if err != nil {
		err = fmt.Errorf("skills: open resource %q/%q: %w", name, resource, err)
	}
	return checkedResourceFile(ctx, operation, name, resource, file, err)
}

// ReadResource reads and closes a bundled skill resource from src.
func ReadResource(ctx context.Context, src ResourceSource, name, resource string) ([]byte, error) {
	if isNil(src) {
		return nil, errNilSource
	}
	if err := validateName(name); err != nil {
		return nil, err
	}
	if err := validateResourcePath(resource); err != nil {
		return nil, err
	}
	operation := fmt.Sprintf("read resource %q/%q", name, resource)
	if err := contextError(ctx, operation); err != nil {
		return nil, err
	}
	file, err := src.OpenResource(ctx, name, resource)
	file, err = checkedResourceFile(ctx, operation, name, resource, file, err)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(file)
	err = errors.Join(err, ctx.Err())
	closeErr := file.Close()
	err = errors.Join(
		resourceIOError("read", name, resource, err),
		resourceIOError("close", name, resource, closeErr),
	)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// checkedResourceFile normalizes the fs.File/error pair returned by a source.
// It gives cancellation precedence, closes any unusable file, and rejects the
// invalid nil-file/nil-error pair before callers can panic in io.ReadAll.
func checkedResourceFile(
	ctx context.Context,
	operation string,
	name string,
	resource string,
	file fs.File,
	err error,
) (fs.File, error) {
	if ctxErr := contextError(ctx, operation); ctxErr != nil {
		return nil, errors.Join(ctxErr, closeResourceFile(name, resource, file))
	}
	if err != nil {
		return nil, errors.Join(err, closeResourceFile(name, resource, file))
	}
	if isNil(file) {
		return nil, fmt.Errorf("skills: %s: %w", operation, errNilResourceFile)
	}
	return file, nil
}

func closeResourceFile(name, resource string, file fs.File) error {
	if isNil(file) {
		return nil
	}
	return resourceIOError("close", name, resource, file.Close())
}

func resourceIOError(operation, name, resource string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("skills: %s resource %q/%q: %w", operation, name, resource, err)
}

func validateResourcePath(resource string) error {
	if resource == "." || !fs.ValidPath(resource) || strings.ContainsRune(resource, '\\') {
		return fmt.Errorf("%w: %q", ErrResourcePath, resource)
	}
	return nil
}

func contextError(ctx context.Context, operation string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("skills: %s: %w", operation, err)
	}
	return nil
}
