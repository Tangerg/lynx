package approval

import "context"

// RuleStore persists approval rules. Matching and precedence live in rule.go;
// adapters own storage validation and scope filtering on read so corrupt rows
// never enter policy evaluation. Defined here (the consumer) per DIP;
// production wires the sqlite-backed store (an in-memory implementation for
// tests lives in the approvaltest package).
type RuleStore interface {
	// Put upserts a rule by its id (deterministic over scope/key/tool/subject),
	// so re-remembering the same rule replaces the decision rather than piling
	// duplicates.
	Put(ctx context.Context, r Rule) error

	// Visible returns every rule reachable from a session: its session-scoped
	// rules (ScopeKey == sessionID), its project's rules (ScopeKey ==
	// projectDir), and all global rules. Any tool — the domain filters by tool.
	Visible(ctx context.Context, sessionID, projectDir string) ([]Rule, error)

	// Delete removes one rule by id; removing a missing id is not an error.
	Delete(ctx context.Context, id string) error

	// DeleteSession removes only rules owned by sessionID. Project and global
	// rules outlive sessions and must remain untouched.
	DeleteSession(ctx context.Context, sessionID string) error
}
