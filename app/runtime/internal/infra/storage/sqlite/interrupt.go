package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// InterruptStore is the SQLite-backed durable open-interrupt registry for
// cross-restart resume — the single implementation each consumer's narrow
// interrupt port binds to. One row per parked run keyed by parent_run_id; the
// wire interrupt payload is stored as opaque JSON text, created_at as unix nanos
// for ordering. Put is UPSERT so re-recording the same parentRunId overwrites.
type InterruptStore struct {
	db *sql.DB
}

// NewInterruptStore binds the SQLite interrupt registry to db. db must
// have been opened via [Open] so the migration ran.
func NewInterruptStore(db *sql.DB) *InterruptStore {
	return &InterruptStore{db: db}
}

func (s *InterruptStore) Put(ctx context.Context, p interrupts.Pending) error {
	if p.ParentRunID == "" {
		return errors.New("sqlite: interrupt parentRunId is required")
	}
	var drained string
	if len(p.DrainedTools) > 0 {
		b, err := json.Marshal(p.DrainedTools)
		if err != nil {
			return fmt.Errorf("sqlite: encode drained tools: %w", err)
		}
		drained = string(b)
	}
	_, err := conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO interrupts(parent_run_id, session_id, turn_id, process_id, provider, model, interrupts, drained_tools, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(parent_run_id) DO UPDATE SET
		   session_id    = excluded.session_id,
		   turn_id       = excluded.turn_id,
		   process_id    = excluded.process_id,
		   provider      = excluded.provider,
		   model         = excluded.model,
		   interrupts    = excluded.interrupts,
		   drained_tools = excluded.drained_tools,
		   created_at    = excluded.created_at`,
		p.ParentRunID, p.SessionID, p.TurnID, p.ProcessID, p.Provider, p.Model, string(p.Interrupts), drained, p.CreatedAt.UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("sqlite: put interrupt: %w", err)
	}
	return nil
}

func (s *InterruptStore) List(ctx context.Context, sessionID string) ([]interrupts.Pending, error) {
	query := `SELECT parent_run_id, session_id, turn_id, process_id, provider, model, interrupts, drained_tools, created_at FROM interrupts`
	args := []any{}
	if sessionID != "" {
		query += ` WHERE session_id = ?`
		args = append(args, sessionID)
	}
	query += ` ORDER BY created_at`

	rows, err := conn(ctx, s.db).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list interrupts: %w", err)
	}
	defer rows.Close()

	out := make([]interrupts.Pending, 0)
	for rows.Next() {
		p, err := scanPending(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list interrupts: %w", err)
	}
	return out, nil
}

func (s *InterruptStore) Get(ctx context.Context, parentRunID string) (interrupts.Pending, bool, error) {
	row := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT parent_run_id, session_id, turn_id, process_id, provider, model, interrupts, drained_tools, created_at
		 FROM interrupts WHERE parent_run_id = ?`, parentRunID)
	p, err := scanPending(row)
	if errors.Is(err, sql.ErrNoRows) {
		return interrupts.Pending{}, false, nil
	}
	if err != nil {
		return interrupts.Pending{}, false, err
	}
	return p, true, nil
}

// Consume atomically reads AND deletes the pending interrupt for parentRunID
// (one DELETE ... RETURNING), or returns ok=false when none is recorded — the
// resume claim contract. A single statement means two concurrent resumes can't
// both observe the same open interrupt: one claims it, the other gets ok=false,
// so a non-idempotent tool never re-fires.
func (s *InterruptStore) Consume(ctx context.Context, parentRunID string) (interrupts.Pending, bool, error) {
	row := conn(ctx, s.db).QueryRowContext(ctx,
		`DELETE FROM interrupts WHERE parent_run_id = ?
		 RETURNING parent_run_id, session_id, turn_id, process_id, provider, model, interrupts, drained_tools, created_at`,
		parentRunID)
	p, err := scanPending(row)
	if errors.Is(err, sql.ErrNoRows) {
		return interrupts.Pending{}, false, nil
	}
	if err != nil {
		return interrupts.Pending{}, false, err
	}
	return p, true, nil
}

func (s *InterruptStore) Delete(ctx context.Context, parentRunID string) error {
	if _, err := conn(ctx, s.db).ExecContext(ctx,
		`DELETE FROM interrupts WHERE parent_run_id = ?`, parentRunID,
	); err != nil {
		return fmt.Errorf("sqlite: delete interrupt: %w", err)
	}
	return nil
}

// scanRow abstracts *sql.Row and *sql.Rows so one scan path serves Get +
// List.
type scanRow interface {
	Scan(dest ...any) error
}

func scanPending(row scanRow) (interrupts.Pending, error) {
	var (
		p         interrupts.Pending
		payload   string
		drained   string
		createdNs int64
	)
	if err := row.Scan(&p.ParentRunID, &p.SessionID, &p.TurnID, &p.ProcessID, &p.Provider, &p.Model, &payload, &drained, &createdNs); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return interrupts.Pending{}, err
		}
		return interrupts.Pending{}, fmt.Errorf("sqlite: scan interrupt: %w", err)
	}
	if payload != "" {
		p.Interrupts = []byte(payload)
	}
	if drained != "" {
		if err := json.Unmarshal([]byte(drained), &p.DrainedTools); err != nil {
			return interrupts.Pending{}, fmt.Errorf("sqlite: decode drained tools: %w", err)
		}
	}
	p.CreatedAt = time.Unix(0, createdNs).UTC()
	return p, nil
}
