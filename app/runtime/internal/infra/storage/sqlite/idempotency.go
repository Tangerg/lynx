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
	// Route the claim's prune+insert+lookup through the shared tx seam rather than a
	// bare s.db.BeginTx: the pool runs at MaxOpenConns(1), so opening an independent
	// transaction while a caller's cross-store transaction is live would deadlock.
	// Standalone it still runs atomically (RunInTx begins its own).
	err = RunInTx(ctx, s.db, func(ctx context.Context) error {
		db := conn(ctx, s.db)
		if _, err := db.ExecContext(ctx, `DELETE FROM idempotency_records WHERE expires_at <= ?`, now); err != nil {
			return fmt.Errorf("sqlite: prune idempotency records: %w", err)
		}
		res, err := db.ExecContext(ctx, `INSERT INTO idempotency_records(
				key, fingerprint, payload, created_at, expires_at
			) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(key) DO NOTHING`,
			key, fingerprint, []byte{}, now, now+int64(idempotency.Retention/time.Second))
		if err != nil {
			return fmt.Errorf("sqlite: insert idempotency claim: %w", err)
		}
		changed, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: inspect idempotency claim: %w", err)
		}
		if changed != 0 {
			record, claimed = idempotency.Record{Key: key, Fingerprint: fingerprint}, true
			return nil
		}
		stored := idempotency.Record{Key: key}
		if err := db.QueryRowContext(ctx,
			`SELECT fingerprint, payload FROM idempotency_records WHERE key = ?`, key,
		).Scan(&stored.Fingerprint, &stored.Payload); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return idempotency.ErrClaimLost
			}
			return fmt.Errorf("sqlite: read idempotency claim: %w", err)
		}
		if stored.Fingerprint != fingerprint {
			return idempotency.ErrKeyConflict
		}
		record, claimed = stored, false
		return nil
	})
	if err != nil {
		return idempotency.Record{}, false, err
	}
	return record, claimed, nil
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
