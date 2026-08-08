package runsegment

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

func testExecutorCheckpoint(rootProcessID string) runs.ExecutorCheckpoint {
	selection, err := modelref.New("anthropic", "claude")
	if err != nil {
		panic(err)
	}
	return runs.ExecutorCheckpoint{
		RootProcessID:  rootProcessID,
		Payload:        []byte(`{"root":"` + rootProcessID + `"}`),
		BuildID:        "build",
		Scope:          runs.ExecutionScope{SessionID: "ses_1"},
		ModelSelection: selection,
	}
}

type recordingExecutorCheckpointStore struct {
	saved     []runs.ExecutorCheckpoint
	deleted   [][]string
	saveErr   error
	deleteErr error
}

func (store *recordingExecutorCheckpointStore) SaveCheckpoint(
	_ context.Context,
	checkpoint runs.ExecutorCheckpoint,
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
