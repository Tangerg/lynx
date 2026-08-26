// Package agentmemory defines Lyra's agent-maintained long-term memory: the
// durable facts the agent mines from conversations, folded into addressable
// memory items. It is distinct from the human-authored LYRA.md cascade
// (package knowledge) — that stays a user-owned file the agent never writes;
// this is agent-owned, curated from an append-only fact ledger into discrete,
// individually addressable items.
//
// Which items get injected into an agent prompt, and in what order, is a prompt
// composition policy. This domain owns the durable memory values, lifecycle,
// and content invariants only.
package agentmemory

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// MaxContentCharacters is the one durable-item and ledger-fact content
	// bound. It is expressed in Unicode code points because memory is durable
	// text, not a byte payload. The model prompt's conservative token estimator
	// charges every non-ASCII code point as one token, so one valid item can
	// never exceed its 4096-token whole-item prompt budget.
	MaxContentCharacters = 4096

	// MaxFactsPerBatch bounds one extraction result and therefore one ledger
	// append transaction. Deduplication happens after this admission check: a
	// provider cannot evade the request envelope with repeated output.
	MaxFactsPerBatch = 32

	// MaxCurationProposals bounds one curator generation. Approved memory is
	// sticky, so this is the number of review proposals one fold may create,
	// not the total durable target capacity.
	MaxCurationProposals = 32

	// MaxLedgerFoldFacts bounds the cursor page loaded for one curation pass.
	// The byte envelope may choose a shorter prefix, but no configuration can
	// turn the database read preceding that envelope into an unbounded query.
	MaxLedgerFoldFacts = 128

	// MaxVisiblePerTarget makes the non-paginated management and prompt read
	// models complete but finite. Active and pending items are visible; rejected
	// tombstones have their own larger retention window below.
	MaxVisiblePerTarget = 512

	// MaxRejectedPerTarget bounds negative-history retention. Recent rejections
	// still suppress repeated proposals without turning that suppression log
	// into an unbounded durable collection.
	MaxRejectedPerTarget = 2048
)

// NormalizeContent returns the canonical representation accepted at every
// Agent Memory write boundary.
func NormalizeContent(content string) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", errors.New("agentmemory: item content is required")
	}
	if !utf8.ValidString(content) {
		return "", errors.New("agentmemory: item content must be valid UTF-8")
	}
	if utf8.RuneCountInString(content) > MaxContentCharacters {
		return "", fmt.Errorf(
			"agentmemory: item content exceeds %d Unicode characters",
			MaxContentCharacters,
		)
	}
	return content, nil
}

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

// ErrTargetFull reports that a new visible item would exceed the finite
// complete-list capacity of its project or user target.
var ErrTargetFull = errors.New("agentmemory: target is full")

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

// Result returns the terminal review status selected by r.
func (r ReviewDecision) Result() (Status, error) {
	switch r {
	case ReviewApprove:
		return StatusActive, nil
	case ReviewReject:
		return StatusRejected, nil
	default:
		return "", fmt.Errorf("agentmemory: invalid review decision %q", r)
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

	// EmbeddingSpace and Embedding are a search-only cache pair. Space identifies
	// the embedder that produced the vector; both fields are empty on ordinary
	// reads and until semantic search has cached the current content.
	EmbeddingSpace string
	Embedding      []float32
}

// EmbeddingUpdate conditionally caches one item's current-content vector.
// ContentDigest prevents a late embedding result from being attached after the
// item was edited, while Space prevents vectors from different models being
// compared as though they shared one coordinate system.
type EmbeddingUpdate struct {
	ItemID        string
	ContentDigest string
	Space         string
	Vector        []float32
}

// NewEmbeddingUpdate binds a vector to the exact item content and embedding
// space that produced it.
func NewEmbeddingUpdate(item Item, space string, vector []float32) (EmbeddingUpdate, error) {
	if strings.TrimSpace(item.ID) == "" {
		return EmbeddingUpdate{}, errors.New("agentmemory: embedding item id is required")
	}
	content, err := NormalizeContent(item.Content)
	if err != nil {
		return EmbeddingUpdate{}, fmt.Errorf("agentmemory: embedding item content: %w", err)
	}
	if content != item.Content {
		return EmbeddingUpdate{}, errors.New("agentmemory: embedding item content is not canonical")
	}
	if err := validateEmbedding(space, vector); err != nil {
		return EmbeddingUpdate{}, err
	}
	return EmbeddingUpdate{
		ItemID:        item.ID,
		ContentDigest: Digest(item.Content),
		Space:         space,
		Vector:        slices.Clone(vector),
	}, nil
}

// Validate protects a cache update received at a persistence boundary.
func (e EmbeddingUpdate) Validate() error {
	if strings.TrimSpace(e.ItemID) == "" {
		return errors.New("agentmemory: embedding item id is required")
	}
	if strings.TrimSpace(e.ContentDigest) == "" {
		return errors.New("agentmemory: embedding content digest is required")
	}
	return validateEmbedding(e.Space, e.Vector)
}

func validateEmbedding(space string, vector []float32) error {
	if space == "" || strings.TrimSpace(space) != space {
		return errors.New("agentmemory: embedding space is required without surrounding whitespace")
	}
	return ValidateEmbeddingVector(vector)
}

// ValidateEmbeddingVector rejects cache values that cannot participate in a
// deterministic similarity calculation.
func ValidateEmbeddingVector(vector []float32) error {
	if len(vector) == 0 {
		return errors.New("agentmemory: embedding vector is required")
	}
	for _, value := range vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return errors.New("agentmemory: embedding vector must be finite")
		}
	}
	return nil
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
	content, err := NormalizeContent(content)
	if err != nil {
		return Item{}, err
	}
	now = now.UTC()
	item := Item{
		ID:        id,
		Scope:     scope,
		Project:   project,
		Content:   content,
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
func (i Item) Validate() error {
	if strings.TrimSpace(i.ID) == "" {
		return errors.New("agentmemory: item id is required")
	}
	if err := i.Scope.Validate(); err != nil {
		return err
	}
	switch i.Scope {
	case ScopeProject:
		if strings.TrimSpace(i.Project) == "" {
			return errors.New("agentmemory: project scope requires a project")
		}
	case ScopeUser:
		if i.Project != "" {
			return errors.New("agentmemory: user scope forbids a project")
		}
	}
	content, err := NormalizeContent(i.Content)
	if err != nil {
		return err
	}
	if content != i.Content {
		return errors.New("agentmemory: item content is not canonical")
	}
	if err := i.Origin.Validate(); err != nil {
		return err
	}
	if err := i.Status.Validate(); err != nil {
		return err
	}
	if i.Origin == OriginUser && i.Status != StatusActive {
		return errors.New("agentmemory: user-authored item must be active")
	}
	if i.CreatedAt.IsZero() || i.UpdatedAt.IsZero() {
		return errors.New("agentmemory: item timestamps are required")
	}
	if i.UpdatedAt.Before(i.CreatedAt) {
		return errors.New("agentmemory: item update precedes creation")
	}
	if i.EmbeddingSpace == "" && len(i.Embedding) == 0 {
		return nil
	}
	if err := validateEmbedding(i.EmbeddingSpace, i.Embedding); err != nil {
		return fmt.Errorf("agentmemory: item embedding: %w", err)
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
// Parsing and rendering a model's Markdown response belong to the caller.
func (f FactBatch) Normalize() (FactBatch, error) {
	f.Project = strings.TrimSpace(f.Project)
	f.SessionID = strings.TrimSpace(f.SessionID)
	if f.Project == "" {
		return FactBatch{}, errors.New("agentmemory: fact batch project is required")
	}
	if f.SessionID == "" {
		return FactBatch{}, errors.New("agentmemory: fact batch session is required")
	}
	day, err := time.Parse(time.DateOnly, f.Day)
	if err != nil || day.Format(time.DateOnly) != f.Day {
		return FactBatch{}, fmt.Errorf("agentmemory: invalid ledger day %q", f.Day)
	}
	if f.CapturedAt.IsZero() {
		return FactBatch{}, errors.New("agentmemory: fact batch capture time is required")
	}
	f.Facts, err = normalizeFactList(f.Facts, MaxFactsPerBatch, "fact batch")
	if err != nil {
		return FactBatch{}, err
	}
	return f, nil
}

// LedgerFact is one immutable fact in a project's daily ledger. Sequence is the
// durable ordering key and curation watermark.
type LedgerFact struct {
	Sequence   int64
	Day        string
	Content    string
	CapturedAt time.Time
}

// Validate protects one ledger fact read back from durable storage before it
// can enter the curation prompt.
func (l LedgerFact) Validate() error {
	if l.Sequence <= 0 {
		return errors.New("agentmemory: ledger fact sequence must be positive")
	}
	day, err := time.Parse(time.DateOnly, l.Day)
	if err != nil || day.Format(time.DateOnly) != l.Day {
		return fmt.Errorf("agentmemory: invalid ledger day %q", l.Day)
	}
	content, err := NormalizeContent(l.Content)
	if err != nil {
		return fmt.Errorf("agentmemory: ledger fact content: %w", err)
	}
	if content != l.Content {
		return errors.New("agentmemory: ledger fact content is not canonical")
	}
	if l.CapturedAt.IsZero() {
		return errors.New("agentmemory: ledger fact capture time is required")
	}
	return nil
}

// State is the curation watermark for a project: the highest ledger sequence
// already folded into the item set.
type State struct {
	Watermark int64
	UpdatedAt time.Time
}

func normalizeFactList(input []string, maximum int, collection string) ([]string, error) {
	if len(input) > maximum {
		return nil, fmt.Errorf("agentmemory: %s exceeds %d items", collection, maximum)
	}
	var normalized []string
	seen := make(map[string]struct{})
	for _, fact := range input {
		if strings.TrimSpace(fact) == "" {
			continue
		}
		var err error
		fact, err = NormalizeContent(fact)
		if err != nil {
			return nil, fmt.Errorf("agentmemory: invalid %s item: %w", collection, err)
		}
		if _, duplicate := seen[fact]; duplicate {
			continue
		}
		seen[fact] = struct{}{}
		normalized = append(normalized, fact)
	}
	return normalized, nil
}
