package runtime

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
)

// SpawnChild creates and runs a child sub-agent process under the parent
// process attached to ctx via [core.WithProcess]. It is the public
// helper that ties together [Platform.CreateChildProcess] +
// [Platform.ContinueProcess] + initial Bind for the typed input — the
// same plumbing [AsChatTool]'s internal subagent strategy uses, exposed
// for deterministic agent-level orchestration.
//
// The child blackboard inherits the parent's via
// [core.Blackboard.Spawn] (default behaviour of CreateChildProcess) so
// staged artifacts on the parent are visible to the child. Use this
// for **supervisor-style** flows where the sub-agent should see what
// the parent already knows. For **orchestration** flows (loop / fan-out
// where each child should run cleanly without seeing the orchestrator's
// accumulated outputs), use [SpawnChildFresh] instead.
//
// Steps:
//
//  1. Resolve the parent [core.Process] from ctx; error if not present.
//  2. Look the parent up by id in platform.procs; error if not registered.
//  3. CreateChildProcess (inherits parent blackboard via Spawn, joins
//     parent's budget tree).
//  4. Bind in onto the child blackboard (typed dual-binding so the
//     child's first action's planner can resolve its In by type).
//  5. ContinueProcess to drive the child's OODA loop.
//
// Returns the child *AgentProcess in a terminal state (Completed /
// Failed / Waiting / Stuck / Terminated / Killed). Callers inspect
// Status() and extract the typed output via [core.ResultOfType].
//
// nil platform / nil agentDef / missing parent in ctx return errors
// rather than panic — workflow callers fan in user data and a runtime
// error is the right response.
func SpawnChild(
	ctx context.Context,
	platform *Platform,
	agentDef *core.Agent,
	in any,
) (*AgentProcess, error) {
	return spawnChildOptions(ctx, platform, agentDef, in, core.ProcessOptions{})
}

// SpawnChildFresh is the orchestration-flow variant of [SpawnChild]:
// the child receives a **fresh** blackboard (constructed via
// [Platform.NewBlackboard]) seeded only with the typed input. Parent
// state — including any Out values the calling action may have
// returned in prior ticks — is NOT inherited.
//
// Use for:
//
//   - Loop iterations where each pass must start clean (otherwise the
//     orchestrator's accumulated Out short-circuits the body's
//     "produce Out" goal).
//   - Parallel fan-out where peers must be branch-isolated to keep
//     LLM contexts from cross-pollinating.
//   - Pipeline steps where each stage should see only the previous
//     stage's typed output (passed as in), not the original orchestrator
//     input or peer-step artifacts.
//
// Same error contract and budget aggregation as [SpawnChild]; the only
// difference is the blackboard seed.
func SpawnChildFresh(
	ctx context.Context,
	platform *Platform,
	agentDef *core.Agent,
	in any,
) (*AgentProcess, error) {
	if platform == nil {
		return nil, fmt.Errorf("spawn child fresh: platform is nil")
	}
	options := core.ProcessOptions{Blackboard: platform.NewBlackboard()}
	return spawnChildOptions(ctx, platform, agentDef, in, options)
}

// spawnChildOptions is the shared core of [SpawnChild] and
// [SpawnChildFresh] — the only difference between the two is the
// ProcessOptions.Blackboard slot.
func spawnChildOptions(
	ctx context.Context,
	platform *Platform,
	agentDef *core.Agent,
	in any,
	options core.ProcessOptions,
) (*AgentProcess, error) {
	if platform == nil {
		return nil, fmt.Errorf("spawn child: platform is nil")
	}
	if agentDef == nil {
		return nil, fmt.Errorf("spawn child: agent is nil")
	}
	parent := core.ProcessFrom(ctx)
	if parent == nil {
		return nil, fmt.Errorf("spawn child: no parent process in ctx (use core.WithProcess to inject one)")
	}
	parentProc, ok := platform.GetProcess(parent.ID())
	if !ok {
		return nil, fmt.Errorf("spawn child: parent process %q not registered on platform", parent.ID())
	}

	child, err := platform.CreateChildProcess(agentDef, parentProc, options)
	if err != nil {
		return nil, fmt.Errorf("spawn child %q: create: %w", agentDef.Name, err)
	}
	if in != nil {
		child.Blackboard().Bind(in)
	}

	if err := platform.ContinueProcess(ctx, child.ID()); err != nil {
		return nil, fmt.Errorf("spawn child %q (process %q): run: %w", agentDef.Name, child.ID(), err)
	}
	return child, nil
}
