package persistence

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/sqlite"
)

// WorkspaceMutationStore translates the Session rollback use case's durable
// intent into a SQLite technical record. The transaction ordering remains an
// Application concern; SQLite only persists and lists records.
type WorkspaceMutationStore struct {
	storage *sqlite.WorkspaceMutationStore
}

func NewWorkspaceMutationStore(storage *sqlite.WorkspaceMutationStore) *WorkspaceMutationStore {
	return &WorkspaceMutationStore{storage: storage}
}

func (store *WorkspaceMutationStore) Record(ctx context.Context, mutation sessions.WorkspaceMutation) error {
	return store.storage.Record(ctx, sqlite.WorkspaceMutationRecord{
		SessionID:      mutation.SessionID,
		CWD:            mutation.CWD,
		ToRunID:        mutation.ToRunID,
		RestoreHistory: mutation.RestoreHistory,
	})
}

func (store *WorkspaceMutationStore) Complete(ctx context.Context, sessionID string) error {
	return store.storage.Complete(ctx, sessionID)
}

func (store *WorkspaceMutationStore) ListPending(ctx context.Context) ([]sessions.WorkspaceMutation, error) {
	records, err := store.storage.ListPending(ctx)
	if err != nil {
		return nil, err
	}
	mutations := make([]sessions.WorkspaceMutation, len(records))
	for index, record := range records {
		mutations[index] = sessions.WorkspaceMutation{
			SessionID:      record.SessionID,
			CWD:            record.CWD,
			ToRunID:        record.ToRunID,
			RestoreHistory: record.RestoreHistory,
		}
	}
	return mutations, nil
}
