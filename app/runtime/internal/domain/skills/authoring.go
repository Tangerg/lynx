package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	skillspec "github.com/Tangerg/lynx/skills"
)

var (
	// ErrConflict reports that the destination lifecycle state already contains
	// a different skill. Promotion and archival never destroy either side to
	// force a move; the caller must resolve the conflict explicitly. It is a
	// domain-lifecycle outcome (not a storage-tech error), so it lives here where
	// every layer — including delivery — can classify it.
	ErrConflict = errors.New("skills: destination already exists")
	// ErrDraftChanged reports that staged bytes no longer match the handle a
	// promotion or discard was issued against. Neither the draft nor the active
	// library is touched.
	ErrDraftChanged = errors.New("skills: staged draft content changed")
	// ErrNotFound reports a requested draft or lifecycle entry that is absent.
	// It is a product outcome, not a filesystem sentinel, so Application and
	// Delivery can classify it without importing io/fs.
	ErrNotFound = errors.New("skills: entry not found")
)

// CreatedByAgent marks a skill whose content an agent authored through the
// draft flow (background trajectory mining or propose_skill), as distinct from
// a hand-written one. Only agent-authored skills are subject to automatic idle
// curation; a human-authored skill is never auto-archived.
const CreatedByAgent = "agent"

// Lifecycle is a skill's curator state in the management surface.
type Lifecycle string

const (
	// Active skills are discovered + loadable by the agent.
	Active Lifecycle = "active"
	// Archived skills are preserved but not loaded; a human can restore them.
	Archived Lifecycle = "archived"
)

// Entry is one skill in the management view: its identity + curator state.
type Entry struct {
	Name        string
	Description string
	Lifecycle   Lifecycle
}

// Draft is a skill an agent proposes through propose_skill: the required
// frontmatter fields plus the SKILL.md body. It is never visible to the model
// until a human approves its promotion into the active skill set.
type Draft struct {
	Name        string
	Description string
	Body        string

	// CreatedBy records who authored the draft's content (e.g. [CreatedByAgent]);
	// empty for a human-hand-authored skill. SourceSession is the session it was
	// mined from, or empty. The skill-authoring adapter persists both as
	// frontmatter metadata so the curator can gate automatic archival on
	// provenance.
	CreatedBy     string
	SourceSession string

	// Revises marks this draft as a new version of the already-active skill of
	// the same name — a feedback-driven refinement rather than a new skill.
	// Promotion of a revising draft replaces the active skill (archiving the old
	// version) instead of conflicting. Its file-store representation is an
	// infrastructure concern.
	Revises bool
}

// DraftHandle identifies the immutable bytes staged for one proposal. Name is
// the eventual active skill identity; Revision binds that name to the rendered
// SKILL.md. Approval and publication carry this value so a same-name proposal
// cannot substitute different bytes while a human decision is pending.
type DraftHandle struct {
	Name     string
	Revision string
}

// NewDraftHandle returns the content-addressed identity for rendered SKILL.md
// bytes. Equal proposals intentionally receive the same handle, making a
// suspended tool replay idempotent.
func NewDraftHandle(name string, content []byte) DraftHandle {
	payload := make([]byte, 0, len(name)+1+len(content))
	payload = append(payload, name...)
	payload = append(payload, 0)
	payload = append(payload, content...)
	digest := sha256.Sum256(payload)
	return DraftHandle{Name: name, Revision: hex.EncodeToString(digest[:])}
}

// Validate rejects malformed or path-like handles before a store uses them.
func (h DraftHandle) Validate() error {
	if err := (skillspec.Frontmatter{Name: h.Name, Description: "draft handle"}).Validate(); err != nil {
		return fmt.Errorf("draft handle name: %w", err)
	}
	if len(h.Revision) != sha256.Size*2 {
		return errors.New("draft handle revision must be a SHA-256 digest")
	}
	if _, err := hex.DecodeString(h.Revision); err != nil {
		return fmt.Errorf("draft handle revision: %w", err)
	}
	return nil
}

// Matches reports whether content is the exact rendered proposal represented
// by h.
func (h DraftHandle) Matches(content []byte) bool {
	return h == NewDraftHandle(h.Name, content)
}

// DraftInfo describes one staged proposal for the offline review surface: its
// content-addressed handle plus the human-facing description and provenance
// read back from the rendered SKILL.md frontmatter. It is what a reviewer sees
// before deciding to promote or reject a draft the agent mined.
type DraftInfo struct {
	Handle        DraftHandle
	Description   string
	CreatedBy     string
	SourceSession string
	Revises       bool
}

// Validate checks a proposed skill against the SKILL.md spec — the same
// name/description rules the read-only loader enforces ([skillspec.Frontmatter]),
// plus a non-empty body — so a draft can never promote into a skill the loader
// would then reject.
func (d Draft) Validate() error {
	if err := (skillspec.Frontmatter{Name: d.Name, Description: d.Description}).Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(d.Body) == "" {
		return errors.New("skill body is required")
	}
	return nil
}

// dangerousSkillPattern flags a handful of instructions a proposed skill should
// essentially never contain. Conservative and near-zero false positive.
var dangerousSkillPattern = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\brm\s+-[a-z]*r[a-z]*f[a-z]*\s+(/|~|\$\{?HOME\}?)(\s|$)`),
	regexp.MustCompile(`(?i)\brm\s+-[a-z]*f[a-z]*r[a-z]*\s+(/|~|\$\{?HOME\}?)(\s|$)`),
	regexp.MustCompile(`(?i)--no-preserve-root`),
	regexp.MustCompile(`:\s*\(\s*\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;\s*:`),           // fork bomb
	regexp.MustCompile(`(?i)\b(curl|wget)\b[^\n|]*\|\s*(sudo\s+)?(sh|bash|zsh)\b`), // pipe remote script into a shell
	regexp.MustCompile(`(?i)\bmkfs(\.\w+)?\b`),
	regexp.MustCompile(`(?i)\bdd\b[^\n|]*\bof=/dev/`),
}

// DraftSafetyIssue is the domain classification produced by a conservative
// static scan. Presentation of an issue belongs to the adapter that rejects a
// draft for its own consumer.
type DraftSafetyIssue uint8

const (
	DraftSafe DraftSafetyIssue = iota
	DraftDangerousInstruction
)

// SafetyIssue is a CONSERVATIVE static check over a proposed skill's content.
// It is explicitly NOT a security boundary — a skill is prose the agent reads,
// not code that runs, and the check is trivially evadable — it only refuses a
// draft that spells out an obviously-catastrophic instruction (rm -rf of a
// root/home path, --no-preserve-root, a fork bomb, curl|sh, a device wipe)
// before it reaches the human promotion gate.
func (d Draft) SafetyIssue() DraftSafetyIssue {
	content := d.Name + "\n" + d.Description + "\n" + d.Body
	for _, re := range dangerousSkillPattern {
		if re.MatchString(content) {
			return DraftDangerousInstruction
		}
	}
	return DraftSafe
}
