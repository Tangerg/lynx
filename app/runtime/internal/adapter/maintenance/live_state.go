package maintenance

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/exec"
)

// TodoReader is the compactor's read-only view of a session todo list.
type TodoReader interface {
	List(ctx context.Context, sessionID string) ([]todo.Item, error)
}

// NewLiveState adapts live shells and persisted todos to the compactor's
// reminder source. A todo-read failure omits todos rather than failing the
// compaction it decorates.
func NewLiveState(shells *exec.Shells, todos TodoReader) LiveStateFunc {
	if shells == nil && todos == nil {
		return nil
	}
	return func(ctx context.Context, sessionID string) LiveStateSnapshot {
		var snap LiveStateSnapshot
		if shells != nil {
			for _, sh := range shells.RunningForSession(sessionID) {
				snap.Shells = append(snap.Shells, RunningShell{ID: sh.ID, Command: sh.Command})
			}
		}
		if todos != nil {
			if items, err := todos.List(ctx, sessionID); err == nil {
				for _, item := range items {
					if item.Status == todo.StatusInProgress {
						snap.Todos = append(snap.Todos, item.Content)
					}
				}
			}
		}
		return snap
	}
}
