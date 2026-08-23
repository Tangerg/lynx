package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/idempotency"
)

// SQLite compares persisted timestamps lexicographically. A fixed-width
// fractional component keeps chronological and lexical order identical.
const idempotencyTimeLayout = "2006-01-02T15:04:05.000000000Z07:00"

// IdempotencyStore is the durable adapter for operation replay reservations.
// Completed outcomes expire after retention; unresolved reservations never do,
// because elapsed time cannot prove whether their business mutation committed.
type IdempotencyStore struct {
	database  *sql.DB
	retention time.Duration
	clock     func() time.Time
}

func NewIdempotencyStore(database *Database, retention time.Duration) (*IdempotencyStore, error) {
	if database == nil || database.database == nil || retention <= 0 {
		return nil, errors.New("sqlite: open database and positive idempotency retention are required")
	}
	return &IdempotencyStore{
		database: database.database, retention: retention, clock: time.Now,
	}, nil
}

func (store *IdempotencyStore) Claim(
	ctx context.Context,
	key string,
	fingerprint string,
) (record idempotency.Record, claimed bool, err error) {
	now := store.clock().UTC()
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return idempotency.Record{}, false, fmt.Errorf("sqlite: begin idempotency claim: %w", err)
	}
	defer transaction.Rollback()

	cutoff := now.Add(-store.retention).Format(idempotencyTimeLayout)
	if _, err := transaction.ExecContext(ctx, `
		DELETE FROM idempotency_outcomes
		WHERE state = 'complete' AND updated_at <= ?`, cutoff); err != nil {
		return idempotency.Record{}, false, fmt.Errorf("sqlite: prune idempotency outcomes: %w", err)
	}
	timestamp := now.Format(idempotencyTimeLayout)
	result, err := transaction.ExecContext(ctx, `
		INSERT INTO idempotency_outcomes (
			key, fingerprint, state, body, created_at, updated_at
		) VALUES (?, ?, 'pending', NULL, ?, ?)
		ON CONFLICT(key) DO NOTHING`, key, fingerprint, timestamp, timestamp)
	if err != nil {
		return idempotency.Record{}, false, fmt.Errorf("sqlite: reserve idempotency key: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return idempotency.Record{}, false, fmt.Errorf("sqlite: inspect idempotency reservation: %w", err)
	}
	if changed != 0 {
		if err := transaction.Commit(); err != nil {
			return idempotency.Record{}, false, fmt.Errorf("sqlite: commit idempotency reservation: %w", err)
		}
		return idempotency.Record{Key: key, Fingerprint: fingerprint}, true, nil
	}

	var (
		storedFingerprint string
		state             string
		body              sql.NullString
	)
	if err := transaction.QueryRowContext(ctx, `
		SELECT fingerprint, state, body
		FROM idempotency_outcomes WHERE key = ?`, key,
	).Scan(&storedFingerprint, &state, &body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return idempotency.Record{}, false, idempotency.ErrClaimLost
		}
		return idempotency.Record{}, false, fmt.Errorf("sqlite: read idempotency reservation: %w", err)
	}
	if storedFingerprint != fingerprint {
		return idempotency.Record{}, false, idempotency.ErrKeyConflict
	}
	if state != "pending" && (state != "complete" || !body.Valid || body.String == "") {
		return idempotency.Record{}, false, errors.New("sqlite: idempotency reservation is corrupt")
	}
	if err := transaction.Commit(); err != nil {
		return idempotency.Record{}, false, fmt.Errorf("sqlite: commit idempotency lookup: %w", err)
	}
	record = idempotency.Record{Key: key, Fingerprint: fingerprint}
	if body.Valid {
		record.Payload = []byte(body.String)
	}
	return record, false, nil
}

func (store *IdempotencyStore) Complete(
	ctx context.Context,
	record idempotency.Record,
) (idempotency.Record, error) {
	if len(record.Payload) == 0 {
		return idempotency.Record{}, errors.New("sqlite: idempotency completion payload is empty")
	}
	now := store.clock().UTC().Format(idempotencyTimeLayout)
	result, err := store.database.ExecContext(ctx, `
		UPDATE idempotency_outcomes
		SET state = 'complete', body = ?, updated_at = ?
		WHERE key = ? AND fingerprint = ? AND state = 'pending'`,
		string(record.Payload), now, record.Key, record.Fingerprint)
	if err != nil {
		return idempotency.Record{}, fmt.Errorf("sqlite: complete idempotency reservation: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return idempotency.Record{}, fmt.Errorf("sqlite: inspect idempotency completion: %w", err)
	}
	if changed != 0 {
		record.Payload = append([]byte(nil), record.Payload...)
		return record, nil
	}

	var (
		fingerprint string
		state       string
		body        sql.NullString
	)
	err = store.database.QueryRowContext(ctx, `
		SELECT fingerprint, state, body
		FROM idempotency_outcomes WHERE key = ?`, record.Key,
	).Scan(&fingerprint, &state, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return idempotency.Record{}, idempotency.ErrClaimLost
	}
	if err != nil {
		return idempotency.Record{}, fmt.Errorf("sqlite: inspect completed idempotency reservation: %w", err)
	}
	if fingerprint != record.Fingerprint {
		return idempotency.Record{}, idempotency.ErrKeyConflict
	}
	if state == "complete" && body.Valid && body.String != "" {
		record.Payload = []byte(body.String)
		return record, nil
	}
	return idempotency.Record{}, idempotency.ErrClaimLost
}

var _ idempotency.Store = (*IdempotencyStore)(nil)
