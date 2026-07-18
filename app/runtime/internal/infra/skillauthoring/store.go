// Package skillauthoring is the write side of Agent Skills: it stages a
// proposed skill as a draft under the skills root's reserved _drafts directory
// and, on human approval, promotes it into the active set. The read side (the
// skills module + the promptsource adapter) stays strictly read-only; this is
// the only writer, and it lives above that module (never inside it).
package skillauthoring

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	skillspec "github.com/Tangerg/lynx/skills"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

// Store stages and promotes skill drafts under a single skills root — the global
// skills directory (<LYRA_HOME>/skills). A draft lives at
// <root>/_drafts/<name>/SKILL.md; promotion moves <root>/_drafts/<name> to
// <root>/<name>, where the read-only source discovers it.
type Store struct {
	root string
}

// NewStore roots the authoring store at the global skills directory. An empty
// root disables authoring (SaveDraft errors), so a runtime without a skills
// home simply omits the propose_skill tool.
func NewStore(root string) *Store { return &Store{root: root} }

// Enabled reports whether a skills root is configured.
func (s *Store) Enabled() bool { return s != nil && s.root != "" }

// SaveDraft validates the proposal and writes it to the draft area, replacing
// any existing draft of the same name. It never touches the active set.
func (s *Store) SaveDraft(_ context.Context, draft skills.Draft) error {
	if !s.Enabled() {
		return fmt.Errorf("skillauthoring: no skills root configured")
	}
	if err := draft.Validate(); err != nil {
		return err
	}
	content, err := draft.Render()
	if err != nil {
		return err
	}
	dir := s.draftDir(draft.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("skillauthoring: create draft dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, skillFile), []byte(content), 0o644); err != nil {
		return fmt.Errorf("skillauthoring: write draft: %w", err)
	}
	return nil
}

// Promote moves a validated draft into the active skill set, replacing an
// existing skill of the same name (the human approved this version). It errors
// if no such draft exists.
func (s *Store) Promote(_ context.Context, name string) error {
	if !s.Enabled() {
		return fmt.Errorf("skillauthoring: no skills root configured")
	}
	if !validName(name) {
		return fmt.Errorf("skillauthoring: invalid skill name %q", name)
	}
	draft := s.draftDir(name)
	if _, err := os.Stat(filepath.Join(draft, skillFile)); err != nil {
		return fmt.Errorf("skillauthoring: no draft %q to promote: %w", name, err)
	}
	active := filepath.Join(s.root, name)
	// Rename can't overwrite a populated directory; drop any prior version first.
	// Not transactional (a crash between the two steps loses the old skill), which
	// is acceptable for a best-effort, human-gated authoring flow.
	if err := os.RemoveAll(active); err != nil {
		return fmt.Errorf("skillauthoring: clear existing skill %q: %w", name, err)
	}
	if err := os.Rename(draft, active); err != nil {
		return fmt.Errorf("skillauthoring: promote draft %q: %w", name, err)
	}
	return nil
}

// Archive moves an active skill into the archive (preserved, not loaded). It
// errors if no such active skill exists; a prior archived version of the same
// name is replaced. The move is not transactional (see [Store.Promote]).
func (s *Store) Archive(_ context.Context, name string) error {
	return s.move(name, s.activeDir(name), s.archiveDir(name), "archive")
}

// Restore moves an archived skill back into the active set. It errors if no such
// archived skill exists; a prior active version of the same name is replaced.
func (s *Store) Restore(_ context.Context, name string) error {
	return s.move(name, s.archiveDir(name), s.activeDir(name), "restore")
}

// move relocates a skill directory from src to dest, replacing any existing dest.
func (s *Store) move(name, src, dest, op string) error {
	if !s.Enabled() {
		return fmt.Errorf("skillauthoring: no skills root configured")
	}
	if !validName(name) {
		return fmt.Errorf("skillauthoring: invalid skill name %q", name)
	}
	if _, err := os.Stat(filepath.Join(src, skillFile)); err != nil {
		return fmt.Errorf("skillauthoring: cannot %s %q: %w", op, name, err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("skillauthoring: %s %q: %w", op, name, err)
	}
	if err := os.RemoveAll(dest); err != nil {
		return fmt.Errorf("skillauthoring: %s %q: clear destination: %w", op, name, err)
	}
	if err := os.Rename(src, dest); err != nil {
		return fmt.Errorf("skillauthoring: %s %q: %w", op, name, err)
	}
	return nil
}

// List returns the management view: every active skill followed by every
// archived one, each with its description (parsed via the read-only spec loader,
// so the two views can't disagree on what a skill says).
func (s *Store) List(ctx context.Context) ([]skills.Entry, error) {
	if !s.Enabled() {
		return nil, nil
	}
	active, err := entries(ctx, s.root, skills.Active)
	if err != nil {
		return nil, err
	}
	archived, err := entries(ctx, filepath.Join(s.root, skills.ArchivedSubdir), skills.Archived)
	if err != nil {
		return nil, err
	}
	return append(active, archived...), nil
}

// entries lists the skills directly under dir (the spec loader skips reserved
// underscore dirs and invalid entries), tagging each with lifecycle.
func entries(ctx context.Context, dir string, lifecycle skills.Lifecycle) ([]skills.Entry, error) {
	summaries, err := skillspec.Dir(dir).List(ctx)
	if err != nil {
		return nil, fmt.Errorf("skillauthoring: list %s skills: %w", lifecycle, err)
	}
	out := make([]skills.Entry, len(summaries))
	for i, sm := range summaries {
		out[i] = skills.Entry{Name: sm.Name, Description: sm.Description, Lifecycle: lifecycle}
	}
	return out, nil
}

func (s *Store) activeDir(name string) string { return filepath.Join(s.root, name) }
func (s *Store) archiveDir(name string) string {
	return filepath.Join(s.root, skills.ArchivedSubdir, name)
}

// DiscardDraft removes a rejected/abandoned draft. Missing is not an error.
func (s *Store) DiscardDraft(_ context.Context, name string) error {
	if !s.Enabled() || !validName(name) {
		return nil
	}
	if err := os.RemoveAll(s.draftDir(name)); err != nil {
		return fmt.Errorf("skillauthoring: discard draft %q: %w", name, err)
	}
	return nil
}

const skillFile = "SKILL.md"

func (s *Store) draftDir(name string) string {
	return filepath.Join(s.root, skills.DraftsSubdir, name)
}

// validName is a defensive guard against path traversal: callers pass a
// spec-validated name, but the store still refuses anything with a separator or
// dot segment before it touches the filesystem.
func validName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	return name == filepath.Base(name) && !filepath.IsAbs(name)
}
