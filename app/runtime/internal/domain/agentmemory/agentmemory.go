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

// Scope selects the breadth of a memory item.
type Scope int

const (
	// ScopeProject — knowledge tied to one project directory: conventions,
	// build/test commands, decisions, gotchas. Keyed by the project path.
	ScopeProject Scope = iota
	// ScopeUser — cross-project knowledge about how the user works. Project is
	// empty. The mining path populates it from a later batch; the model carries
	// the scope from the start so storage and injection need no reshape then.
	ScopeUser
)

// String renders the scope as its stored token ("project" | "user").
func (s Scope) String() string {
	if s == ScopeUser {
		return "user"
	}
	return "project"
}

// ParseScope maps a stored token back to a Scope, defaulting to ScopeProject
// for the zero value and any unknown token.
func ParseScope(s string) Scope {
	if s == "user" {
		return ScopeUser
	}
	return ScopeProject
}

// Status is a memory item's place in the human-in-the-loop review lifecycle.
type Status int

const (
	// StatusActive — approved (or user-authored) memory: injected into the prompt
	// and returned by search.
	StatusActive Status = iota
	// StatusPending — proposed by the extractor, awaiting the user's review. Not
	// injected or searched until approved.
	StatusPending
	// StatusRejected — a tombstone for a proposal the user declined. Kept so the
	// same fact is not proposed again; never injected, searched, or shown.
	StatusRejected
)

// String renders the status as its stored token.
func (s Status) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusRejected:
		return "rejected"
	default:
		return "active"
	}
}

// ParseStatus maps a stored token back to a Status, defaulting to StatusActive.
func ParseStatus(s string) Status {
	switch s {
	case "pending":
		return StatusPending
	case "rejected":
		return StatusRejected
	default:
		return StatusActive
	}
}

// Origin records how an item entered memory — its provenance for the review
// surface and for auto-curation eligibility: only auto items are rewritten by
// the extractor's fold; user items are never clobbered.
type Origin int

const (
	// OriginAuto — mined by the extractor and folded by curation.
	OriginAuto Origin = iota
	// OriginUser — authored or edited by the user; auto-curation never touches it.
	OriginUser
)

// String renders the origin as its stored token ("auto" | "user").
func (o Origin) String() string {
	if o == OriginUser {
		return "user"
	}
	return "auto"
}

// ParseOrigin maps a stored token back to an Origin, defaulting to OriginAuto.
func ParseOrigin(s string) Origin {
	if s == "user" {
		return OriginUser
	}
	return OriginAuto
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
func NewProposal(id, project, content string, now time.Time) Item {
	return newItem(id, ScopeProject, project, content, OriginAuto, StatusPending, now)
}

// NewUserItem builds a user-authored memory item: active immediately (the user
// is the author, so there is nothing to review).
func NewUserItem(id string, scope Scope, project, content string, now time.Time) Item {
	return newItem(id, scope, project, content, OriginUser, StatusActive, now)
}

func newItem(id string, scope Scope, project, content string, origin Origin, status Status, now time.Time) Item {
	now = now.UTC()
	return Item{
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
// into a unique, trimmed list while preserving first-seen order. Parsing a
// model's Markdown response belongs to the extraction adapter.
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
		if !strings.HasPrefix(fact, "- ") {
			fact = "- " + fact
		}
		if _, duplicate := seen[fact]; duplicate {
			continue
		}
		seen[fact] = struct{}{}
		normalized = append(normalized, fact)
	}
	return normalized
}
