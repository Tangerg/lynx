package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// InterruptStore is the SQLite-backed durable open-interrupt registry for
// cross-restart resume — the single implementation each consumer's narrow
// interrupt port binds to. One row per parked run keyed by run_id (the stable
// logical run). The canonical typed interrupt union is serialized as an adapter
// detail; protocol payloads never enter this store. Timestamps use unix nanos.
// Put is UPSERT so re-recording the same runId overwrites (a run parks at most
// once at a time).
type InterruptStore struct {
	db *sql.DB
}

type drainedToolRow struct {
	ItemID    string `json:"itemId"`
	CallID    string `json:"callId,omitempty"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// NewInterruptStore binds the SQLite interrupt registry to a database opened via
// [Open].
func NewInterruptStore(db *sql.DB) *InterruptStore {
	return &InterruptStore{db: db}
}

func (s *InterruptStore) Put(ctx context.Context, p interrupts.Pending) error {
	if p.RunID == "" {
		return errors.New("sqlite: interrupt runId is required")
	}
	var drained string
	payload, err := json.Marshal(p.Interrupts)
	if err != nil {
		return fmt.Errorf("sqlite: encode interrupts: %w", err)
	}
	if len(p.DrainedTools) > 0 {
		b, err := json.Marshal(drainedToolRows(p.DrainedTools))
		if err != nil {
			return fmt.Errorf("sqlite: encode drained tools: %w", err)
		}
		drained = string(b)
	}
	accounting, err := encodeRunAccounting(p.Metrics, p.Limits)
	if err != nil {
		return fmt.Errorf("sqlite: put interrupt: %w", err)
	}
	profile, err := encodeRunProtocolProfile(p.ProtocolProfile)
	if err != nil {
		return fmt.Errorf("sqlite: put interrupt: %w", err)
	}
	_, err = conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO interrupts(run_id, session_id, turn_id, process_id, provider, model, payload, drained_tools, accounting, protocol_profile, run_created_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(run_id) DO UPDATE SET
		   session_id      = excluded.session_id,
		   turn_id         = excluded.turn_id,
		   process_id      = excluded.process_id,
		   provider        = excluded.provider,
		   model           = excluded.model,
		   payload         = excluded.payload,
		   drained_tools   = excluded.drained_tools,
		   accounting      = excluded.accounting,
		   protocol_profile = excluded.protocol_profile,
		   run_created_at  = excluded.run_created_at,
		   created_at      = excluded.created_at`,
		p.RunID, p.SessionID, p.TurnID, p.ProcessID, p.ModelSelection.Provider(), p.ModelSelection.Model(), string(payload), drained, accounting, profile, p.RunCreatedAt.UnixNano(), p.CreatedAt.UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("sqlite: put interrupt: %w", err)
	}
	return nil
}

const interruptColumns = `run_id, session_id, turn_id, process_id, provider, model, payload, drained_tools, accounting, protocol_profile, run_created_at, created_at`

func (s *InterruptStore) List(ctx context.Context, sessionID string) ([]interrupts.Pending, error) {
	return s.list(ctx, sessionID, 0, "", 0)
}

// ListPage returns open interrupts oldest first, bounded by the query. after is
// the (open time, run id) position a previous page ended at; the pair is what
// makes the order total, since two runs can park in the same nanosecond.
func (s *InterruptStore) ListPage(ctx context.Context, sessionID string, afterCreatedAt int64, afterRunID string, limit int) ([]interrupts.Pending, error) {
	return s.list(ctx, sessionID, afterCreatedAt, afterRunID, limit)
}

func (s *InterruptStore) list(ctx context.Context, sessionID string, afterCreatedAt int64, afterRunID string, limit int) ([]interrupts.Pending, error) {
	query := `SELECT ` + interruptColumns + ` FROM interrupts`
	args := []any{}
	var conditions []string
	if sessionID != "" {
		conditions = append(conditions, `session_id = ?`)
		args = append(args, sessionID)
	}
	if afterCreatedAt > 0 || afterRunID != "" {
		conditions = append(conditions, `(created_at > ? OR (created_at = ? AND run_id > ?))`)
		args = append(args, afterCreatedAt, afterCreatedAt, afterRunID)
	}
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, ` AND `)
	}
	query += ` ORDER BY created_at, run_id`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

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

func (s *InterruptStore) Get(ctx context.Context, runID string) (interrupts.Pending, bool, error) {
	row := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT `+interruptColumns+` FROM interrupts WHERE run_id = ?`, runID)
	p, err := scanPending(row)
	if errors.Is(err, sql.ErrNoRows) {
		return interrupts.Pending{}, false, nil
	}
	if err != nil {
		return interrupts.Pending{}, false, err
	}
	return p, true, nil
}

// Consume atomically reads AND deletes the pending interrupt for runID (one
// DELETE ... RETURNING), or returns ok=false when none is recorded — the resume
// claim contract. A single statement means two concurrent resumes can't both
// observe the same open interrupt: one claims it, the other gets ok=false, so a
// non-idempotent tool never re-fires.
func (s *InterruptStore) Consume(ctx context.Context, runID string) (interrupts.Pending, bool, error) {
	row := conn(ctx, s.db).QueryRowContext(ctx,
		`DELETE FROM interrupts WHERE run_id = ?
		 RETURNING `+interruptColumns,
		runID)
	p, err := scanPending(row)
	if errors.Is(err, sql.ErrNoRows) {
		return interrupts.Pending{}, false, nil
	}
	if err != nil {
		return interrupts.Pending{}, false, err
	}
	return p, true, nil
}

func (s *InterruptStore) Delete(ctx context.Context, runID string) error {
	if _, err := conn(ctx, s.db).ExecContext(ctx,
		`DELETE FROM interrupts WHERE run_id = ?`, runID,
	); err != nil {
		return fmt.Errorf("sqlite: delete interrupt: %w", err)
	}
	return nil
}

// scanRow abstracts *sql.Row and *sql.Rows so one scan path serves Get +
// List.
func scanPending(row scanRow) (interrupts.Pending, error) {
	var (
		p            interrupts.Pending
		provider     string
		model        string
		payload      string
		drained      string
		accounting   string
		profile      string
		runCreatedNs int64
		createdNs    int64
	)
	if err := row.Scan(&p.RunID, &p.SessionID, &p.TurnID, &p.ProcessID, &provider, &model, &payload, &drained, &accounting, &profile, &runCreatedNs, &createdNs); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return interrupts.Pending{}, err
		}
		return interrupts.Pending{}, fmt.Errorf("sqlite: scan interrupt: %w", err)
	}
	selection, err := modelref.New(provider, model)
	if err != nil {
		return interrupts.Pending{}, fmt.Errorf("sqlite: decode interrupt model selection: %w", err)
	}
	p.ModelSelection = selection
	if p.Interrupts, err = decodeInterrupts(payload); err != nil {
		return interrupts.Pending{}, fmt.Errorf("sqlite: decode interrupts: %w", err)
	}
	if drained != "" {
		var rows []drainedToolRow
		if err := json.Unmarshal([]byte(drained), &rows); err != nil {
			return interrupts.Pending{}, fmt.Errorf("sqlite: decode drained tools: %w", err)
		}
		p.DrainedTools = drainedToolsFromRows(rows)
	}
	if p.ProtocolProfile, err = decodeRunProtocolProfile(profile); err != nil {
		return interrupts.Pending{}, err
	}
	if p.Metrics, p.Limits, err = decodeRunAccounting(accounting); err != nil {
		return interrupts.Pending{}, fmt.Errorf("sqlite: decode interrupt %q: %w", p.RunID, err)
	}
	p.RunCreatedAt = time.Unix(0, runCreatedNs).UTC()
	p.CreatedAt = time.Unix(0, createdNs).UTC()
	return p, nil
}

// decodeInterrupts reads the stored open-interrupt set. It is the one reader of
// that encoding: the Run table joins the same column to answer what a parked Run
// is waiting on, and a second decoder there could disagree about the format.
func decodeInterrupts(payload string) ([]transcript.Interrupt, error) {
	if payload == "" {
		return nil, nil
	}
	var out []transcript.Interrupt
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func drainedToolRows(tools []interrupts.DrainedTool) []drainedToolRow {
	rows := make([]drainedToolRow, len(tools))
	for index, tool := range tools {
		rows[index] = drainedToolRow{ItemID: tool.ItemID, CallID: tool.CallID, Name: tool.Name, Arguments: tool.Arguments}
	}
	return rows
}

func drainedToolsFromRows(rows []drainedToolRow) []interrupts.DrainedTool {
	tools := make([]interrupts.DrainedTool, len(rows))
	for index, row := range rows {
		tools[index] = interrupts.DrainedTool{ItemID: row.ItemID, CallID: row.CallID, Name: row.Name, Arguments: row.Arguments}
	}
	return tools
}
