package bootstrap

import (
	"context"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/models/catalog"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/maintenance"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/exec"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/skillauthoring"
)

type turnServices struct {
	steering  turn.SteeringSink
	compactor turn.Compactor
	extractor turn.Extractor
	miner     turn.SkillMiner
}

func buildTurnServices(cfg Config, messages messageEnvironment, shells *exec.Shells, skillStore *skillauthoring.Store, resolveUtility func(context.Context) *chatclient.Client, embedder func(context.Context) (agentmemory.Embedder, error)) turnServices {
	services := turnServices{
		steering:  cfg.Steering,
		compactor: cfg.Compactor,
		extractor: cfg.Extractor,
		miner:     cfg.Miner,
	}
	if services.steering == nil {
		services.steering = messages.conversation
	}
	if services.compactor == nil {
		window := 0
		if info, ok := catalog.Lookup(cfg.Provider, cfg.Model); ok {
			window = int(info.Limits.ContextWindow)
		}
		services.compactor = maintenance.NewCompactor(
			messages.store,
			resolveUtility,
			liveStateSnapshot(shells, cfg.TodoStore),
			maintenance.CompactionConfig{ContextWindow: window},
		)
	}
	if services.extractor == nil && cfg.AgentMemoryStore != nil {
		services.extractor = maintenance.NewExtractor(messages.store, cfg.AgentMemoryStore, resolveUtility, embedder, maintenance.CurationConfig{})
	}
	if services.miner == nil && skillStore.Enabled() {
		services.miner = maintenance.NewSkillMiner(messages.store, skillStore, resolveUtility, maintenance.MinerConfig{})
	}
	return services
}

// liveStateSnapshot adapts the background-shell set and the todo store into the
// compactor's live-state source: a session's still-running shells and its
// in-progress tasks. Returns nil (reminder disabled) when neither source exists.
// Building the reminder is best-effort — a todo-store read error omits the tasks
// section rather than failing the compaction it decorates.
func liveStateSnapshot(shells *exec.Shells, todos todo.Store) maintenance.LiveStateFunc {
	if shells == nil && todos == nil {
		return nil
	}
	return func(ctx context.Context, sessionID string) maintenance.LiveStateSnapshot {
		var snap maintenance.LiveStateSnapshot
		if shells != nil {
			for _, sh := range shells.RunningForSession(sessionID) {
				snap.Shells = append(snap.Shells, maintenance.RunningShell{ID: sh.ID, Command: sh.Command})
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
