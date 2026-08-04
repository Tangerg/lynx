package runsegment

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

func testExecutorCheckpoint(rootProcessID string) execution.ExecutorCheckpoint {
	selection, err := modelref.New("anthropic", "claude")
	if err != nil {
		panic(err)
	}
	return execution.ExecutorCheckpoint{
		RootProcessID:  rootProcessID,
		Payload:        []byte(`{"root":"` + rootProcessID + `"}`),
		BuildID:        "build",
		Scope:          execution.ExecutionScope{SessionID: "ses_1"},
		ModelSelection: selection,
	}
}

type recordingExecutorCheckpointStore struct {
	saved     []execution.ExecutorCheckpoint
	deleted   [][]string
	saveErr   error
	deleteErr error
}

func (store *recordingExecutorCheckpointStore) SaveCheckpoint(
	_ context.Context,
	checkpoint execution.ExecutorCheckpoint,
) error {
	store.saved = append(store.saved, checkpoint.Clone())
	return store.saveErr
}

func (store *recordingExecutorCheckpointStore) DeleteCheckpoints(
	_ context.Context,
	_ string,
	rootIDs []string,
) error {
	store.deleted = append(store.deleted, append([]string(nil), rootIDs...))
	return store.deleteErr
}
