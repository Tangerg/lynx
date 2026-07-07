package runtime

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
)

// SpawnChild creates and runs a child sub-agent under the parent process
// attached to ctx via [core.WithProcess]. The child inherits the FULL parent
// blackboard via [core.Blackboard.Spawn] (CreateChildProcess's default), so
// every artifact the parent has staged is visible to it.
//
// This is the widest-inheritance spawn — the top of the gradient [SpawnChild]
// (everything) → [SpawnChildProtectedOnly] (ambient only) → [SpawnChildFresh]
// (nothing). Use it for supervisor flows where the sub-agent genuinely needs to
// read the parent's accumulated state. Beware the trade-off: a sub-agent with a
// [GoalProducing] goal whose type the parent already staged finds its goal
// pre-satisfied and runs no action — for self-contained delegation prefer
// [SpawnChildProtectedOnly].
//
// Same steps / error contract / budget aggregation as
// [SpawnChildProtectedOnly]; only the inherited blackboard state differs.
func SpawnChild(
	ctx context.Context,
	platform *Platform,
	agentDef *core.Agent,
	in any,
) (*AgentProcess, error) {
	return childSpawn{
		ctx:         ctx,
		platform:    platform,
		agentDef:    agentDef,
		input:       in,
		inheritance: childInheritsAll,
	}.run()
}

// SpawnChildProtectedOnly creates and runs a child sub-agent process under the
// parent attached to ctx via [core.WithProcess]. The child gets a FRESH
// blackboard that retains ONLY the parent's protected entries — those bound via
// [core.BlackboardWriter.BindProtected]: session id, working directory, and
// other ambient context. The parent's ordinary working state (named bindings,
// conditions, accumulated objects) is NOT inherited.
//
// This is the right default for delegating a self-contained subtask: the
// sub-agent starts clean — so its produce-Out goal isn't accidentally
// pre-satisfied by an object the parent already staged (the no-op failure mode
// [SpawnChildFresh]'s doc warns about) — yet still sees the ambient context the
// session needs (e.g. which project directory the tools run in). It backs the
// agent-as-tool constructors ([AsChatTool] / [AsChatToolFromAgent] /
// [SubagentTools] / [AllAchievableTools]).
//
// Steps:
//
//  1. Resolve the parent [core.Process] from ctx; error if not present.
//  2. Derive the child blackboard via [protectedOnlyBlackboard]: Spawn the
//     parent's, then Clear it — Clear keeps protected entries and drops
//     everything else.
//  3. CreateChildProcess with that blackboard (joins the parent's budget tree)
//     and Bind in (typed dual-binding so the child's first action's planner
//     resolves its In by type).
//  4. ContinueProcess to drive the child's OODA loop.
//
// Returns the child *AgentProcess in a terminal state (Completed / Failed /
// Waiting / Stuck / Terminated / Killed). Callers inspect Status() and either
// extract output via [core.ResultOfType] or classify the failure via
// [TerminalError].
//
// nil platform / nil agentDef / missing parent in ctx return errors rather than
// panic — callers fan in user data and a runtime error is the right response.
func SpawnChildProtectedOnly(
	ctx context.Context,
	platform *Platform,
	agentDef *core.Agent,
	in any,
) (*AgentProcess, error) {
	return childSpawn{
		ctx:         ctx,
		platform:    platform,
		agentDef:    agentDef,
		input:       in,
		inheritance: childInheritsProtectedOnly,
	}.run()
}

// SpawnChildFresh is the fully-isolated spawn: the child receives a **fresh**
// blackboard (constructed via [Platform.NewBlackboard]) seeded only with the
// typed input. Parent state — including any Out values the calling action may
// have returned in prior ticks, AND the parent's protected entries — is NOT
// inherited. (For delegation that should keep ambient/session context, use
// [SpawnChildProtectedOnly].)
//
// Use for:
//
//   - Loop iterations where each pass must start clean (otherwise the
//     orchestrator's accumulated Out short-circuits the body's "produce Out"
//     goal).
//   - Parallel fan-out where peers must be branch-isolated to keep LLM contexts
//     from cross-pollinating.
//   - Pipeline steps where each stage should see only the previous stage's typed
//     output (passed as in), not the original orchestrator input or peer-step
//     artifacts.
//
// Same error contract and budget aggregation as [SpawnChildProtectedOnly]; the
// only difference is the blackboard seed (nothing vs. protected-only).
func SpawnChildFresh(
	ctx context.Context,
	platform *Platform,
	agentDef *core.Agent,
	in any,
) (*AgentProcess, error) {
	return childSpawn{
		ctx:         ctx,
		platform:    platform,
		agentDef:    agentDef,
		input:       in,
		inheritance: childInheritsNothing,
	}.run()
}
