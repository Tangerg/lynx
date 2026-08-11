package persistence

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/sqlite"
)

// InterruptStore translates the Application-owned waiting-tree hand-off to
// SQLite's technical record. The database adapter never imports Application or
// acquires ownership of resume semantics.
type InterruptStore struct {
	storage *sqlite.InterruptStore
}

func NewInterruptStore(storage *sqlite.InterruptStore) *InterruptStore {
	return &InterruptStore{storage: storage}
}

func (store *InterruptStore) Open(ctx context.Context, pending runs.Pending) error {
	if err := pending.Validate(); err != nil {
		return err
	}
	return store.storage.Open(ctx, interruptRecord(pending))
}

func (store *InterruptStore) List(ctx context.Context, sessionID string) ([]runs.Pending, error) {
	records, err := store.storage.List(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return pendingValues(records)
}

func (store *InterruptStore) ListPage(
	ctx context.Context,
	sessionID, rootRunID string,
	afterCreatedAt int64,
	afterRootRunID string,
	limit int,
) ([]runs.Pending, error) {
	records, err := store.storage.ListPage(
		ctx,
		sessionID,
		rootRunID,
		afterCreatedAt,
		afterRootRunID,
		limit,
	)
	if err != nil {
		return nil, err
	}
	return pendingValues(records)
}

func (store *InterruptStore) Get(ctx context.Context, rootRunID string) (runs.Pending, bool, error) {
	record, ok, err := store.storage.Get(ctx, rootRunID)
	if err != nil || !ok {
		return runs.Pending{}, ok, err
	}
	pending := pendingValue(record)
	if err := pending.Validate(); err != nil {
		return runs.Pending{}, false, err
	}
	return pending, true, nil
}

func (store *InterruptStore) Consume(ctx context.Context, sessionID, rootRunID string) (runs.Pending, bool, error) {
	record, ok, err := store.storage.Consume(ctx, sessionID, rootRunID)
	if err != nil || !ok {
		return runs.Pending{}, ok, err
	}
	pending := pendingValue(record)
	if err := pending.Validate(); err != nil {
		return runs.Pending{}, false, err
	}
	return pending, true, nil
}

func (store *InterruptStore) ClaimResume(
	ctx context.Context,
	sessionID, rootRunID string,
	answers []runs.InterruptAnswer,
	claimedAt time.Time,
) (runs.Pending, bool, error) {
	rows := make([]resumeAnswerRow, len(answers))
	for index, answer := range answers {
		rows[index] = resumeAnswerRow{
			InterruptItemID: answer.InterruptItemID,
			MemberID:        answer.MemberID,
			RequestID:       answer.RequestID,
			Approved:        answer.Resolution.Approved,
			Arguments:       answer.Resolution.Arguments,
			Answers:         answer.Resolution.Answers,
			Reason:          answer.Resolution.Reason,
			RememberScope:   string(answer.Resolution.RememberScope),
		}
	}
	encoded, err := json.Marshal(rows)
	if err != nil {
		return runs.Pending{}, false, err
	}
	record, found, err := store.storage.ClaimResume(ctx, sessionID, rootRunID, encoded, claimedAt)
	if err != nil || !found {
		return runs.Pending{}, found, err
	}
	pending := pendingValue(record)
	if err := pending.Validate(); err != nil {
		return runs.Pending{}, false, err
	}
	return pending, true, nil
}

func (store *InterruptStore) RequireResumeClaim(ctx context.Context, sessionID, rootRunID string) error {
	return store.storage.RequireResumeClaim(ctx, sessionID, rootRunID)
}

type resumeAnswerRow struct {
	InterruptItemID string     `json:"interruptItemId"`
	MemberID        string     `json:"memberId"`
	RequestID       string     `json:"requestId"`
	Approved        bool       `json:"approved"`
	Arguments       string     `json:"arguments,omitempty"`
	Answers         [][]string `json:"answers,omitempty"`
	Reason          string     `json:"reason,omitempty"`
	RememberScope   string     `json:"rememberScope,omitempty"`
}

func (store *InterruptStore) Delete(ctx context.Context, sessionID, rootRunID string) error {
	return store.storage.Delete(ctx, sessionID, rootRunID)
}

func pendingValues(records []sqlite.InterruptRecord) ([]runs.Pending, error) {
	values := make([]runs.Pending, len(records))
	for index, record := range records {
		values[index] = pendingValue(record)
		if err := values[index].Validate(); err != nil {
			return nil, err
		}
	}
	return values, nil
}

func interruptRecord(pending runs.Pending) sqlite.InterruptRecord {
	continuations := make([]sqlite.ContinuationRecord, len(pending.Continuations))
	for index, continuation := range pending.Continuations {
		drained := make([]sqlite.DrainedToolRecord, len(continuation.DrainedTools))
		for toolIndex, tool := range continuation.DrainedTools {
			drained[toolIndex] = sqlite.DrainedToolRecord{
				ItemID: tool.ItemID, ItemOccurredAt: tool.ItemOccurredAt,
				CallID: tool.CallID, SourceCallID: tool.SourceCallID,
				Name: tool.Name, Arguments: tool.Arguments,
			}
		}
		committed := make([]sqlite.CommittedToolRecord, len(continuation.CommittedTools))
		for toolIndex, tool := range continuation.CommittedTools {
			committed[toolIndex] = sqlite.CommittedToolRecord{
				ItemID: tool.ItemID, CallID: tool.CallID, SourceCallID: tool.SourceCallID, Name: tool.Name,
				Arguments: tool.Arguments, Failure: tool.Failure,
			}
		}
		continuations[index] = sqlite.ContinuationRecord{
			RunID: continuation.RunID, MemberID: continuation.MemberID,
			Lineage: continuation.Lineage, ModelSelection: continuation.ModelSelection,
			DrainedTools: drained, CommittedTools: committed,
			RunCreatedAt: continuation.RunCreatedAt,
			Metrics:      continuation.Metrics, Limits: continuation.Limits,
		}
	}
	bindings := make([]sqlite.InterruptBindingRecord, len(pending.Bindings))
	for index, binding := range pending.Bindings {
		bindings[index] = sqlite.InterruptBindingRecord{
			InterruptItemID: binding.InterruptItemID,
			MemberID:        binding.MemberID,
			RequestID:       binding.RequestID,
		}
	}
	return sqlite.InterruptRecord{
		RootRunID: pending.RootRunID, SessionID: pending.SessionID,
		ExecutorID: pending.ExecutorID, GoalLeaseID: pending.GoalLeaseID,
		Interrupts: pending.Interrupts, Bindings: bindings,
		Continuations: continuations, Capabilities: pending.Capabilities,
		CreatedAt: pending.CreatedAt,
	}
}

func pendingValue(record sqlite.InterruptRecord) runs.Pending {
	continuations := make([]runs.Continuation, len(record.Continuations))
	for index, continuation := range record.Continuations {
		var drained []runs.DrainedTool
		if len(continuation.DrainedTools) > 0 {
			drained = make([]runs.DrainedTool, len(continuation.DrainedTools))
		}
		for toolIndex, tool := range continuation.DrainedTools {
			drained[toolIndex] = runs.DrainedTool{
				ItemID: tool.ItemID, ItemOccurredAt: tool.ItemOccurredAt,
				CallID: tool.CallID, SourceCallID: tool.SourceCallID,
				Name: tool.Name, Arguments: tool.Arguments,
			}
		}
		var committed []runs.CommittedTool
		if len(continuation.CommittedTools) > 0 {
			committed = make([]runs.CommittedTool, len(continuation.CommittedTools))
		}
		for toolIndex, tool := range continuation.CommittedTools {
			committed[toolIndex] = runs.CommittedTool{
				ItemID: tool.ItemID, CallID: tool.CallID, SourceCallID: tool.SourceCallID, Name: tool.Name,
				Arguments: tool.Arguments, Failure: tool.Failure,
			}
		}
		continuations[index] = runs.Continuation{
			RunID: continuation.RunID, MemberID: continuation.MemberID,
			Lineage: continuation.Lineage, ModelSelection: continuation.ModelSelection,
			DrainedTools: drained, CommittedTools: committed,
			RunCreatedAt: continuation.RunCreatedAt,
			Metrics:      continuation.Metrics, Limits: continuation.Limits,
		}
	}
	bindings := make([]runs.InterruptBinding, len(record.Bindings))
	for index, binding := range record.Bindings {
		bindings[index] = runs.InterruptBinding{
			InterruptItemID: binding.InterruptItemID,
			MemberID:        binding.MemberID,
			RequestID:       binding.RequestID,
		}
	}
	return runs.Pending{
		RootRunID: record.RootRunID, SessionID: record.SessionID,
		ExecutorID: record.ExecutorID, GoalLeaseID: record.GoalLeaseID,
		Interrupts: record.Interrupts, Bindings: bindings,
		Continuations: continuations, Capabilities: record.Capabilities,
		CreatedAt: record.CreatedAt,
	}
}
