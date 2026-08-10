package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/sqlite"
)

// ExecutorCheckpointStore translates the Application-owned checkpoint
// aggregate to SQLite's technical record. SQLite never imports Application and
// never becomes an owner of executor lifecycle semantics.
type ExecutorCheckpointStore struct {
	storage *sqlite.ExecutorCheckpointStore
}

func NewExecutorCheckpointStore(storage *sqlite.ExecutorCheckpointStore) *ExecutorCheckpointStore {
	return &ExecutorCheckpointStore{storage: storage}
}

func (store *ExecutorCheckpointStore) SaveCheckpoint(ctx context.Context, checkpoint runs.ExecutorCheckpoint) error {
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	err := store.storage.SaveCheckpoint(ctx, sqlite.ExecutorCheckpointRecord{
		RootMemberID: checkpoint.RootMemberID,
		Payload:      append([]byte(nil), checkpoint.Payload...),
		BuildID:      checkpoint.BuildID,
		Scope: sqlite.ExecutorScopeRecord{
			SessionID:    checkpoint.Scope.SessionID,
			CWD:          checkpoint.Scope.CWD,
			WorkspaceCWD: checkpoint.Scope.WorkspaceCWD,
			Isolated:     checkpoint.Scope.Isolated,
			GoalLeaseID:  checkpoint.Scope.GoalLeaseID,
		},
		ModelSelection: checkpoint.ModelSelection,
		Limits:         checkpoint.Limits,
		Usage:          checkpoint.Usage,
	})
	return translateCheckpointStorageError(err)
}

func (store *ExecutorCheckpointStore) LoadCheckpoint(ctx context.Context, rootMemberID string) (runs.ExecutorCheckpoint, error) {
	record, err := store.storage.LoadCheckpoint(ctx, rootMemberID)
	if err != nil {
		return runs.ExecutorCheckpoint{}, translateCheckpointStorageError(err)
	}
	checkpoint := runs.ExecutorCheckpoint{
		RootMemberID: record.RootMemberID,
		Payload:      append([]byte(nil), record.Payload...),
		BuildID:      record.BuildID,
		Scope: runs.ExecutionScope{
			SessionID:    record.Scope.SessionID,
			CWD:          record.Scope.CWD,
			WorkspaceCWD: record.Scope.WorkspaceCWD,
			Isolated:     record.Scope.Isolated,
			GoalLeaseID:  record.Scope.GoalLeaseID,
		},
		ModelSelection: record.ModelSelection,
		Limits:         record.Limits,
		Usage:          record.Usage,
	}
	if err := checkpoint.Validate(); err != nil {
		return runs.ExecutorCheckpoint{}, fmt.Errorf("persistence: load executor checkpoint: %w", err)
	}
	return checkpoint, nil
}

func (store *ExecutorCheckpointStore) DeleteCheckpoints(ctx context.Context, sessionID string, rootIDs []string) error {
	return translateCheckpointStorageError(store.storage.DeleteCheckpoints(ctx, sessionID, rootIDs))
}

func (store *ExecutorCheckpointStore) DeleteSessionCheckpoints(ctx context.Context, sessionID string) error {
	return translateCheckpointStorageError(store.storage.DeleteSessionCheckpoints(ctx, sessionID))
}

func (store *ExecutorCheckpointStore) DeleteUnownedCheckpoints(ctx context.Context, keepRootIDs []string) error {
	return translateCheckpointStorageError(store.storage.DeleteUnownedCheckpoints(ctx, keepRootIDs))
}

func translateCheckpointStorageError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sqlite.ErrExecutorCheckpointRecordNotFound):
		return fmt.Errorf("%w: %w", runs.ErrExecutorCheckpointNotFound, err)
	case errors.Is(err, sqlite.ErrInvalidExecutorCheckpointRecord):
		return fmt.Errorf("%w: %w", runs.ErrInvalidExecutorCheckpoint, err)
	default:
		return err
	}
}
