package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// TrustStore records which project roots are trusted to run their project-scope
// hooks (internal/domain/hooks). A cloned repo's hooks stay inert until the user
// trusts the project explicitly. Global (~/.lyra) hooks need no entry.
type TrustStore struct {
	db *sql.DB
}

// NewTrustStore wires the given *sql.DB to the trusted-projects table.
func NewTrustStore(db *sql.DB) *TrustStore {
	return &TrustStore{db: db}
}

// IsTrusted reports whether projectRoot has been granted hook trust.
func (s *TrustStore) IsTrusted(ctx context.Context, projectRoot string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM trusted_projects WHERE project_root = ?`, projectRoot).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("sqlite: is-trusted: %w", err)
	}
	return true, nil
}

// Trust grants hook trust to projectRoot (idempotent upsert).
func (s *TrustStore) Trust(ctx context.Context, projectRoot string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO trusted_projects (project_root, trusted_at) VALUES (?, ?)
		 ON CONFLICT(project_root) DO UPDATE SET trusted_at = excluded.trusted_at`,
		projectRoot, time.Now().UTC().UnixNano())
	if err != nil {
		return fmt.Errorf("sqlite: trust project: %w", err)
	}
	return nil
}

// Untrust revokes hook trust for projectRoot. Idempotent.
func (s *TrustStore) Untrust(ctx context.Context, projectRoot string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM trusted_projects WHERE project_root = ?`, projectRoot)
	if err != nil {
		return fmt.Errorf("sqlite: untrust project: %w", err)
	}
	return nil
}

// List returns every trusted project root, newest grant first.
func (s *TrustStore) List(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT project_root FROM trusted_projects ORDER BY trusted_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list trusted: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var root string
		if err := rows.Scan(&root); err != nil {
			return nil, fmt.Errorf("sqlite: scan trusted: %w", err)
		}
		out = append(out, root)
	}
	return out, rows.Err()
}
