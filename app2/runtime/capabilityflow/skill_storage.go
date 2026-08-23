package capabilityflow

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	lyraskills "github.com/Tangerg/lynx/skills"
	"gopkg.in/yaml.v3"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/workspacefs"
)

type skillLibrary struct {
	anchor   string
	path     string
	dirMode  fs.FileMode
	fileMode fs.FileMode
}

func prepareUserSkillLibrary(home string) error {
	root, err := os.OpenRoot(home)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.MkdirAll(".lyra/skills", 0o700)
}

func (service *Service) skillLibrary(
	resolved workspacefs.Resolution,
	scope protocol.SkillScope,
) (skillLibrary, error) {
	anchor := resolved.Workspace.Path()
	dirMode := fs.FileMode(0o755)
	fileMode := fs.FileMode(0o644)
	switch scope {
	case protocol.SkillScopeProject:
	case protocol.SkillScopeUser:
		anchor = service.home
		dirMode = 0o700
		fileMode = 0o600
	default:
		return skillLibrary{}, fmt.Errorf("%w: invalid skill scope", protocol.ErrInvalidParams)
	}
	path, err := confinedSkillLibrary(anchor)
	if err != nil {
		return skillLibrary{}, err
	}
	return skillLibrary{anchor: anchor, path: path, dirMode: dirMode, fileMode: fileMode}, nil
}

func confinedSkillLibrary(anchor string) (string, error) {
	physicalAnchor, err := filepath.EvalSymlinks(anchor)
	if err != nil {
		return "", err
	}
	physicalAnchor, err = filepath.Abs(physicalAnchor)
	if err != nil {
		return "", err
	}
	target := filepath.Join(anchor, ".lyra", "skills")
	existing := target
	for {
		_, err := os.Lstat(existing)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		next := filepath.Dir(existing)
		if next == existing {
			return "", errors.New("capabilityflow: skill library has no existing anchor")
		}
		existing = next
	}
	physicalExisting, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(physicalAnchor, physicalExisting)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", protocol.ErrPathOutsideRoot
	}
	return target, nil
}

func validateSkillProposal(proposal protocol.SkillProposal) ([]byte, error) {
	if proposal.Scope != protocol.SkillScopeProject && proposal.Scope != protocol.SkillScopeUser {
		return nil, errors.New("scope must be project or user")
	}
	if proposal.Origin != protocol.SkillProposalOriginRequested && proposal.Origin != protocol.SkillProposalOriginMined {
		return nil, errors.New("origin must be requested or mined")
	}
	if strings.TrimSpace(proposal.SourceSession) == "" {
		return nil, errors.New("source session is required")
	}
	frontmatter := lyraskills.Frontmatter{
		Name:        strings.TrimSpace(proposal.Name),
		Description: strings.TrimSpace(proposal.Description),
	}
	if err := frontmatter.Validate(); err != nil {
		return nil, err
	}
	instructions := strings.TrimSpace(proposal.Instructions)
	if instructions == "" {
		return nil, errors.New("instructions are required")
	}
	if dangerousSkillContent(proposal.Name + "\n" + proposal.Description + "\n" + instructions) {
		return nil, errors.New("instructions contain a known destructive pattern")
	}
	metadata, err := yaml.Marshal(frontmatter)
	if err != nil {
		return nil, err
	}
	document := []byte("---\n" + string(metadata) + "---\n\n" + instructions + "\n")
	if len(document) > maxAuthoredDocumentBytes {
		return nil, fmt.Errorf("skill document exceeds %d bytes", maxAuthoredDocumentBytes)
	}
	return document, nil
}

func validateSkillProposalRef(ref protocol.SkillProposalRef) error {
	if ref.Scope != protocol.SkillScopeProject && ref.Scope != protocol.SkillScopeUser {
		return errors.New("scope must be project or user")
	}
	if err := (lyraskills.Frontmatter{
		Name: ref.Name, Description: "proposal reference",
	}).Validate(); err != nil {
		return err
	}
	if len(ref.Revision) != sha256.Size*2 {
		return errors.New("revision must be a SHA-256 digest")
	}
	if _, err := hex.DecodeString(ref.Revision); err != nil {
		return fmt.Errorf("revision: %w", err)
	}
	return nil
}

func skillProposalRevision(scope protocol.SkillScope, name string, document []byte) string {
	hash := sha256.New()
	_, _ = io.WriteString(hash, string(scope))
	_, _ = hash.Write([]byte{0})
	_, _ = io.WriteString(hash, name)
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(document)
	return hex.EncodeToString(hash.Sum(nil))
}

func loadSkill(
	ctx context.Context,
	root string,
	name string,
) (*lyraskills.Skill, bool, error) {
	skill, err := lyraskills.Dir(root).Load(ctx, name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("capabilityflow: load target skill: %w", err)
	}
	return skill, true, nil
}

func skillMatchesProposal(skill *lyraskills.Skill, proposal protocol.SkillProposal) bool {
	return skill != nil &&
		skill.Name == proposal.Name &&
		skill.Description == strings.TrimSpace(proposal.Description) &&
		skill.Body == strings.TrimSpace(proposal.Instructions)
}

func publishSkill(library skillLibrary, name string, document []byte) error {
	anchor, err := os.OpenRoot(library.anchor)
	if err != nil {
		return err
	}
	defer anchor.Close()
	const relativeLibrary = ".lyra/skills"
	if err := anchor.MkdirAll(relativeLibrary, library.dirMode); err != nil {
		return err
	}
	root, err := anchor.OpenRoot(relativeLibrary)
	if err != nil {
		return err
	}
	defer root.Close()
	if info, err := root.Lstat(name); errors.Is(err, os.ErrNotExist) {
		if err := root.Mkdir(name, library.dirMode); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("skill destination is not a directory")
	}
	skillRoot, err := root.OpenRoot(name)
	if err != nil {
		return err
	}
	defer skillRoot.Close()
	temporary, err := openTemporarySkillFile(skillRoot, library.fileMode)
	if err != nil {
		return err
	}
	temporaryName := filepath.Base(temporary.Name())
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = skillRoot.Remove(temporaryName)
		}
	}()
	if _, err := temporary.Write(document); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := skillRoot.Rename(temporaryName, lyraskills.SkillFile); err != nil {
		return err
	}
	committed = true
	return nil
}

func openTemporarySkillFile(root *os.Root, mode fs.FileMode) (*os.File, error) {
	for range 8 {
		random := make([]byte, 8)
		if _, err := rand.Read(random); err != nil {
			return nil, err
		}
		name := ".lyra-skill-" + hex.EncodeToString(random) + ".tmp"
		file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return file, err
	}
	return nil, errors.New("capabilityflow: could not reserve a temporary skill file")
}

type curatedSkillSource struct {
	source   lyraskills.ResourceSource
	archived map[string]struct{}
}

func (source *curatedSkillSource) List(ctx context.Context) ([]lyraskills.Summary, error) {
	values, err := source.source.List(ctx)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(values, func(value lyraskills.Summary) bool {
		_, archived := source.archived[value.Name]
		return archived
	}), nil
}

func (source *curatedSkillSource) Load(ctx context.Context, name string) (*lyraskills.Skill, error) {
	if _, archived := source.archived[name]; archived {
		return nil, fmt.Errorf("skills: %q is archived: %w", name, fs.ErrNotExist)
	}
	return source.source.Load(ctx, name)
}

func (source *curatedSkillSource) OpenResource(
	ctx context.Context,
	name string,
	resource string,
) (fs.File, error) {
	if _, archived := source.archived[name]; archived {
		return nil, fmt.Errorf("skills: %q is archived: %w", name, fs.ErrNotExist)
	}
	return source.source.OpenResource(ctx, name, resource)
}

var destructiveSkillPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\brm\s+-[a-z]*r[a-z]*f[a-z]*\s+(/|~|\$\{?HOME\}?)(\s|$)`),
	regexp.MustCompile(`(?i)\brm\s+-[a-z]*f[a-z]*r[a-z]*\s+(/|~|\$\{?HOME\}?)(\s|$)`),
	regexp.MustCompile(`(?i)--no-preserve-root`),
	regexp.MustCompile(`:\s*\(\s*\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;\s*:`),
	regexp.MustCompile(`(?i)\b(curl|wget)\b[^\n|]*\|\s*(sudo\s+)?(sh|bash|zsh)\b`),
	regexp.MustCompile(`(?i)\bmkfs(\.\w+)?\b`),
	regexp.MustCompile(`(?i)\bdd\b[^\n|]*\bof=/dev/`),
}

func dangerousSkillContent(content string) bool {
	for _, pattern := range destructiveSkillPatterns {
		if pattern.MatchString(content) {
			return true
		}
	}
	return false
}
