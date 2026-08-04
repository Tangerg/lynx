package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// InterruptStore is the SQLite-backed registry of root-owned pending interrupt
// sets. The typed domain values are encoded through explicit adapter rows;
// protocol payloads and Go field names never define this storage shape.
type InterruptStore struct {
	db *sql.DB
}

type drainedToolRow struct {
	ItemID         string `json:"itemId"`
	ItemOccurredAt int64  `json:"itemOccurredAt"`
	CallID         string `json:"callId"`
	Name           string `json:"name"`
	Arguments      string `json:"arguments"`
}

type committedToolRow struct {
	ItemID    string             `json:"itemId"`
	CallID    string             `json:"callId"`
	Name      string             `json:"name"`
	Arguments string             `json:"arguments"`
	Problem   transcript.Problem `json:"problem"`
}

type continuationRow struct {
	RunID           string             `json:"runId"`
	ProcessID       string             `json:"processId"`
	SpawnedByItemID string             `json:"spawnedByItemId,omitempty"`
	ParentRunID     string             `json:"parentRunId,omitempty"`
	RootRunID       string             `json:"rootRunId,omitempty"`
	Provider        string             `json:"provider,omitempty"`
	Model           string             `json:"model,omitempty"`
	DrainedTools    []drainedToolRow   `json:"drainedTools,omitempty"`
	CommittedTools  []committedToolRow `json:"committedTools,omitempty"`
	RunCreatedAt    int64              `json:"runCreatedAt"`
	Accounting      runAccountingRow   `json:"accounting"`
}

type suspensionBindingRow struct {
	InterruptItemID string `json:"interruptItemId"`
	ProcessID       string `json:"processId"`
	SuspensionID    string `json:"suspensionId"`
}

// NewInterruptStore binds the SQLite interrupt registry to a database opened via
// [Open].
func NewInterruptStore(db *sql.DB) *InterruptStore {
	return &InterruptStore{db: db}
}

// Open records a newly reached barrier. An existing root Run or executor root
// is an identity conflict; a barrier is replaced only after its owner consumes
// the previous one in the same application transaction.
func (s *InterruptStore) Open(ctx context.Context, p interrupts.Pending) error {
	if err := p.Validate(); err != nil {
		return fmt.Errorf("sqlite: open interrupt: %w", err)
	}
	root, _ := p.RootContinuation()
	payload, err := json.Marshal(p.Interrupts)
	if err != nil {
		return fmt.Errorf("sqlite: encode interrupts: %w", err)
	}
	continuations, err := json.Marshal(continuationRows(p.Continuations))
	if err != nil {
		return fmt.Errorf("sqlite: encode interrupt continuations: %w", err)
	}
	suspensions, err := json.Marshal(suspensionBindingRows(p.Suspensions))
	if err != nil {
		return fmt.Errorf("sqlite: encode interrupt suspension bindings: %w", err)
	}
	profile, err := encodeRunProtocolProfile(p.ProtocolProfile)
	if err != nil {
		return fmt.Errorf("sqlite: open interrupt: %w", err)
	}
	_, err = conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO interrupts(root_run_id, session_id, turn_id, goal_lease_id, root_process_id, payload, continuations, suspension_bindings, protocol_profile, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.RootRunID,
		p.SessionID,
		p.TurnID,
		p.GoalLeaseID,
		root.ProcessID,
		string(payload),
		string(continuations),
		string(suspensions),
		profile,
		p.CreatedAt.UnixNano(),
	)
	if isUniqueViolation(err) {
		return fmt.Errorf(
			"%w: Pending root Run %q or executor root %q is already claimed",
			transcript.ErrIdentityConflict,
			p.RootRunID,
			root.ProcessID,
		)
	}
	if err != nil {
		return fmt.Errorf("sqlite: open interrupt: %w", err)
	}
	return nil
}

const interruptColumns = `root_run_id, session_id, turn_id, goal_lease_id, root_process_id, payload, continuations, suspension_bindings, protocol_profile, created_at`

func (s *InterruptStore) List(ctx context.Context, sessionID string) ([]interrupts.Pending, error) {
	return s.list(ctx, sessionID, "", 0, "", 0)
}

// ListPage returns open interrupts oldest first, bounded by the query. after is
// the (open time, run id) position a previous page ended at; the pair is what
// makes the order total, since two runs can park in the same nanosecond.
func (s *InterruptStore) ListPage(ctx context.Context, sessionID, rootRunID string, afterCreatedAt int64, afterRootRunID string, limit int) ([]interrupts.Pending, error) {
	return s.list(ctx, sessionID, rootRunID, afterCreatedAt, afterRootRunID, limit)
}

func (s *InterruptStore) list(ctx context.Context, sessionID, rootRunID string, afterCreatedAt int64, afterRunID string, limit int) ([]interrupts.Pending, error) {
	query := `SELECT ` + interruptColumns + ` FROM interrupts`
	args := []any{}
	var conditions []string
	if sessionID != "" {
		conditions = append(conditions, `session_id = ?`)
		args = append(args, sessionID)
	}
	if rootRunID != "" {
		conditions = append(conditions, `root_run_id = ?`)
		args = append(args, rootRunID)
	}
	if afterCreatedAt > 0 || afterRunID != "" {
		conditions = append(conditions, `(created_at > ? OR (created_at = ? AND root_run_id > ?))`)
		args = append(args, afterCreatedAt, afterCreatedAt, afterRunID)
	}
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, ` AND `)
	}
	query += ` ORDER BY created_at, root_run_id`
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
		`SELECT `+interruptColumns+` FROM interrupts WHERE root_run_id = ?`, runID)
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
func (s *InterruptStore) Consume(ctx context.Context, sessionID, runID string) (interrupts.Pending, bool, error) {
	if err := validatePendingOwner(sessionID, runID); err != nil {
		return interrupts.Pending{}, false, fmt.Errorf("sqlite: consume interrupt: %w", err)
	}
	row := conn(ctx, s.db).QueryRowContext(ctx,
		`DELETE FROM interrupts WHERE session_id = ? AND root_run_id = ?
		 RETURNING `+interruptColumns,
		sessionID, runID)
	p, err := scanPending(row)
	if errors.Is(err, sql.ErrNoRows) {
		if err := s.rejectForeignPendingOwner(ctx, sessionID, runID); err != nil {
			return interrupts.Pending{}, false, err
		}
		return interrupts.Pending{}, false, nil
	}
	if err != nil {
		return interrupts.Pending{}, false, err
	}
	return p, true, nil
}

func (s *InterruptStore) Delete(ctx context.Context, sessionID, runID string) error {
	if err := validatePendingOwner(sessionID, runID); err != nil {
		return fmt.Errorf("sqlite: delete interrupt: %w", err)
	}
	result, err := conn(ctx, s.db).ExecContext(ctx,
		`DELETE FROM interrupts WHERE session_id = ? AND root_run_id = ?`, sessionID, runID,
	)
	if err != nil {
		return fmt.Errorf("sqlite: delete interrupt: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect deleted interrupt: %w", err)
	}
	if deleted == 1 {
		return nil
	}
	return s.rejectForeignPendingOwner(ctx, sessionID, runID)
}

func validatePendingOwner(sessionID, rootRunID string) error {
	if strings.TrimSpace(sessionID) == "" || sessionID != strings.TrimSpace(sessionID) {
		return errors.New("session ID must be non-empty without surrounding whitespace")
	}
	if strings.TrimSpace(rootRunID) == "" || rootRunID != strings.TrimSpace(rootRunID) {
		return errors.New("root Run ID must be non-empty without surrounding whitespace")
	}
	return nil
}

func (s *InterruptStore) rejectForeignPendingOwner(ctx context.Context, sessionID, rootRunID string) error {
	var owner string
	err := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT session_id FROM interrupts WHERE root_run_id = ?`,
		rootRunID,
	).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("sqlite: inspect interrupt %q owner: %w", rootRunID, err)
	}
	return fmt.Errorf(
		"%w: Pending root Run %q belongs to Session %q, not %q",
		transcript.ErrIdentityConflict,
		rootRunID,
		owner,
		sessionID,
	)
}

// scanRow abstracts *sql.Row and *sql.Rows so one scan path serves Get +
// List.
func scanPending(row scanRow) (interrupts.Pending, error) {
	var (
		p             interrupts.Pending
		payload       string
		rootProcessID string
		continuations string
		suspensions   string
		profile       string
		createdNs     int64
	)
	if err := row.Scan(
		&p.RootRunID,
		&p.SessionID,
		&p.TurnID,
		&p.GoalLeaseID,
		&rootProcessID,
		&payload,
		&continuations,
		&suspensions,
		&profile,
		&createdNs,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return interrupts.Pending{}, err
		}
		return interrupts.Pending{}, fmt.Errorf("sqlite: scan interrupt: %w", err)
	}
	var err error
	if p.Interrupts, err = decodeInterrupts(payload); err != nil {
		return interrupts.Pending{}, fmt.Errorf("sqlite: decode interrupts: %w", err)
	}
	var continuationValues []continuationRow
	if err := decodeInterruptJSON(continuations, &continuationValues); err != nil {
		return interrupts.Pending{}, fmt.Errorf("sqlite: decode interrupt continuations: %w", err)
	}
	if p.Continuations, err = continuationsFromRows(continuationValues); err != nil {
		return interrupts.Pending{}, fmt.Errorf("sqlite: decode interrupt continuations: %w", err)
	}
	var bindingValues []suspensionBindingRow
	if err := decodeInterruptJSON(suspensions, &bindingValues); err != nil {
		return interrupts.Pending{}, fmt.Errorf("sqlite: decode interrupt suspension bindings: %w", err)
	}
	p.Suspensions = suspensionBindingsFromRows(bindingValues)
	if p.ProtocolProfile, err = decodeRunProtocolProfile(profile); err != nil {
		return interrupts.Pending{}, err
	}
	p.CreatedAt = time.Unix(0, createdNs).UTC()
	if err := p.Validate(); err != nil {
		return interrupts.Pending{}, fmt.Errorf("sqlite: decode interrupt %q: %w", p.RootRunID, err)
	}
	root, _ := p.RootContinuation()
	if root.ProcessID != rootProcessID {
		return interrupts.Pending{}, fmt.Errorf(
			"sqlite: decode interrupt %q: root process index %q does not match continuation %q",
			p.RootRunID,
			rootProcessID,
			root.ProcessID,
		)
	}
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
	if err := decodeInterruptJSON(payload, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func decodeInterruptJSON(encoded string, target any) error {
	decoder := json.NewDecoder(strings.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("stored interrupt JSON has a trailing value")
		}
		return fmt.Errorf("stored interrupt JSON trailing value: %w", err)
	}
	return nil
}

func drainedToolRows(tools []interrupts.DrainedTool) []drainedToolRow {
	rows := make([]drainedToolRow, len(tools))
	for index, tool := range tools {
		rows[index] = drainedToolRow{
			ItemID: tool.ItemID, ItemOccurredAt: tool.ItemOccurredAt.UnixNano(),
			CallID: tool.CallID, Name: tool.Name, Arguments: tool.Arguments,
		}
	}
	return rows
}

func drainedToolsFromRows(rows []drainedToolRow) []interrupts.DrainedTool {
	tools := make([]interrupts.DrainedTool, len(rows))
	for index, row := range rows {
		tools[index] = interrupts.DrainedTool{
			ItemID: row.ItemID, ItemOccurredAt: time.Unix(0, row.ItemOccurredAt).UTC(),
			CallID: row.CallID, Name: row.Name, Arguments: row.Arguments,
		}
	}
	return tools
}

func committedToolRows(tools []interrupts.CommittedTool) []committedToolRow {
	rows := make([]committedToolRow, len(tools))
	for index, tool := range tools {
		rows[index] = committedToolRow{
			ItemID:    tool.ItemID,
			CallID:    tool.CallID,
			Name:      tool.Name,
			Arguments: tool.Arguments,
			Problem:   tool.Problem,
		}
	}
	return rows
}

func committedToolsFromRows(rows []committedToolRow) []interrupts.CommittedTool {
	tools := make([]interrupts.CommittedTool, len(rows))
	for index, row := range rows {
		tools[index] = interrupts.CommittedTool{
			ItemID:    row.ItemID,
			CallID:    row.CallID,
			Name:      row.Name,
			Arguments: row.Arguments,
			Problem:   row.Problem,
		}
	}
	return tools
}

func continuationRows(values []interrupts.Continuation) []continuationRow {
	rows := make([]continuationRow, len(values))
	for index, value := range values {
		rows[index] = continuationRow{
			RunID:           value.RunID,
			ProcessID:       value.ProcessID,
			SpawnedByItemID: value.Lineage.SpawnedByItemID,
			ParentRunID:     value.Lineage.ParentRunID,
			RootRunID:       value.Lineage.RootRunID,
			Provider:        value.ModelSelection.Provider(),
			Model:           value.ModelSelection.Model(),
			DrainedTools:    drainedToolRows(value.DrainedTools),
			CommittedTools:  committedToolRows(value.CommittedTools),
			RunCreatedAt:    value.RunCreatedAt.UnixNano(),
			Accounting:      runAccountingRowOf(value.Metrics, value.Limits),
		}
	}
	return rows
}

func continuationsFromRows(rows []continuationRow) ([]interrupts.Continuation, error) {
	values := make([]interrupts.Continuation, len(rows))
	for index, row := range rows {
		selection, err := modelref.New(row.Provider, row.Model)
		if err != nil {
			return nil, fmt.Errorf("continuation[%d] model selection: %w", index, err)
		}
		metrics, limits, err := row.Accounting.values()
		if err != nil {
			return nil, fmt.Errorf("continuation[%d] accounting: %w", index, err)
		}
		values[index] = interrupts.Continuation{
			RunID:     row.RunID,
			ProcessID: row.ProcessID,
			Lineage: execution.RunLineage{
				SpawnedByItemID: row.SpawnedByItemID,
				ParentRunID:     row.ParentRunID,
				RootRunID:       row.RootRunID,
			},
			ModelSelection: selection,
			DrainedTools:   drainedToolsFromRows(row.DrainedTools),
			CommittedTools: committedToolsFromRows(row.CommittedTools),
			RunCreatedAt:   time.Unix(0, row.RunCreatedAt).UTC(),
			Metrics:        metrics,
			Limits:         limits,
		}
	}
	return values, nil
}

func suspensionBindingRows(values []interrupts.SuspensionBinding) []suspensionBindingRow {
	rows := make([]suspensionBindingRow, len(values))
	for index, value := range values {
		rows[index] = suspensionBindingRow{
			InterruptItemID: value.InterruptItemID,
			ProcessID:       value.ProcessID,
			SuspensionID:    value.SuspensionID,
		}
	}
	return rows
}

func suspensionBindingsFromRows(rows []suspensionBindingRow) []interrupts.SuspensionBinding {
	values := make([]interrupts.SuspensionBinding, len(rows))
	for index, row := range rows {
		values[index] = interrupts.SuspensionBinding{
			InterruptItemID: row.InterruptItemID,
			ProcessID:       row.ProcessID,
			SuspensionID:    row.SuspensionID,
		}
	}
	return values
}
