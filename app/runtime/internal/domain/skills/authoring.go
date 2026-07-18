package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	skillspec "github.com/Tangerg/lynx/skills"
)

// DraftsSubdir is the reserved directory, under a skills root, where proposed
// skills wait for promotion. Its underscore-prefixed name is deliberately not a
// valid skill name ([skillspec]'s name rule forbids '_'), so the read-only skill
// source skips it during discovery — a draft is invisible to the agent until a
// human promotes it out.
const DraftsSubdir = "_drafts"

// ArchivedSubdir is the reserved directory holding archived skills. Archiving is
// "remove from active use without deleting" — the skill moves here (skipped by
// discovery, same as _drafts) and can be restored. Never a valid skill name.
const ArchivedSubdir = "_archive"

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

// Render produces the on-disk SKILL.md content: a YAML frontmatter block
// (name + description) followed by the body. yaml.Marshal quotes/escapes the
// fields so a description with special characters can't break the block.
func (d Draft) Render() (string, error) {
	front, err := yaml.Marshal(map[string]string{"name": d.Name, "description": d.Description})
	if err != nil {
		return "", fmt.Errorf("skills: render frontmatter: %w", err)
	}
	return "---\n" + string(front) + "---\n\n" + strings.TrimSpace(d.Body) + "\n", nil
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

// Scan is a CONSERVATIVE static check over a proposed skill's rendered content.
// It is explicitly NOT a security boundary — a skill is prose the agent reads,
// not code that runs, and the check is trivially evadable — it only refuses a
// draft that spells out an obviously-catastrophic instruction (rm -rf of a
// root/home path, --no-preserve-root, a fork bomb, curl|sh, a device wipe)
// before it reaches the human promotion gate. Returns a reason + true when the
// draft should be refused outright.
func (d Draft) Scan() (reason string, dangerous bool) {
	content := d.Name + "\n" + d.Description + "\n" + d.Body
	for _, re := range dangerousSkillPattern {
		if re.MatchString(content) {
			return "the proposed skill contains an obviously-dangerous instruction (e.g. rm -rf of a root/home path, a fork bomb, or piping a remote script into a shell)", true
		}
	}
	return "", false
}
