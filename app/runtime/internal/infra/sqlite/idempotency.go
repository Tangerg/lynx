package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/idempotency"
)

// IdempotencyStore persists replay records across runtime restarts.
type IdempotencyStore struct{ db *sql.DB }

// NewIdempotencyStore returns a replay store backed by db.
func NewIdempotencyStore(db *sql.DB) *IdempotencyStore { return &IdempotencyStore{db: db} }

// IdempotencyNamespace returns the opaque identity of db's durable replay
// store. It is published as a transport capability so a client never replays a
// persisted key into a different store that happens to occupy the same URL.
func IdempotencyNamespace(ctx context.Context, db *sql.DB) (string, error) {
	var namespace string
	if err := db.QueryRowContext(ctx,
		`SELECT idempotency_namespace FROM runtime_identity WHERE id = 1`,
	).Scan(&namespace); err != nil {
		return "", fmt.Errorf("sqlite: read idempotency namespace: %w", err)
	}
	if len(namespace) != len("idp_")+32 || !strings.HasPrefix(namespace, "idp_") {
		return "", errors.New("sqlite: invalid idempotency namespace")
	}
	for _, digit := range namespace[len("idp_"):] {
		if (digit < '0' || digit > '9') && (digit < 'a' || digit > 'f') {
			return "", errors.New("sqlite: invalid idempotency namespace")
		}
	}
	return namespace, nil
}

func (i *IdempotencyStore) Claim(ctx context.Context, key, fingerprint string) (record idempotency.Record, claimed bool, err error) {
	now := time.Now().Unix()
	// Route the claim's prune+insert+lookup through the shared tx seam rather than a
	// bare i.db.BeginTx: the pool runs at MaxOpenConns(1), so opening an independent
	// transaction while a caller's cross-store transaction is live would deadlock.
	// Standalone it still runs atomically (RunInTx begins its own).
	err = RunInTx(ctx, i.db, func(ctx context.Context) error {
		db := conn(ctx, i.db)
		// Only completed results expire. An empty payload is an unresolved
		// reservation: releasing it on elapsed wall time would let the same key
		// execute its business mutation again after a process crash, even though
		// the first mutation may already have committed.
		if _, execContextErr := db.ExecContext(ctx,
			`DELETE FROM idempotency_records WHERE expires_at <= ? AND length(payload) > 0`, now,
		); execContextErr != nil {
			return fmt.Errorf("sqlite: prune idempotency records: %w", execContextErr)
		}
		res, execContextErr := db.ExecContext(ctx, `INSERT INTO idempotency_records(
				key, fingerprint, payload, created_at, expires_at
			) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(key) DO NOTHING`,
			key, fingerprint, []byte{}, now, now+int64(idempotency.Retention/time.Second))
		if execContextErr != nil {
			return fmt.Errorf("sqlite: insert idempotency claim: %w", execContextErr)
		}
		changed, execContextErr := res.RowsAffected()
		if execContextErr != nil {
			return fmt.Errorf("sqlite: inspect idempotency claim: %w", execContextErr)
		}
		if changed != 0 {
			record, claimed = idempotency.Record{Key: key, Fingerprint: fingerprint}, true
			return nil
		}
		stored := idempotency.Record{Key: key}
		if scanErr := db.QueryRowContext(ctx,
			`SELECT fingerprint, payload FROM idempotency_records WHERE key = ?`, key,
		).Scan(&stored.Fingerprint, &stored.Payload); scanErr != nil {
			if errors.Is(scanErr, sql.ErrNoRows) {
				return idempotency.ErrClaimLost
			}
			return fmt.Errorf("sqlite: read idempotency claim: %w", scanErr)
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

func (i *IdempotencyStore) Complete(ctx context.Context, record idempotency.Record) error {
	now := time.Now().Unix()
	res, err := conn(ctx, i.db).ExecContext(ctx,
		`UPDATE idempotency_records SET payload = ?, expires_at = ?
		 WHERE key = ? AND fingerprint = ? AND length(payload) = 0`,
		record.Payload, now+int64(idempotency.Retention/time.Second), record.Key, record.Fingerprint)
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
	err = conn(ctx, i.db).QueryRowContext(ctx,
		`SELECT fingerprint, payload FROM idempotency_records WHERE key = ? AND expires_at > ?`,
		record.Key, now).Scan(&fingerprint, &payload)
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
