// Package skills owns the managed Skill library vocabulary: proposal identity,
// review provenance, lifecycle state, and content safety classification. Skill
// discovery, filesystem layout, rendering, and publication remain outside this
// package.
package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	skillspec "github.com/Tangerg/scope/skills"
)

var (
	ErrConflict          = errors.New("skills: destination already exists")
	ErrProposalChanged   = errors.New("skills: proposal content changed")
	ErrProposalQueueFull = errors.New("skills: proposal review queue is full")
	ErrDocumentTooLarge  = errors.New("skills: document is too large")
	ErrLibraryCapacity   = errors.New("skills: library capacity exceeded")
	ErrResourceTooLarge  = errors.New("skills: resource is too large")
	ErrUsageTooLarge     = errors.New("skills: usage metadata is too large")
	ErrNotFound          = errors.New("skills: entry not found")
)

const (
	// MaxAuthoredSkillDocumentBytes bounds the complete rendered SKILL.md
	// accepted by the governed authoring lifecycle. Bundled resources remain
	// separate and are disclosed on demand.
	MaxAuthoredSkillDocumentBytes = 1 << 20

	// MaxPendingProposalsPerScope bounds one complete, non-paginated review
	// queue. Project and user libraries each own an independent queue.
	MaxPendingProposalsPerScope = 128

	// MaxSkillsPerSource bounds one complete, non-paginated Skill source. It is
	// shared by project/user discovery and the active+archived managed library,
	// so model, UI, and curator consumers cannot disagree about admissible size.
	MaxSkillsPerSource = 256

	// MaxSkillDirectoryEntries bounds the raw top-level directory snapshot used
	// to discover MaxSkillsPerSource valid Skills. The small allowance covers
	// reserved lifecycle metadata while preventing junk entries from turning a
	// complete list into an unbounded scan.
	MaxSkillDirectoryEntries = MaxSkillsPerSource + 16

	// MaxSkillResourceBytes bounds one model-facing bundled resource read. A
	// resource is progressive-disclosure context, not an arbitrary binary
	// transport; larger assets must be consumed by a purpose-built tool.
	MaxSkillResourceBytes = 1 << 20
)

// Scope identifies the Skill library that owns a proposal or active Skill.
type Scope string

const (
	ScopeProject Scope = "project"
	ScopeUser    Scope = "user"
)

func (s Scope) Validate() error {
	switch s {
	case ScopeProject, ScopeUser:
		return nil
	default:
		return fmt.Errorf("skills: invalid scope %q", s)
	}
}

// ProposalOrigin identifies why the runtime created a proposal.
type ProposalOrigin string

const (
	ProposalOriginRequested ProposalOrigin = "requested"
	ProposalOriginMined     ProposalOrigin = "mined"
)

func (p ProposalOrigin) Validate() error {
	switch p {
	case ProposalOriginRequested, ProposalOriginMined:
		return nil
	default:
		return fmt.Errorf("skills: invalid proposal origin %q", p)
	}
}

// Lifecycle is the managed user library state of an approved Skill.
type Lifecycle string

const (
	Active   Lifecycle = "active"
	Archived Lifecycle = "archived"
)

// Entry describes one approved Skill in the managed user library.
type Entry struct {
	Name        string
	Description string
	Lifecycle   Lifecycle
}

// Proposal is immutable Skill content awaiting an explicit review decision.
// Scope identifies the target library; Origin and SourceSession are runtime
// provenance and must never be accepted from model-authored tool arguments.
type Proposal struct {
	Scope         Scope
	Name          string
	Description   string
	Instructions  string
	Origin        ProposalOrigin
	SourceSession string
	Revises       bool
}

// ProposalRef binds one review decision to the exact immutable proposal bytes
// in one Skill scope.
type ProposalRef struct {
	Scope    Scope
	Name     string
	Revision string
}

// NewProposalRef derives the stable reference for exact proposal content.
func NewProposalRef(scope Scope, name string, content []byte) ProposalRef {
	payload := make([]byte, 0, len(scope)+len(name)+len(content)+2)
	payload = append(payload, scope...)
	payload = append(payload, 0)
	payload = append(payload, name...)
	payload = append(payload, 0)
	payload = append(payload, content...)
	digest := sha256.Sum256(payload)
	return ProposalRef{Scope: scope, Name: name, Revision: hex.EncodeToString(digest[:])}
}

// Validate checks whether the reference can identify a stored proposal.
func (p ProposalRef) Validate() error {
	if err := p.Scope.Validate(); err != nil {
		return err
	}
	if err := skillspec.ValidateName(p.Name); err != nil {
		return fmt.Errorf("proposal reference name: %w", err)
	}
	if len(p.Revision) != sha256.Size*2 {
		return errors.New("proposal reference revision must be a SHA-256 digest")
	}
	if _, err := hex.DecodeString(p.Revision); err != nil {
		return fmt.Errorf("proposal reference revision: %w", err)
	}
	return nil
}

// Matches reports whether content produces this exact reference.
func (p ProposalRef) Matches(content []byte) bool {
	return p == NewProposalRef(p.Scope, p.Name, content)
}

// ProposalReview contains the complete immutable content and provenance a human
// needs to review before approving or rejecting a proposal.
type ProposalReview struct {
	Ref           ProposalRef
	Description   string
	Instructions  string
	Origin        ProposalOrigin
	SourceSession string
	Revises       bool
}

// Validate checks proposal scope, provenance, frontmatter, and instructions.
func (p Proposal) Validate() error {
	if err := p.Scope.Validate(); err != nil {
		return err
	}
	if p.Origin != "" {
		if err := p.Origin.Validate(); err != nil {
			return err
		}
	}
	if err := (skillspec.Frontmatter{Name: p.Name, Description: p.Description}).Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(p.Instructions) == "" {
		return errors.New("skill instructions are required")
	}
	if len(p.Instructions) > MaxAuthoredSkillDocumentBytes {
		return fmt.Errorf(
			"%w: instructions use %d bytes before frontmatter (maximum document %d)",
			ErrDocumentTooLarge,
			len(p.Instructions),
			MaxAuthoredSkillDocumentBytes,
		)
	}
	return nil
}

var dangerousSkillPattern = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\brm\s+-[a-z]*r[a-z]*f[a-z]*\s+(/|~|\$\{?HOME\}?)(\s|$)`),
	regexp.MustCompile(`(?i)\brm\s+-[a-z]*f[a-z]*r[a-z]*\s+(/|~|\$\{?HOME\}?)(\s|$)`),
	regexp.MustCompile(`(?i)--no-preserve-root`),
	regexp.MustCompile(`:\s*\(\s*\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;\s*:`),
	regexp.MustCompile(`(?i)\b(curl|wget)\b[^\n|]*\|\s*(sudo\s+)?(sh|bash|zsh)\b`),
	regexp.MustCompile(`(?i)\bmkfs(\.\w+)?\b`),
	regexp.MustCompile(`(?i)\bdd\b[^\n|]*\bof=/dev/`),
}

// ProposalSafetyIssue classifies built-in proposal safety checks.
type ProposalSafetyIssue string

const (
	ProposalSafe                 ProposalSafetyIssue = "safe"
	ProposalDangerousInstruction ProposalSafetyIssue = "dangerousInstruction"
)

// Valid reports whether p is a supported proposal safety classification.
func (p ProposalSafetyIssue) Valid() bool {
	return p == ProposalSafe || p == ProposalDangerousInstruction
}

// SafetyIssue reports whether proposal content contains a known destructive instruction.
func (p Proposal) SafetyIssue() ProposalSafetyIssue {
	content := p.Name + "\n" + p.Description + "\n" + p.Instructions
	for _, re := range dangerousSkillPattern {
		if re.MatchString(content) {
			return ProposalDangerousInstruction
		}
	}
	return ProposalSafe
}
