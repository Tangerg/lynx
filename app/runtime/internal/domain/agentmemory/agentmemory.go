// Package agentmemory defines Lyra's agent-maintained long-term memory: the
// durable facts the agent mines from conversations, folded into addressable
// memory items. It is distinct from the human-authored LYRA.md cascade
// (package knowledge) — that stays a user-owned file the agent never writes;
// this is agent-owned, curated from an append-only fact ledger into discrete,
// individually addressable items.
//
// Which items get injected into an agent prompt, and in what order, is a
// model-adapter policy. This domain owns the durable memory values, lifecycle,
// and content invariants only.
package agentmemory

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Digest is a memory item's content identity: the same fact always hashes the
// same, so the fold deduplicates across statuses and a reconcile keeps an
// unchanged item's id and provenance. It is a domain concept — the persistence
// layer stores it, but the meaning (content identity) lives here.
func Digest(content string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(content)))
	return hex.EncodeToString(sum[:])
}

// ErrNotFound is returned by the management operations when no item has the
// given id.
var ErrNotFound = errors.New("agentmemory: item not found")

// ErrNotPending reports that a review targeted an item whose review boundary
// has already been resolved or that was user-authored as active.
var ErrNotPending = errors.New("agentmemory: item is not pending review")

// Scope selects the breadth of a memory item.
type Scope string

const (
	// ScopeProject — knowledge tied to one project directory: conventions,
	// build/test commands, decisions, gotchas. Keyed by the project path.
	ScopeProject Scope = "project"
	// ScopeUser — cross-project knowledge about how the user works. Project is
	// empty. The mining path populates it from a later batch; the model carries
	// the scope from the start so storage and injection need no reshape then.
	ScopeUser Scope = "user"
)

// Valid reports whether s names one of the two memory scopes.
func (s Scope) Valid() bool {
	return s == ScopeProject || s == ScopeUser
}

// Validate rejects an unknown scope before it can select a project or user
// memory partition.
func (s Scope) Validate() error {
	if !s.Valid() {
		return fmt.Errorf("agentmemory: invalid scope %q", s)
	}
	return nil
}

func (s Scope) String() string { return string(s) }

// ParseScope maps a stored token back to the closed Scope vocabulary.
func ParseScope(value string) (Scope, error) {
	scope := Scope(value)
	return scope, scope.Validate()
}

// Status is a memory item's place in the human-in-the-loop review lifecycle.
type Status string

const (
	// StatusActive — approved (or user-authored) memory: injected into the prompt
	// and returned by search.
	StatusActive Status = "active"
	// StatusPending — proposed by the extractor, awaiting the user's review. Not
	// injected or searched until approved.
	StatusPending Status = "pending"
	// StatusRejected — a tombstone for a proposal the user declined. Kept so the
	// same fact is not proposed again; never injected, searched, or shown.
	StatusRejected Status = "rejected"
)

// Valid reports whether s is a state in the memory review lifecycle.
func (s Status) Valid() bool {
	return s == StatusActive || s == StatusPending || s == StatusRejected
}

// Validate rejects a state outside the memory review lifecycle.
func (s Status) Validate() error {
	if !s.Valid() {
		return fmt.Errorf("agentmemory: invalid status %q", s)
	}
	return nil
}

func (s Status) String() string { return string(s) }

// ParseStatus maps a stored token back to the closed Status vocabulary.
func ParseStatus(value string) (Status, error) {
	status := Status(value)
	return status, status.Validate()
}

// Origin records how an item entered memory — its provenance for the review
// surface and for auto-curation eligibility: only auto items are rewritten by
// the extractor's fold; user items are never clobbered.
type Origin string

const (
	// OriginAuto — mined by the extractor and folded by curation.
	OriginAuto Origin = "auto"
	// OriginUser — authored or edited by the user; auto-curation never touches it.
	OriginUser Origin = "user"
)

// Valid reports whether o is a known memory provenance.
func (o Origin) Valid() bool {
	return o == OriginAuto || o == OriginUser
}

// Validate rejects provenance outside the closed Origin vocabulary.
func (o Origin) Validate() error {
	if !o.Valid() {
		return fmt.Errorf("agentmemory: invalid origin %q", o)
	}
	return nil
}

func (o Origin) String() string { return string(o) }

// ParseOrigin maps a stored token back to the closed Origin vocabulary.
func ParseOrigin(value string) (Origin, error) {
	origin := Origin(value)
	return origin, origin.Validate()
}

// ReviewDecision is the command applied to one pending proposal. It is not a
// Status: callers decide approve or reject, while the domain owns which state
// that decision produces.
type ReviewDecision string

const (
	ReviewApprove ReviewDecision = "approve"
	ReviewReject  ReviewDecision = "reject"
)

// Result returns the terminal review status selected by d.
func (d ReviewDecision) Result() (Status, error) {
	switch d {
	case ReviewApprove:
		return StatusActive, nil
	case ReviewReject:
		return StatusRejected, nil
	default:
		return "", fmt.Errorf("agentmemory: invalid review decision %q", d)
	}
}

// Item is one addressable unit of agent-maintained memory. ID is a stable
// handle that survives content edits; Content is the verbatim markdown injected
// into (or retrieved for) the model. Pinned items are always injected — the L1
// core — and are never auto-pruned. SessionID/Day carry provenance.
type Item struct {
	ID        string
	Scope     Scope
	Project   string // "" for ScopeUser
	Content   string
	Origin    Origin
	Status    Status
	Pinned    bool
	SessionID string
	Day       string
	CreatedAt time.Time
	UpdatedAt time.Time

	// Embedding is the item's content vector for semantic search. Populated only
	// by the search-fetch path ([Store.ItemsForSearch]); empty on ordinary reads
	// and until an embedder has run over the item.
	Embedding []float32
}

// NewProposal builds a mined memory item awaiting review: project-scoped, auto
// origin, pending status. The caller supplies the id and the clock.
func NewProposal(id, project, content string, now time.Time) (Item, error) {
	return newItem(id, ScopeProject, project, content, OriginAuto, StatusPending, now)
}

// NewUserItem builds a user-authored memory item: active immediately (the user
// is the author, so there is nothing to review).
func NewUserItem(id string, scope Scope, project, content string, now time.Time) (Item, error) {
	return newItem(id, scope, project, content, OriginUser, StatusActive, now)
}

func newItem(id string, scope Scope, project, content string, origin Origin, status Status, now time.Time) (Item, error) {
	now = now.UTC()
	item := Item{
		ID:        id,
		Scope:     scope,
		Project:   project,
		Content:   strings.TrimSpace(content),
		Origin:    origin,
		Status:    status,
		Day:       now.Format(time.DateOnly),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := item.Validate(); err != nil {
		return Item{}, err
	}
	return item, nil
}

// Validate protects the identity, partition, provenance, lifecycle, and time
// invariants of one durable memory item.
func (item Item) Validate() error {
	if strings.TrimSpace(item.ID) == "" {
		return errors.New("agentmemory: item id is required")
	}
	if err := item.Scope.Validate(); err != nil {
		return err
	}
	switch item.Scope {
	case ScopeProject:
		if strings.TrimSpace(item.Project) == "" {
			return errors.New("agentmemory: project scope requires a project")
		}
	case ScopeUser:
		if item.Project != "" {
			return errors.New("agentmemory: user scope forbids a project")
		}
	}
	if strings.TrimSpace(item.Content) == "" {
		return errors.New("agentmemory: item content is required")
	}
	if err := item.Origin.Validate(); err != nil {
		return err
	}
	if err := item.Status.Validate(); err != nil {
		return err
	}
	if item.Origin == OriginUser && item.Status != StatusActive {
		return errors.New("agentmemory: user-authored item must be active")
	}
	if item.CreatedAt.IsZero() || item.UpdatedAt.IsZero() {
		return errors.New("agentmemory: item timestamps are required")
	}
	if item.UpdatedAt.Before(item.CreatedAt) {
		return errors.New("agentmemory: item update precedes creation")
	}
	return nil
}

// FactBatch is one extraction boundary's project-scoped ledger append.
type FactBatch struct {
	Project    string
	SessionID  string
	Day        string
	Facts      []string
	CapturedAt time.Time
}

// Normalize validates the batch identity and canonicalizes already-parsed facts
// into a unique, trimmed plain-text list while preserving first-seen order.
// Parsing and rendering a model's Markdown response belong to the extraction
// adapter.
func (b FactBatch) Normalize() (FactBatch, error) {
	b.Project = strings.TrimSpace(b.Project)
	b.SessionID = strings.TrimSpace(b.SessionID)
	if b.Project == "" {
		return FactBatch{}, errors.New("agentmemory: fact batch project is required")
	}
	if b.SessionID == "" {
		return FactBatch{}, errors.New("agentmemory: fact batch session is required")
	}
	day, err := time.Parse(time.DateOnly, b.Day)
	if err != nil || day.Format(time.DateOnly) != b.Day {
		return FactBatch{}, fmt.Errorf("agentmemory: invalid ledger day %q", b.Day)
	}
	if b.CapturedAt.IsZero() {
		return FactBatch{}, errors.New("agentmemory: fact batch capture time is required")
	}
	b.Facts = normalizeFactList(b.Facts)
	return b, nil
}

// LedgerFact is one immutable fact in a project's daily ledger. Sequence is the
// durable ordering key and curation watermark.
type LedgerFact struct {
	Sequence   int64
	Day        string
	Content    string
	CapturedAt time.Time
}

// State is the curation watermark for a project: the highest ledger sequence
// already folded into the item set.
type State struct {
	Watermark int64
	UpdatedAt time.Time
}

func normalizeFactList(input []string) []string {
	var normalized []string
	seen := make(map[string]struct{})
	for _, fact := range input {
		fact = strings.TrimSpace(fact)
		if fact == "" {
			continue
		}
		if _, duplicate := seen[fact]; duplicate {
			continue
		}
		seen[fact] = struct{}{}
		normalized = append(normalized, fact)
	}
	return normalized
}
