package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/idempotency"
)

// IdempotencyStore persists replay records across runtime restarts.
type IdempotencyStore struct{ db *sql.DB }

// NewIdempotencyStore returns a replay store backed by db.
func NewIdempotencyStore(db *sql.DB) *IdempotencyStore { return &IdempotencyStore{db: db} }

func (s *IdempotencyStore) Claim(ctx context.Context, key, fingerprint string) (record idempotency.Record, claimed bool, err error) {
	now := time.Now().Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return idempotency.Record{}, false, fmt.Errorf("sqlite: begin idempotency claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM idempotency_records WHERE expires_at <= ?`, now); err != nil {
		return idempotency.Record{}, false, fmt.Errorf("sqlite: prune idempotency records: %w", err)
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO idempotency_records(
			key, fingerprint, payload, created_at, expires_at
		) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(key) DO NOTHING`,
		key, fingerprint, []byte{}, now, now+int64(idempotency.Retention/time.Second))
	if err != nil {
		return idempotency.Record{}, false, fmt.Errorf("sqlite: insert idempotency claim: %w", err)
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return idempotency.Record{}, false, fmt.Errorf("sqlite: inspect idempotency claim: %w", err)
	}
	if changed != 0 {
		if err := tx.Commit(); err != nil {
			return idempotency.Record{}, false, fmt.Errorf("sqlite: commit idempotency claim: %w", err)
		}
		return idempotency.Record{Key: key, Fingerprint: fingerprint}, true, nil
	}
	record.Key = key
	if err := tx.QueryRowContext(ctx,
		`SELECT fingerprint, payload FROM idempotency_records WHERE key = ?`, key,
	).Scan(&record.Fingerprint, &record.Payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return idempotency.Record{}, false, idempotency.ErrClaimLost
		}
		return idempotency.Record{}, false, fmt.Errorf("sqlite: read idempotency claim: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return idempotency.Record{}, false, fmt.Errorf("sqlite: commit idempotency lookup: %w", err)
	}
	if record.Fingerprint != fingerprint {
		return idempotency.Record{}, false, idempotency.ErrKeyConflict
	}
	return record, false, nil
}

func (s *IdempotencyStore) Complete(ctx context.Context, record idempotency.Record) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE idempotency_records SET payload = ?
		 WHERE key = ? AND fingerprint = ? AND length(payload) = 0 AND expires_at > ?`,
		record.Payload, record.Key, record.Fingerprint, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("sqlite: complete idempotency claim: %w", err)
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect idempotency completion: %w", err)
	}
	if changed != 0 {
		return nil
	}
	var fingerprint string
	var payload []byte
	err = s.db.QueryRowContext(ctx,
		`SELECT fingerprint, payload FROM idempotency_records WHERE key = ? AND expires_at > ?`,
		record.Key, time.Now().Unix()).Scan(&fingerprint, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return idempotency.ErrClaimLost
	}
	if err != nil {
		return fmt.Errorf("sqlite: inspect completed idempotency claim: %w", err)
	}
	if fingerprint != record.Fingerprint {
		return idempotency.ErrKeyConflict
	}
	if len(payload) == 0 {
		return idempotency.ErrClaimLost
	}
	return nil
}
