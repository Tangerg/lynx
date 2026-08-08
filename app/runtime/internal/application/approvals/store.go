package approvals

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
)

// RuleStore persists approval rules. Matching and precedence live in rule.go;
// implementations own storage validation and scope filtering on read so corrupt
// records never enter policy evaluation.
type RuleStore interface {
	// Put upserts a rule by its id (deterministic over scope/key/tool/subject),
	// so re-remembering the same rule replaces the decision rather than piling
	// duplicates.
	Put(ctx context.Context, r approval.Rule) error

	// Visible returns every rule reachable from a session: its session-scoped
	// rules (ScopeKey == sessionID), its project's rules (ScopeKey ==
	// projectDir), and all global rules. Any tool — the domain filters by tool.
	Visible(ctx context.Context, sessionID, projectDir string) ([]approval.Rule, error)

	// Delete removes one rule by id; removing a missing id is not an error.
	Delete(ctx context.Context, id string) error
}
