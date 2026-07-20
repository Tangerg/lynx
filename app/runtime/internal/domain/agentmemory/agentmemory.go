// Package agentmemory defines Lyra's agent-maintained long-term memory: the
// durable facts the agent mines from conversations, folded into addressable
// memory items. It is distinct from the human-authored LYRA.md cascade
// (package knowledge) — that stays a user-owned file the agent never writes;
// this is agent-owned, curated from an append-only fact ledger into discrete,
// individually addressable items.
//
// Which items get injected into a prompt, and in what order, is a policy of
// this domain via [Render]; the agent-execution adapter only wraps the rendered
// body with its heading.
package agentmemory

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

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

// FactBatch is one extraction boundary's project-scoped ledger append.
type FactBatch struct {
	Project    string
	SessionID  string
	Day        string
	Facts      []string
	CapturedAt time.Time
}

// Normalize validates the batch identity and canonicalizes its facts into
// unique markdown bullets while preserving first-seen order.
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
	b.Facts = NormalizeFacts(strings.Join(b.Facts, "\n"))
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

// Store is the agent-memory persistence contract, implemented by the SQLite
// backend. The fact ledger is project-scoped raw capture; the item set is the
// curated projection reconciled from it.
type Store interface {
	// AppendLedger inserts facts not already present in the project (dedup by
	// content digest) and returns the newly inserted facts in sequence order.
	AppendLedger(ctx context.Context, batch FactBatch) ([]LedgerFact, error)

	// PendingLedger lists a project's facts strictly after watermark in sequence
	// order. limit must be positive so every curation call has an explicit bound.
	PendingLedger(ctx context.Context, project string, watermark int64, limit int) ([]LedgerFact, error)

	// State returns the project's curation watermark (zero value for an unknown
	// project).
	State(ctx context.Context, project string) (State, error)

	// Reconcile folds the project's ledger through `through` into its
	// auto-origin item set: it replaces those items with contents (preserving a
	// stable ID and provenance for any item whose content is unchanged, matched
	// by digest), never touching pinned or user-authored items, and advances the
	// watermark. expectedWatermark is a compare-and-swap guard around the LLM
	// curation call; a lost race returns published=false without clobbering a
	// fresher fold. `through` must be a ledger sequence greater than
	// expectedWatermark and belonging to the project.
	Reconcile(ctx context.Context, project string, expectedWatermark, through int64, contents []string, now time.Time) (published bool, err error)

	// Items lists the active items for (scope, project): pinned first, then most
	// recently updated. Empty scope/project is valid (returns no items).
	Items(ctx context.Context, scope Scope, project string) ([]Item, error)

	// ItemsForSearch lists the (scope, project) items with their Embedding
	// populated, for the [Searcher] to rank. The corpus is small (a project's
	// curated set), so the searcher scores in-process rather than in SQL.
	ItemsForSearch(ctx context.Context, scope Scope, project string) ([]Item, error)

	// UnembeddedItems lists the (scope, project) items that still lack an
	// embedding, so a configured embedder can backfill them.
	UnembeddedItems(ctx context.Context, scope Scope, project string) ([]Item, error)

	// SetEmbeddings stores a content vector for each item id.
	SetEmbeddings(ctx context.Context, vectors map[string][]float32) error
}

// NormalizeFacts converts an extraction response into stable markdown bullets.
// Blank lines, fences, and the NO_FACTS sentinel are discarded; duplicate facts
// within one response collapse without reordering the survivors.
func NormalizeFacts(markdown string) []string {
	var facts []string
	seen := make(map[string]struct{})
	for line := range strings.SplitSeq(markdown, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "```" || strings.EqualFold(line, "NO_FACTS") {
			continue
		}
		line = trimBullet(line)
		if line == "" {
			continue
		}
		fact := "- " + line
		if _, duplicate := seen[fact]; duplicate {
			continue
		}
		seen[fact] = struct{}{}
		facts = append(facts, fact)
	}
	return slices.Clip(facts)
}

func trimBullet(line string) string {
	if len(line) >= 2 && (line[0] == '-' || line[0] == '*' || line[0] == '+') && line[1] == ' ' {
		return strings.TrimSpace(line[2:])
	}
	if index := strings.IndexByte(line, '.'); index > 0 && index+1 < len(line) && line[index+1] == ' ' {
		for _, digit := range line[:index] {
			if digit < '0' || digit > '9' {
				return line
			}
		}
		return strings.TrimSpace(line[index+2:])
	}
	return line
}

const charsPerToken = 4

// EstimateTokens approximates a token count for a memory budget: one token per
// non-ASCII rune (CJK and friends tokenize roughly per-character) plus one per
// [charsPerToken] ASCII bytes. Deliberately cheap and provider-agnostic.
func EstimateTokens(text string) int {
	ascii := 0
	tokens := 0
	for _, r := range text {
		if r <= 0x7f {
			ascii++
		} else {
			tokens++
		}
	}
	return tokens + (ascii+charsPerToken-1)/charsPerToken
}

// Render assembles the memory body injected into the system prompt: pinned
// items first, then the rest by recency, accumulated until maxTokens would be
// exceeded. The budget is a defensive whole-inject bound for the always-on
// core; retrieval trims the wider corpus separately. Returns "" when there is
// nothing to inject. maxTokens <= 0 means unbounded.
func Render(items []Item, maxTokens int) string {
	if len(items) == 0 {
		return ""
	}
	ordered := slices.Clone(items)
	slices.SortStableFunc(ordered, func(a, b Item) int {
		if a.Pinned != b.Pinned {
			if a.Pinned {
				return -1
			}
			return 1
		}
		return b.UpdatedAt.Compare(a.UpdatedAt)
	})
	var b strings.Builder
	used := 0
	for _, item := range ordered {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		cost := EstimateTokens(content)
		if maxTokens > 0 && b.Len() > 0 && used+cost > maxTokens {
			break
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(content)
		used += cost
	}
	return b.String()
}
