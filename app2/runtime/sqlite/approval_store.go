package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/approvalpolicy"
)

func (database *Database) GetApprovalMode(ctx context.Context) (approvalpolicy.Mode, error) {
	var mode approvalpolicy.Mode
	err := database.database.QueryRowContext(ctx, `
		SELECT mode FROM approval_state WHERE singleton = 1`).Scan(&mode)
	if errors.Is(err, sql.ErrNoRows) {
		return approvalpolicy.ModeBalanced, nil
	}
	if err != nil {
		return "", fmt.Errorf("sqlite: get approval mode: %w", err)
	}
	if !mode.Valid() {
		return "", fmt.Errorf("sqlite: stored approval mode %q is invalid", mode)
	}
	return mode, nil
}

func (database *Database) SetApprovalMode(ctx context.Context, mode approvalpolicy.Mode) (bool, error) {
	if !mode.Valid() {
		return false, approvalpolicy.ErrInvalid
	}
	result, err := database.database.ExecContext(ctx, `
		INSERT INTO approval_state (singleton, mode, revision, updated_at)
		SELECT 1, ?, 1, ?
		WHERE ? != 'balanced'
			OR EXISTS (SELECT 1 FROM approval_state WHERE singleton = 1)
		ON CONFLICT(singleton) DO UPDATE SET
			mode = excluded.mode,
			revision = approval_state.revision + 1,
			updated_at = excluded.updated_at
		WHERE approval_state.mode != excluded.mode`, mode, encodeTime(time.Now()), mode)
	if err != nil {
		return false, fmt.Errorf("sqlite: set approval mode: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("sqlite: inspect approval mode update: %w", err)
	}
	return changed > 0, nil
}

func (database *Database) ListVisibleApprovalRules(
	ctx context.Context,
	sessionID string,
	projectDir string,
) ([]approvalpolicy.Rule, error) {
	rows, err := database.database.QueryContext(ctx, `
		SELECT id, scope, scope_key, tool, subject, match_kind, decision, created_at, updated_at
		FROM approval_rules
		WHERE scope = 'global'
			OR (scope = 'session' AND scope_key = ?)
			OR (scope = 'project' AND scope_key = ?)
		ORDER BY CASE scope WHEN 'session' THEN 0 WHEN 'project' THEN 1 ELSE 2 END,
			updated_at DESC, id`, sessionID, projectDir)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list visible approval rules: %w", err)
	}
	defer rows.Close()

	rules := make([]approvalpolicy.Rule, 0)
	for rows.Next() {
		var state approvalpolicy.RuleState
		var createdAt string
		var updatedAt string
		if err := rows.Scan(
			&state.ID, &state.Scope, &state.ScopeKey, &state.Tool, &state.Subject,
			&state.MatchKind, &state.Decision, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("sqlite: scan approval rule: %w", err)
		}
		state.CreatedAt, err = decodeTime(createdAt)
		if err != nil {
			return nil, fmt.Errorf("sqlite: decode approval rule creation: %w", err)
		}
		state.UpdatedAt, err = decodeTime(updatedAt)
		if err != nil {
			return nil, fmt.Errorf("sqlite: decode approval rule update: %w", err)
		}
		rule, err := approvalpolicy.Rehydrate(state)
		if err != nil {
			return nil, fmt.Errorf("sqlite: rehydrate approval rule %q: %w", state.ID, err)
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate approval rules: %w", err)
	}
	return rules, nil
}

func (database *Database) PutApprovalRule(ctx context.Context, rule approvalpolicy.Rule) (bool, error) {
	state := rule.State()
	var sessionID any
	if state.Scope == approvalpolicy.ScopeSession {
		sessionID = state.ScopeKey
	}
	result, err := database.database.ExecContext(ctx, `
		INSERT INTO approval_rules (
			id, scope, scope_key, session_id, tool, subject, match_kind,
			decision, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			decision = excluded.decision,
			updated_at = excluded.updated_at
		WHERE approval_rules.decision != excluded.decision`,
		state.ID, state.Scope, state.ScopeKey, sessionID, state.Tool, state.Subject,
		state.MatchKind, state.Decision, encodeTime(state.CreatedAt), encodeTime(state.UpdatedAt),
	)
	if err != nil {
		return false, fmt.Errorf("sqlite: put approval rule %q: %w", state.ID, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("sqlite: inspect approval rule update: %w", err)
	}
	return changed > 0, nil
}

func (database *Database) DeleteApprovalRule(ctx context.Context, id string) (bool, error) {
	result, err := database.database.ExecContext(ctx, `DELETE FROM approval_rules WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("sqlite: delete approval rule %q: %w", id, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("sqlite: inspect approval rule deletion: %w", err)
	}
	return changed > 0, nil
}
