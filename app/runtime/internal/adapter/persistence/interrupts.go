package persistence

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
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
				CallID: tool.CallID, Name: tool.Name, Arguments: tool.Arguments,
			}
		}
		committed := make([]sqlite.CommittedToolRecord, len(continuation.CommittedTools))
		for toolIndex, tool := range continuation.CommittedTools {
			committed[toolIndex] = sqlite.CommittedToolRecord{
				ItemID: tool.ItemID, CallID: tool.CallID, Name: tool.Name,
				Arguments: tool.Arguments, Problem: tool.Problem,
			}
		}
		continuations[index] = sqlite.ContinuationRecord{
			RunID: continuation.RunID, ProcessID: continuation.ProcessID,
			Lineage: continuation.Lineage, ModelSelection: continuation.ModelSelection,
			DrainedTools: drained, CommittedTools: committed,
			RunCreatedAt: continuation.RunCreatedAt,
			Metrics:      continuation.Metrics, Limits: continuation.Limits,
		}
	}
	suspensions := make([]sqlite.SuspensionBindingRecord, len(pending.Suspensions))
	for index, binding := range pending.Suspensions {
		suspensions[index] = sqlite.SuspensionBindingRecord{
			InterruptItemID: binding.InterruptItemID,
			ProcessID:       binding.ProcessID,
			SuspensionID:    binding.SuspensionID,
		}
	}
	return sqlite.InterruptRecord{
		RootRunID: pending.RootRunID, SessionID: pending.SessionID,
		ExecutorID: pending.ExecutorID, GoalLeaseID: pending.GoalLeaseID,
		Interrupts: pending.Interrupts, Suspensions: suspensions,
		Continuations: continuations, Capabilities: pending.Capabilities,
		CreatedAt: pending.CreatedAt,
	}
}

func pendingValue(record sqlite.InterruptRecord) runs.Pending {
	continuations := make([]runs.Continuation, len(record.Continuations))
	for index, continuation := range record.Continuations {
		drained := make([]runs.DrainedTool, len(continuation.DrainedTools))
		for toolIndex, tool := range continuation.DrainedTools {
			drained[toolIndex] = runs.DrainedTool{
				ItemID: tool.ItemID, ItemOccurredAt: tool.ItemOccurredAt,
				CallID: tool.CallID, Name: tool.Name, Arguments: tool.Arguments,
			}
		}
		committed := make([]runs.CommittedTool, len(continuation.CommittedTools))
		for toolIndex, tool := range continuation.CommittedTools {
			committed[toolIndex] = runs.CommittedTool{
				ItemID: tool.ItemID, CallID: tool.CallID, Name: tool.Name,
				Arguments: tool.Arguments, Problem: tool.Problem,
			}
		}
		continuations[index] = runs.Continuation{
			RunID: continuation.RunID, ProcessID: continuation.ProcessID,
			Lineage: continuation.Lineage, ModelSelection: continuation.ModelSelection,
			DrainedTools: drained, CommittedTools: committed,
			RunCreatedAt: continuation.RunCreatedAt,
			Metrics:      continuation.Metrics, Limits: continuation.Limits,
		}
	}
	suspensions := make([]runs.SuspensionBinding, len(record.Suspensions))
	for index, binding := range record.Suspensions {
		suspensions[index] = runs.SuspensionBinding{
			InterruptItemID: binding.InterruptItemID,
			ProcessID:       binding.ProcessID,
			SuspensionID:    binding.SuspensionID,
		}
	}
	return runs.Pending{
		RootRunID: record.RootRunID, SessionID: record.SessionID,
		ExecutorID: record.ExecutorID, GoalLeaseID: record.GoalLeaseID,
		Interrupts: record.Interrupts, Suspensions: suspensions,
		Continuations: continuations, Capabilities: record.Capabilities,
		CreatedAt: record.CreatedAt,
	}
}
