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

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// InterruptStore is the SQLite-backed registry of root-owned pending interrupt
// sets. The typed domain values are encoded through explicit adapter rows;
// protocol payloads and Go field names never define this storage shape.
type InterruptStore struct {
	db *sql.DB
}

// InterruptRecord is SQLite's technical representation of one durable
// waiting-tree hand-off. Application semantics belong to the persistence
// adapter; this record only names the values required by the storage codec.
type InterruptRecord struct {
	RootRunID     string
	SessionID     string
	ExecutorID    string
	GoalLeaseID   string
	Interrupts    []transcript.Interrupt
	Suspensions   []SuspensionBindingRecord
	Continuations []ContinuationRecord
	Capabilities  run.RunCapabilities
	CreatedAt     time.Time
}

// ContinuationRecord is the stored continuation row for one Run.
type ContinuationRecord struct {
	RunID          string
	ProcessID      string
	Lineage        run.RunLineage
	ModelSelection modelref.Selection
	DrainedTools   []DrainedToolRecord
	CommittedTools []CommittedToolRecord
	RunCreatedAt   time.Time
	Metrics        transcript.RunMetrics
	Limits         run.RunLimits
}

// SuspensionBindingRecord is the stored item-to-suspension correspondence.
type SuspensionBindingRecord struct {
	InterruptItemID string
	ProcessID       string
	SuspensionID    string
}

// DrainedToolRecord is the stored identity of an open tool invocation.
type DrainedToolRecord struct {
	ItemID         string
	ItemOccurredAt time.Time
	CallID         string
	Name           string
	Arguments      string
}

// CommittedToolRecord is the stored identity and outcome of a settled tool.
type CommittedToolRecord struct {
	ItemID    string
	CallID    string
	Name      string
	Arguments string
	Problem   transcript.Problem
}

func (record InterruptRecord) rootContinuation() (ContinuationRecord, bool) {
	for _, continuation := range record.Continuations {
		if continuation.RunID == record.RootRunID {
			return continuation, true
		}
	}
	return ContinuationRecord{}, false
}

func (record InterruptRecord) validateStorageShape() error {
	switch {
	case strings.TrimSpace(record.RootRunID) == "" || record.RootRunID != strings.TrimSpace(record.RootRunID):
		return errors.New("root Run ID must be non-empty without surrounding whitespace")
	case strings.TrimSpace(record.SessionID) == "" || record.SessionID != strings.TrimSpace(record.SessionID):
		return errors.New("session ID must be non-empty without surrounding whitespace")
	case strings.TrimSpace(record.ExecutorID) == "" || record.ExecutorID != strings.TrimSpace(record.ExecutorID):
		return errors.New("executor ID must be non-empty without surrounding whitespace")
	case record.GoalLeaseID != strings.TrimSpace(record.GoalLeaseID):
		return errors.New("goal lease ID has surrounding whitespace")
	case record.CreatedAt.IsZero():
		return errors.New("creation time is required")
	case len(record.Interrupts) == 0:
		return errors.New("interrupt payload is required")
	case len(record.Continuations) == 0:
		return errors.New("continuation payload is required")
	case len(record.Suspensions) != len(record.Interrupts):
		return errors.New("suspension bindings do not match interrupts")
	}
	root, ok := record.rootContinuation()
	if !ok || strings.TrimSpace(root.ProcessID) == "" {
		return errors.New("root continuation and process ID are required")
	}
	return nil
}

type drainedToolRow struct {
	ItemID         string `json:"itemId"`
	ItemOccurredAt int64  `json:"itemOccurredAt"`
	CallID         string `json:"callId"`
	Name           string `json:"name"`
	Arguments      string `json:"arguments"`
}

type committedToolRow struct {
	ItemID    string         `json:"itemId"`
	CallID    string         `json:"callId"`
	Name      string         `json:"name"`
	Arguments string         `json:"arguments"`
	Problem   problemPayload `json:"problem"`
}

type interruptPayload struct {
	ItemID         string           `json:"itemId"`
	ItemOccurredAt int64            `json:"itemOccurredAt"`
	RunID          string           `json:"runId"`
	Kind           string           `json:"kind"`
	Approval       *approvalPayload `json:"approval,omitempty"`
	Question       *questionPayload `json:"question,omitempty"`
}

type approvalPayload struct {
	Tool         toolInvocationPayload `json:"tool"`
	Risk         string                `json:"risk,omitempty"`
	Reason       string                `json:"reason,omitempty"`
	Rememberable bool                  `json:"rememberable,omitempty"`
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
func (s *InterruptStore) Open(ctx context.Context, p InterruptRecord) error {
	if err := p.validateStorageShape(); err != nil {
		return fmt.Errorf("sqlite: open interrupt: %w", err)
	}
	root, _ := p.rootContinuation()
	interrupts, err := interruptPayloads(p.Interrupts)
	if err != nil {
		return fmt.Errorf("sqlite: encode interrupts: %w", err)
	}
	payload, err := json.Marshal(interrupts)
	if err != nil {
		return fmt.Errorf("sqlite: encode interrupts: %w", err)
	}
	continuationValues, err := continuationRows(p.Continuations)
	if err != nil {
		return fmt.Errorf("sqlite: encode interrupt continuations: %w", err)
	}
	continuations, err := json.Marshal(continuationValues)
	if err != nil {
		return fmt.Errorf("sqlite: encode interrupt continuations: %w", err)
	}
	suspensions, err := json.Marshal(suspensionBindingRows(p.Suspensions))
	if err != nil {
		return fmt.Errorf("sqlite: encode interrupt suspension bindings: %w", err)
	}
	capabilities, err := encodeRunCapabilities(p.Capabilities)
	if err != nil {
		return fmt.Errorf("sqlite: open interrupt: %w", err)
	}
	_, err = conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO interrupts(root_run_id, session_id, executor_id, goal_lease_id, root_process_id, payload, continuations, suspension_bindings, capabilities, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.RootRunID,
		p.SessionID,
		p.ExecutorID,
		p.GoalLeaseID,
		root.ProcessID,
		string(payload),
		string(continuations),
		string(suspensions),
		capabilities,
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

const interruptColumns = `root_run_id, session_id, executor_id, goal_lease_id, root_process_id, payload, continuations, suspension_bindings, capabilities, created_at`

func (s *InterruptStore) List(ctx context.Context, sessionID string) ([]InterruptRecord, error) {
	return s.list(ctx, sessionID, "", 0, "", 0)
}

// ListPage returns open interrupts oldest first, bounded by the query. after is
// the (open time, run id) position a previous page ended at; the pair is what
// makes the order total, since two runs can park in the same nanosecond.
func (s *InterruptStore) ListPage(ctx context.Context, sessionID, rootRunID string, afterCreatedAt int64, afterRootRunID string, limit int) ([]InterruptRecord, error) {
	return s.list(ctx, sessionID, rootRunID, afterCreatedAt, afterRootRunID, limit)
}

func (s *InterruptStore) list(ctx context.Context, sessionID, rootRunID string, afterCreatedAt int64, afterRunID string, limit int) ([]InterruptRecord, error) {
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

	out := make([]InterruptRecord, 0)
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

func (s *InterruptStore) Get(ctx context.Context, runID string) (InterruptRecord, bool, error) {
	row := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT `+interruptColumns+` FROM interrupts WHERE root_run_id = ?`, runID)
	p, err := scanPending(row)
	if errors.Is(err, sql.ErrNoRows) {
		return InterruptRecord{}, false, nil
	}
	if err != nil {
		return InterruptRecord{}, false, err
	}
	return p, true, nil
}

// Consume atomically reads AND deletes the pending interrupt for runID (one
// DELETE ... RETURNING), or returns ok=false when none is recorded — the resume
// claim contract. A single statement means two concurrent resumes can't both
// observe the same open interrupt: one claims it, the other gets ok=false, so a
// non-idempotent tool never re-fires.
func (s *InterruptStore) Consume(ctx context.Context, sessionID, runID string) (InterruptRecord, bool, error) {
	if err := validatePendingOwner(sessionID, runID); err != nil {
		return InterruptRecord{}, false, fmt.Errorf("sqlite: consume interrupt: %w", err)
	}
	row := conn(ctx, s.db).QueryRowContext(ctx,
		`DELETE FROM interrupts WHERE session_id = ? AND root_run_id = ?
		 RETURNING `+interruptColumns,
		sessionID, runID)
	p, err := scanPending(row)
	if errors.Is(err, sql.ErrNoRows) {
		if err := s.rejectForeignPendingOwner(ctx, sessionID, runID); err != nil {
			return InterruptRecord{}, false, err
		}
		return InterruptRecord{}, false, nil
	}
	if err != nil {
		return InterruptRecord{}, false, err
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
func scanPending(row scanRow) (InterruptRecord, error) {
	var (
		p             InterruptRecord
		payload       string
		rootProcessID string
		continuations string
		suspensions   string
		capabilities  string
		createdNs     int64
	)
	if err := row.Scan(
		&p.RootRunID,
		&p.SessionID,
		&p.ExecutorID,
		&p.GoalLeaseID,
		&rootProcessID,
		&payload,
		&continuations,
		&suspensions,
		&capabilities,
		&createdNs,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return InterruptRecord{}, err
		}
		return InterruptRecord{}, fmt.Errorf("sqlite: scan interrupt: %w", err)
	}
	var err error
	if p.Interrupts, err = decodeInterrupts(payload); err != nil {
		return InterruptRecord{}, fmt.Errorf("sqlite: decode interrupts: %w", err)
	}
	var continuationValues []continuationRow
	if err := decodeInterruptJSON(continuations, &continuationValues); err != nil {
		return InterruptRecord{}, fmt.Errorf("sqlite: decode interrupt continuations: %w", err)
	}
	if p.Continuations, err = continuationsFromRows(continuationValues); err != nil {
		return InterruptRecord{}, fmt.Errorf("sqlite: decode interrupt continuations: %w", err)
	}
	var bindingValues []suspensionBindingRow
	if err := decodeInterruptJSON(suspensions, &bindingValues); err != nil {
		return InterruptRecord{}, fmt.Errorf("sqlite: decode interrupt suspension bindings: %w", err)
	}
	p.Suspensions = suspensionBindingsFromRows(bindingValues)
	if p.Capabilities, err = decodeRunCapabilities(capabilities); err != nil {
		return InterruptRecord{}, err
	}
	p.CreatedAt = time.Unix(0, createdNs).UTC()
	if err := p.validateStorageShape(); err != nil {
		return InterruptRecord{}, fmt.Errorf("sqlite: decode interrupt %q: %w", p.RootRunID, err)
	}
	root, _ := p.rootContinuation()
	if root.ProcessID != rootProcessID {
		return InterruptRecord{}, fmt.Errorf(
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
	var rows []interruptPayload
	if err := decodeInterruptJSON(payload, &rows); err != nil {
		return nil, err
	}
	return interruptsFromPayloads(rows)
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

func drainedToolRows(tools []DrainedToolRecord) []drainedToolRow {
	rows := make([]drainedToolRow, len(tools))
	for index, tool := range tools {
		rows[index] = drainedToolRow{
			ItemID: tool.ItemID, ItemOccurredAt: tool.ItemOccurredAt.UnixNano(),
			CallID: tool.CallID, Name: tool.Name, Arguments: tool.Arguments,
		}
	}
	return rows
}

func drainedToolsFromRows(rows []drainedToolRow) []DrainedToolRecord {
	tools := make([]DrainedToolRecord, len(rows))
	for index, row := range rows {
		tools[index] = DrainedToolRecord{
			ItemID: row.ItemID, ItemOccurredAt: time.Unix(0, row.ItemOccurredAt).UTC(),
			CallID: row.CallID, Name: row.Name, Arguments: row.Arguments,
		}
	}
	return tools
}

func committedToolRows(tools []CommittedToolRecord) ([]committedToolRow, error) {
	rows := make([]committedToolRow, len(tools))
	for index, committed := range tools {
		problem, err := encodeProblemPayload(committed.Problem)
		if err != nil {
			return nil, fmt.Errorf("committed tool[%d] problem: %w", index, err)
		}
		rows[index] = committedToolRow{
			ItemID:    committed.ItemID,
			CallID:    committed.CallID,
			Name:      committed.Name,
			Arguments: committed.Arguments,
			Problem:   problem,
		}
	}
	return rows, nil
}

func committedToolsFromRows(rows []committedToolRow) ([]CommittedToolRecord, error) {
	tools := make([]CommittedToolRecord, len(rows))
	for index, row := range rows {
		problem, err := decodeProblemPayload(row.Problem)
		if err != nil {
			return nil, fmt.Errorf("committed tool[%d] problem: %w", index, err)
		}
		tools[index] = CommittedToolRecord{
			ItemID:    row.ItemID,
			CallID:    row.CallID,
			Name:      row.Name,
			Arguments: row.Arguments,
			Problem:   problem,
		}
	}
	return tools, nil
}

func continuationRows(values []ContinuationRecord) ([]continuationRow, error) {
	rows := make([]continuationRow, len(values))
	for index, value := range values {
		committedTools, err := committedToolRows(value.CommittedTools)
		if err != nil {
			return nil, fmt.Errorf("continuation[%d]: %w", index, err)
		}
		rows[index] = continuationRow{
			RunID:           value.RunID,
			ProcessID:       value.ProcessID,
			SpawnedByItemID: value.Lineage.SpawnedByItemID,
			ParentRunID:     value.Lineage.ParentRunID,
			RootRunID:       value.Lineage.RootRunID,
			Provider:        value.ModelSelection.Provider(),
			Model:           value.ModelSelection.Model(),
			DrainedTools:    drainedToolRows(value.DrainedTools),
			CommittedTools:  committedTools,
			RunCreatedAt:    value.RunCreatedAt.UnixNano(),
			Accounting:      runAccountingRowOf(value.Metrics, value.Limits),
		}
	}
	return rows, nil
}

func continuationsFromRows(rows []continuationRow) ([]ContinuationRecord, error) {
	values := make([]ContinuationRecord, len(rows))
	for index, row := range rows {
		selection, err := modelref.New(row.Provider, row.Model)
		if err != nil {
			return nil, fmt.Errorf("continuation[%d] model selection: %w", index, err)
		}
		metrics, limits, err := row.Accounting.values()
		if err != nil {
			return nil, fmt.Errorf("continuation[%d] accounting: %w", index, err)
		}
		committedTools, err := committedToolsFromRows(row.CommittedTools)
		if err != nil {
			return nil, fmt.Errorf("continuation[%d]: %w", index, err)
		}
		values[index] = ContinuationRecord{
			RunID:     row.RunID,
			ProcessID: row.ProcessID,
			Lineage: run.RunLineage{
				SpawnedByItemID: row.SpawnedByItemID,
				ParentRunID:     row.ParentRunID,
				RootRunID:       row.RootRunID,
			},
			ModelSelection: selection,
			DrainedTools:   drainedToolsFromRows(row.DrainedTools),
			CommittedTools: committedTools,
			RunCreatedAt:   time.Unix(0, row.RunCreatedAt).UTC(),
			Metrics:        metrics,
			Limits:         limits,
		}
	}
	return values, nil
}

func interruptPayloads(values []transcript.Interrupt) ([]interruptPayload, error) {
	rows := make([]interruptPayload, len(values))
	for index, value := range values {
		row := interruptPayload{
			ItemID: value.ItemID, ItemOccurredAt: value.ItemOccurredAt.UnixNano(),
			RunID: value.RunID, Kind: value.Kind.String(),
		}
		switch value.Kind {
		case interrupt.Approval:
			if value.Approval == nil {
				return nil, fmt.Errorf("interrupt[%d] approval payload is missing", index)
			}
			row.Approval = &approvalPayload{
				Tool: encodeToolInvocationPayload(value.Approval.Tool),
				Risk: string(value.Approval.Risk), Reason: value.Approval.Reason,
				Rememberable: value.Approval.Rememberable,
			}
		case interrupt.Question:
			if value.Question == nil {
				return nil, fmt.Errorf("interrupt[%d] question payload is missing", index)
			}
			question, err := encodeQuestionPayload(*value.Question)
			if err != nil {
				return nil, fmt.Errorf("interrupt[%d]: %w", index, err)
			}
			row.Question = &question
		default:
			return nil, fmt.Errorf("interrupt[%d] has unknown kind %d", index, value.Kind)
		}
		rows[index] = row
	}
	return rows, nil
}

func interruptsFromPayloads(rows []interruptPayload) ([]transcript.Interrupt, error) {
	values := make([]transcript.Interrupt, len(rows))
	for index, row := range rows {
		kind, ok := interrupt.ParseKind(row.Kind)
		if !ok {
			return nil, fmt.Errorf("interrupt[%d] has unknown kind %q", index, row.Kind)
		}
		value := transcript.Interrupt{
			ItemID: row.ItemID, ItemOccurredAt: time.Unix(0, row.ItemOccurredAt).UTC(),
			RunID: row.RunID, Kind: kind,
		}
		switch kind {
		case interrupt.Approval:
			if row.Approval == nil || row.Question != nil {
				return nil, fmt.Errorf("interrupt[%d] approval payload is invalid", index)
			}
			invocation, err := decodeToolInvocationPayload(row.Approval.Tool)
			if err != nil {
				return nil, fmt.Errorf("interrupt[%d] approval tool: %w", index, err)
			}
			value.Approval = &transcript.Approval{
				Tool: invocation, Risk: tool.RiskLevel(row.Approval.Risk),
				Reason: row.Approval.Reason, Rememberable: row.Approval.Rememberable,
			}
		case interrupt.Question:
			if row.Question == nil || row.Approval != nil {
				return nil, fmt.Errorf("interrupt[%d] question payload is invalid", index)
			}
			question, err := decodeQuestionPayload(*row.Question)
			if err != nil {
				return nil, fmt.Errorf("interrupt[%d]: %w", index, err)
			}
			value.Question = &question
		}
		values[index] = value
	}
	return values, nil
}

func suspensionBindingRows(values []SuspensionBindingRecord) []suspensionBindingRow {
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

func suspensionBindingsFromRows(rows []suspensionBindingRow) []SuspensionBindingRecord {
	values := make([]SuspensionBindingRecord, len(rows))
	for index, row := range rows {
		values[index] = SuspensionBindingRecord{
			InterruptItemID: row.InterruptItemID,
			ProcessID:       row.ProcessID,
			SuspensionID:    row.SuspensionID,
		}
	}
	return values
}
