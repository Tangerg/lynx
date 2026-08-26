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
	RootRunID         string
	SessionID         string
	ExecutorID        string
	GoalIncarnationID string
	Interrupts        []transcript.Interrupt
	Bindings          []InterruptBindingRecord
	Continuations     []ContinuationRecord
	Capabilities      run.Capabilities
	CreatedAt         time.Time
}

// ContinuationRecord is the stored continuation row for one Run.
type ContinuationRecord struct {
	RunID          string
	MemberID       string
	Lineage        run.Lineage
	ModelSelection modelref.Selection
	DrainedTools   []DrainedToolRecord
	CommittedTools []CommittedToolRecord
	RunCreatedAt   time.Time
	Metrics        run.Metrics
	ContextTokens  int64
	Limits         run.Limits
}

// InterruptBindingRecord is the stored item-to-input-request correspondence.
type InterruptBindingRecord struct {
	InterruptItemID string
	MemberID        string
	RequestID       string
	ToolCallID      string
}

// DrainedToolRecord is the stored identity of an open tool invocation.
type DrainedToolRecord struct {
	ItemID         string
	ItemOccurredAt time.Time
	CallID         string
	SourceCallID   string
	Name           string
	Arguments      string
}

// CommittedToolRecord is the stored identity and outcome of a settled tool.
type CommittedToolRecord struct {
	ItemID       string
	CallID       string
	SourceCallID string
	Name         string
	Arguments    string
	Failure      tool.Failure
}

func (i InterruptRecord) rootContinuation() (ContinuationRecord, bool) {
	for _, continuation := range i.Continuations {
		if continuation.RunID == i.RootRunID {
			return continuation, true
		}
	}
	return ContinuationRecord{}, false
}

func (i InterruptRecord) validateStorageShape() error {
	switch {
	case strings.TrimSpace(i.RootRunID) == "" || i.RootRunID != strings.TrimSpace(i.RootRunID):
		return errors.New("root Run ID must be non-empty without surrounding whitespace")
	case strings.TrimSpace(i.SessionID) == "" || i.SessionID != strings.TrimSpace(i.SessionID):
		return errors.New("session ID must be non-empty without surrounding whitespace")
	case strings.TrimSpace(i.ExecutorID) == "" || i.ExecutorID != strings.TrimSpace(i.ExecutorID):
		return errors.New("executor ID must be non-empty without surrounding whitespace")
	case i.GoalIncarnationID != strings.TrimSpace(i.GoalIncarnationID):
		return errors.New("goal incarnation ID has surrounding whitespace")
	case i.CreatedAt.IsZero():
		return errors.New("creation time is required")
	case len(i.Interrupts) == 0:
		return errors.New("interrupt payload is required")
	case len(i.Continuations) == 0:
		return errors.New("continuation payload is required")
	case len(i.Bindings) != len(i.Interrupts):
		return errors.New("interrupt bindings do not match interrupts")
	}
	root, ok := i.rootContinuation()
	if !ok || strings.TrimSpace(root.MemberID) == "" {
		return errors.New("root continuation and member ID are required")
	}
	return nil
}

type drainedToolRow struct {
	ItemID         string `json:"itemId"`
	ItemOccurredAt int64  `json:"itemOccurredAt"`
	CallID         string `json:"callId"`
	SourceCallID   string `json:"sourceCallId,omitempty"`
	Name           string `json:"name"`
	Arguments      string `json:"arguments"`
}

type committedToolRow struct {
	ItemID       string             `json:"itemId"`
	CallID       string             `json:"callId"`
	SourceCallID string             `json:"sourceCallId,omitempty"`
	Name         string             `json:"name"`
	Arguments    string             `json:"arguments"`
	Failure      toolFailurePayload `json:"failure"`
}

type interruptPayload struct {
	ItemID         string           `json:"itemId"`
	ItemOccurredAt int64            `json:"itemOccurredAt"`
	RunID          string           `json:"runId"`
	Kind           interrupt.Kind   `json:"kind"`
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
	MemberID        string             `json:"memberId"`
	SpawnedByItemID string             `json:"spawnedByItemId,omitempty"`
	ParentRunID     string             `json:"parentRunId,omitempty"`
	RootRunID       string             `json:"rootRunId,omitempty"`
	Provider        string             `json:"provider,omitempty"`
	Model           string             `json:"model,omitempty"`
	DrainedTools    []drainedToolRow   `json:"drainedTools,omitempty"`
	CommittedTools  []committedToolRow `json:"committedTools,omitempty"`
	RunCreatedAt    int64              `json:"runCreatedAt"`
	ContextTokens   int64              `json:"contextTokens,omitempty"`
	Accounting      runAccountingRow   `json:"accounting"`
}

type interruptBindingRow struct {
	InterruptItemID string `json:"interruptItemId"`
	MemberID        string `json:"memberId"`
	RequestID       string `json:"requestId"`
	ToolCallID      string `json:"toolCallId,omitempty"`
}

// NewInterruptStore binds the SQLite interrupt registry to a database opened via
// [Open].
func NewInterruptStore(db *sql.DB) *InterruptStore {
	return &InterruptStore{db: db}
}

// Open records a newly reached barrier. An existing root Run or executor root
// is an identity conflict; a barrier is replaced only after its owner consumes
// the previous one in the same application transaction.
func (i *InterruptStore) Open(ctx context.Context, p InterruptRecord) error {
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
	bindings, err := json.Marshal(interruptBindingRows(p.Bindings))
	if err != nil {
		return fmt.Errorf("sqlite: encode interrupt bindings: %w", err)
	}
	capabilities, err := encodeRunCapabilities(p.Capabilities)
	if err != nil {
		return fmt.Errorf("sqlite: open interrupt: %w", err)
	}
	result, err := conn(ctx, i.db).ExecContext(ctx,
		`INSERT INTO interrupts(root_run_id, session_id, executor_id, goal_incarnation_id, root_member_id, payload, continuations, interrupt_bindings, capabilities, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(root_run_id) DO UPDATE SET
		   executor_id = excluded.executor_id,
		   goal_incarnation_id = excluded.goal_incarnation_id,
		   root_member_id = excluded.root_member_id,
		   payload = excluded.payload,
		   continuations = excluded.continuations,
		   interrupt_bindings = excluded.interrupt_bindings,
		   capabilities = excluded.capabilities,
		   created_at = excluded.created_at,
		   state = 'open', answers = '', claimed_at = 0
		 WHERE interrupts.state = 'resuming'
		   AND interrupts.session_id = excluded.session_id
		   AND interrupts.executor_id = excluded.executor_id
		   AND interrupts.root_member_id = excluded.root_member_id`,
		p.RootRunID,
		p.SessionID,
		p.ExecutorID,
		p.GoalIncarnationID,
		root.MemberID,
		string(payload),
		string(continuations),
		string(bindings),
		capabilities,
		p.CreatedAt.UnixNano(),
	)
	if isUniqueViolation(err) {
		return fmt.Errorf(
			"%w: Pending root Run %q or executor root %q is already claimed",
			transcript.ErrIdentityConflict,
			p.RootRunID,
			root.MemberID,
		)
	}
	if err != nil {
		return fmt.Errorf("sqlite: open interrupt: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect opened interrupt: %w", err)
	}
	if changed != 1 {
		return fmt.Errorf("%w: Pending root Run %q is already open", transcript.ErrIdentityConflict, p.RootRunID)
	}
	return nil
}

const interruptColumns = `root_run_id, session_id, executor_id, goal_incarnation_id, root_member_id, payload, continuations, interrupt_bindings, capabilities, created_at`

func (i *InterruptStore) List(ctx context.Context, sessionID string) ([]InterruptRecord, error) {
	return i.list(ctx, sessionID, "", 0, "", 0)
}

// ListPage returns open interrupts oldest first, bounded by the query. after is
// the (open time, run id) position a previous page ended at; the pair is what
// makes the order total, since two runs can park in the same nanosecond.
func (i *InterruptStore) ListPage(ctx context.Context, sessionID, rootRunID string, afterCreatedAt int64, afterRootRunID string, limit int) ([]InterruptRecord, error) {
	return i.list(ctx, sessionID, rootRunID, afterCreatedAt, afterRootRunID, limit)
}

func (i *InterruptStore) list(ctx context.Context, sessionID, rootRunID string, afterCreatedAt int64, afterRunID string, limit int) ([]InterruptRecord, error) {
	query := `SELECT ` + interruptColumns + ` FROM interrupts`
	args := []any{}
	conditions := []string{`state = 'open'`}
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

	rows, err := conn(ctx, i.db).QueryContext(ctx, query, args...)
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

func (i *InterruptStore) Get(ctx context.Context, runID string) (InterruptRecord, bool, error) {
	row := conn(ctx, i.db).QueryRowContext(ctx,
		`SELECT `+interruptColumns+` FROM interrupts WHERE root_run_id = ? AND state = 'open'`, runID)
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
func (i *InterruptStore) Consume(ctx context.Context, sessionID, runID string) (InterruptRecord, bool, error) {
	if err := validatePendingOwner(sessionID, runID); err != nil {
		return InterruptRecord{}, false, fmt.Errorf("sqlite: consume interrupt: %w", err)
	}
	row := conn(ctx, i.db).QueryRowContext(ctx,
		`DELETE FROM interrupts WHERE session_id = ? AND root_run_id = ? AND state = 'open'
		 RETURNING `+interruptColumns,
		sessionID, runID)
	p, err := scanPending(row)
	if errors.Is(err, sql.ErrNoRows) {
		if rejectForeignPendingOwnerErr := i.rejectForeignPendingOwner(ctx, sessionID, runID); rejectForeignPendingOwnerErr != nil {
			return InterruptRecord{}, false, rejectForeignPendingOwnerErr
		}
		return InterruptRecord{}, false, nil
	}
	if err != nil {
		return InterruptRecord{}, false, err
	}
	return p, true, nil
}

// ClaimResume atomically changes one exact open hand-off into a nonrecoverable
// resuming record while retaining the validated answer for audit and crash
// diagnosis. Open reads exclude the row until a new waiting boundary replaces
// it or terminal cleanup deletes it.
func (i *InterruptStore) ClaimResume(
	ctx context.Context,
	sessionID, runID string,
	answers json.RawMessage,
	claimedAt time.Time,
) (InterruptRecord, bool, error) {
	if err := validatePendingOwner(sessionID, runID); err != nil {
		return InterruptRecord{}, false, fmt.Errorf("sqlite: claim resume: %w", err)
	}
	if len(answers) == 0 || !json.Valid(answers) {
		return InterruptRecord{}, false, errors.New("sqlite: claim resume answers must be valid JSON")
	}
	if claimedAt.IsZero() {
		return InterruptRecord{}, false, errors.New("sqlite: claim resume time is required")
	}
	row := conn(ctx, i.db).QueryRowContext(ctx,
		`UPDATE interrupts
		    SET state = 'resuming', answers = ?, claimed_at = ?
		  WHERE session_id = ? AND root_run_id = ? AND state = 'open'
		  RETURNING `+interruptColumns,
		string(answers), claimedAt.UTC().UnixNano(), sessionID, runID,
	)
	record, err := scanPending(row)
	if errors.Is(err, sql.ErrNoRows) {
		if rejectForeignPendingOwnerErr := i.rejectForeignPendingOwner(ctx, sessionID, runID); rejectForeignPendingOwnerErr != nil {
			return InterruptRecord{}, false, rejectForeignPendingOwnerErr
		}
		return InterruptRecord{}, false, nil
	}
	if err != nil {
		return InterruptRecord{}, false, err
	}
	return record, true, nil
}

// RequireResumeClaim proves that the exact root hand-off crossed the answer
// claim linearization point before its Run tree is reopened.
func (i *InterruptStore) RequireResumeClaim(ctx context.Context, sessionID, runID string) error {
	if err := validatePendingOwner(sessionID, runID); err != nil {
		return fmt.Errorf("sqlite: require resume claim: %w", err)
	}
	var owner, state string
	err := conn(ctx, i.db).QueryRowContext(ctx,
		`SELECT session_id, state FROM interrupts WHERE root_run_id = ?`, runID,
	).Scan(&owner, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("sqlite: resume claim does not exist")
	}
	if err != nil {
		return fmt.Errorf("sqlite: inspect resume claim: %w", err)
	}
	if owner != sessionID {
		return fmt.Errorf(
			"%w: Pending root Run %q belongs to Session %q, not %q",
			transcript.ErrIdentityConflict,
			runID,
			owner,
			sessionID,
		)
	}
	if state != "resuming" {
		return fmt.Errorf("sqlite: interrupt for root Run %q is %q, not resuming", runID, state)
	}
	return nil
}

func (i *InterruptStore) Delete(ctx context.Context, sessionID, runID string) error {
	if err := validatePendingOwner(sessionID, runID); err != nil {
		return fmt.Errorf("sqlite: delete interrupt: %w", err)
	}
	result, err := conn(ctx, i.db).ExecContext(ctx,
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
	return i.rejectForeignPendingOwner(ctx, sessionID, runID)
}

// DeleteResumeClaim consumes only the answer claim owned by a failed Resume.
// It leaves ordinary open reads unchanged and cannot delete a replacement open
// barrier that reuses the same root Run identity.
func (i *InterruptStore) DeleteResumeClaim(
	ctx context.Context,
	sessionID, runID, rootMemberID string,
) error {
	if err := validatePendingOwner(sessionID, runID); err != nil {
		return fmt.Errorf("sqlite: delete Resume claim: %w", err)
	}
	if strings.TrimSpace(rootMemberID) == "" || rootMemberID != strings.TrimSpace(rootMemberID) {
		return errors.New("sqlite: delete Resume claim: root member ID must be non-empty without surrounding whitespace")
	}
	result, err := conn(ctx, i.db).ExecContext(ctx,
		`DELETE FROM interrupts
		  WHERE session_id = ? AND root_run_id = ? AND root_member_id = ? AND state = 'resuming'`,
		sessionID, runID, rootMemberID,
	)
	if err != nil {
		return fmt.Errorf("sqlite: delete Resume claim: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect deleted Resume claim: %w", err)
	}
	if deleted == 1 {
		return nil
	}
	if err := i.rejectForeignPendingOwner(ctx, sessionID, runID); err != nil {
		return err
	}
	return fmt.Errorf("sqlite: matching resuming interrupt for root Run %q was not found", runID)
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

func (i *InterruptStore) rejectForeignPendingOwner(ctx context.Context, sessionID, rootRunID string) error {
	var owner string
	err := conn(ctx, i.db).QueryRowContext(ctx,
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
		p               InterruptRecord
		payload         string
		rootMemberID    string
		continuations   string
		encodedBindings string
		capabilities    string
		createdNs       int64
	)
	if err := row.Scan(
		&p.RootRunID,
		&p.SessionID,
		&p.ExecutorID,
		&p.GoalIncarnationID,
		&rootMemberID,
		&payload,
		&continuations,
		&encodedBindings,
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
	if decodeInterruptJSONErr := decodeInterruptJSON(continuations, &continuationValues); decodeInterruptJSONErr != nil {
		return InterruptRecord{}, fmt.Errorf("sqlite: decode interrupt continuations: %w", decodeInterruptJSONErr)
	}
	if p.Continuations, err = continuationsFromRows(continuationValues); err != nil {
		return InterruptRecord{}, fmt.Errorf("sqlite: decode interrupt continuations: %w", err)
	}
	var bindingValues []interruptBindingRow
	if decodeInterruptJSONErr := decodeInterruptJSON(encodedBindings, &bindingValues); decodeInterruptJSONErr != nil {
		return InterruptRecord{}, fmt.Errorf("sqlite: decode input-request bindings: %w", decodeInterruptJSONErr)
	}
	p.Bindings = interruptBindingsFromRows(bindingValues)
	if p.Capabilities, err = decodeRunCapabilities(capabilities); err != nil {
		return InterruptRecord{}, err
	}
	p.CreatedAt = time.Unix(0, createdNs).UTC()
	if err := p.validateStorageShape(); err != nil {
		return InterruptRecord{}, fmt.Errorf("sqlite: decode interrupt %q: %w", p.RootRunID, err)
	}
	root, _ := p.rootContinuation()
	if root.MemberID != rootMemberID {
		return InterruptRecord{}, fmt.Errorf(
			"sqlite: decode interrupt %q: root member index %q does not match continuation %q",
			p.RootRunID,
			rootMemberID,
			root.MemberID,
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
			CallID: tool.CallID, SourceCallID: tool.SourceCallID,
			Name: tool.Name, Arguments: tool.Arguments,
		}
	}
	return rows
}

func drainedToolsFromRows(rows []drainedToolRow) []DrainedToolRecord {
	tools := make([]DrainedToolRecord, len(rows))
	for index, row := range rows {
		tools[index] = DrainedToolRecord{
			ItemID: row.ItemID, ItemOccurredAt: time.Unix(0, row.ItemOccurredAt).UTC(),
			CallID: row.CallID, SourceCallID: row.SourceCallID,
			Name: row.Name, Arguments: row.Arguments,
		}
	}
	return tools
}

func committedToolRows(tools []CommittedToolRecord) ([]committedToolRow, error) {
	rows := make([]committedToolRow, len(tools))
	for index, committed := range tools {
		failure, err := encodeToolFailurePayload(committed.Failure)
		if err != nil {
			return nil, fmt.Errorf("committed tool[%d] failure: %w", index, err)
		}
		rows[index] = committedToolRow{
			ItemID:       committed.ItemID,
			CallID:       committed.CallID,
			SourceCallID: committed.SourceCallID,
			Name:         committed.Name,
			Arguments:    committed.Arguments,
			Failure:      failure,
		}
	}
	return rows, nil
}

func committedToolsFromRows(rows []committedToolRow) ([]CommittedToolRecord, error) {
	tools := make([]CommittedToolRecord, len(rows))
	for index, row := range rows {
		failure, err := decodeToolFailurePayload(row.Failure)
		if err != nil {
			return nil, fmt.Errorf("committed tool[%d] failure: %w", index, err)
		}
		tools[index] = CommittedToolRecord{
			ItemID:       row.ItemID,
			CallID:       row.CallID,
			SourceCallID: row.SourceCallID,
			Name:         row.Name,
			Arguments:    row.Arguments,
			Failure:      failure,
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
			MemberID:        value.MemberID,
			SpawnedByItemID: value.Lineage.SpawnedByItemID,
			ParentRunID:     value.Lineage.ParentRunID,
			RootRunID:       value.Lineage.RootRunID,
			Provider:        value.ModelSelection.Provider(),
			Model:           value.ModelSelection.Model(),
			DrainedTools:    drainedToolRows(value.DrainedTools),
			CommittedTools:  committedTools,
			RunCreatedAt:    value.RunCreatedAt.UnixNano(),
			ContextTokens:   value.ContextTokens,
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
			RunID:    row.RunID,
			MemberID: row.MemberID,
			Lineage: run.Lineage{
				SpawnedByItemID: row.SpawnedByItemID,
				ParentRunID:     row.ParentRunID,
				RootRunID:       row.RootRunID,
			},
			ModelSelection: selection,
			DrainedTools:   drainedToolsFromRows(row.DrainedTools),
			CommittedTools: committedTools,
			RunCreatedAt:   time.Unix(0, row.RunCreatedAt).UTC(),
			Metrics:        metrics,
			ContextTokens:  row.ContextTokens,
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
			RunID: value.RunID, Kind: value.Kind,
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
			return nil, fmt.Errorf("interrupt[%d] has unknown kind %q", index, value.Kind)
		}
		rows[index] = row
	}
	return rows, nil
}

func interruptsFromPayloads(rows []interruptPayload) ([]transcript.Interrupt, error) {
	values := make([]transcript.Interrupt, len(rows))
	for index, row := range rows {
		if !row.Kind.Valid() {
			return nil, fmt.Errorf("interrupt[%d] has unknown kind %q", index, row.Kind)
		}
		value := transcript.Interrupt{
			ItemID: row.ItemID, ItemOccurredAt: time.Unix(0, row.ItemOccurredAt).UTC(),
			RunID: row.RunID, Kind: row.Kind,
		}
		switch row.Kind {
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

func interruptBindingRows(values []InterruptBindingRecord) []interruptBindingRow {
	rows := make([]interruptBindingRow, len(values))
	for index, value := range values {
		rows[index] = interruptBindingRow(value)
	}
	return rows
}

func interruptBindingsFromRows(rows []interruptBindingRow) []InterruptBindingRecord {
	values := make([]InterruptBindingRecord, len(rows))
	for index, row := range rows {
		values[index] = InterruptBindingRecord(row)
	}
	return values
}
