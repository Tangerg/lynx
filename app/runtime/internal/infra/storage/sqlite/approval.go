package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
)

// ApprovalRuleStore implements approval.RuleStore against SQLite — the
// persistent home for fine-grained "remember this decision" rules. Put is an
// upsert by the deterministic rule id (only the decision changes on
// re-remember; created_at is preserved). The DB must have been opened via
// [Open] so the approval_rules table exists.
type ApprovalRuleStore struct {
	db *sql.DB
}

var _ approval.RuleStore = (*ApprovalRuleStore)(nil)

// NewApprovalRuleStore wires the given *sql.DB to the approval.RuleStore surface.
func NewApprovalRuleStore(db *sql.DB) *ApprovalRuleStore {
	return &ApprovalRuleStore{db: db}
}

func (s *ApprovalRuleStore) Put(ctx context.Context, r approval.Rule) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO approval_rules (id, scope, scope_key, tool, subject, decision, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET decision = excluded.decision`,
		r.ID, string(r.Scope), r.ScopeKey, r.Tool, r.Subject, string(r.Decision),
		time.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("sqlite: put approval rule: %w", err)
	}
	return nil
}

func (s *ApprovalRuleStore) Visible(ctx context.Context, sessionID, projectDir string) ([]approval.Rule, error) {
	// Scope predicate expressed as a WHERE clause — the mirror of approval's
	// visible(): session rules for this session, project rules for this cwd
	// (skipped entirely when the call has no cwd), and all global rules.
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, scope, scope_key, tool, subject, decision FROM approval_rules
		 WHERE (scope = 'session' AND scope_key = ?)
		    OR (scope = 'project' AND ? <> '' AND scope_key = ?)
		    OR scope = 'global'
		 ORDER BY created_at DESC`,
		sessionID, projectDir, projectDir)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list approval rules: %w", err)
	}
	defer rows.Close()

	var out []approval.Rule
	for rows.Next() {
		var r approval.Rule
		var scope, decision string
		if err := rows.Scan(&r.ID, &scope, &r.ScopeKey, &r.Tool, &r.Subject, &decision); err != nil {
			return nil, fmt.Errorf("sqlite: scan approval rule: %w", err)
		}
		r.Scope = approval.Scope(scope)
		r.Decision = approval.Decision(decision)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *ApprovalRuleStore) Delete(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM approval_rules WHERE id = ?`, id); err != nil {
		return fmt.Errorf("sqlite: delete approval rule: %w", err)
	}
	return nil
}
