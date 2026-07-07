package runtime

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
)

// SpawnChildAsync is the non-blocking spawn: it creates the child, binds the
// typed input, and drives its OODA loop in the background via
// [Platform.ContinueProcessAsync] — returning the child's process id (use it as
// a task handle) and a done channel that fires the run's terminal error (nil on
// clean exit) the moment the background loop exits.
//
// The caller's tick is NOT blocked: the spawning action returns while the child
// keeps running, and the result is collected later via [Platform.ProcessByID] +
// [core.ResultOfType], or the child is canceled via [Platform.KillProcess] (=
// SDK stopTask). The child joins the parent's budget tree (subtree usage still
// counts against the parent's BudgetPolicy) and inherits the FULL parent
// blackboard via Spawn.
//
// The background run uses [context.WithoutCancel] so the child survives the
// spawning action's ctx ending — a background task whose parent action already
// returned must not die just because that call frame is gone. It therefore has
// NO deadline and is NOT auto-canceled when the parent ends; lifecycle is the
// orchestrator's job via the returned id (KillProcess one, or
// [Platform.KillChildren] to sweep all of a parent's outstanding children on
// turn exit).
//
// nil platform / nil agent / missing parent in ctx return errors.
func SpawnChildAsync(
	ctx context.Context,
	platform *Platform,
	agentDef *core.Agent,
	in any,
) (string, <-chan error, error) {
	spawn := childSpawn{
		ctx:         ctx,
		platform:    platform,
		agentDef:    agentDef,
		input:       in,
		inheritance: childInheritsAll,
	}
	child, err := spawn.prepare()
	if err != nil {
		return "", nil, err
	}
	done := platform.ContinueProcessAsync(context.WithoutCancel(ctx), child.ID())
	return child.ID(), done, nil
}
