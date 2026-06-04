package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
)

// SpawnChild creates and runs a child sub-agent under the parent process
// attached to ctx via [core.WithProcess]. The child inherits the FULL
// parent blackboard via [core.Blackboard.Spawn] (CreateChildProcess's
// default), so every artifact the parent has staged is visible to it.
//
// This is the widest-inheritance spawn — the top of the gradient
// [SpawnChild] (everything) → [SpawnChildProtectedOnly] (ambient only) →
// [SpawnChildFresh] (nothing). Use it for supervisor flows where the
// sub-agent genuinely needs to read the parent's accumulated state. Beware
// the trade-off: a sub-agent with a [GoalProducing] goal whose type the
// parent already staged finds its goal pre-satisfied and runs no action —
// for self-contained delegation prefer [SpawnChildProtectedOnly].
//
// Same steps / error contract / budget aggregation as
// [SpawnChildProtectedOnly]; only the inherited blackboard state differs.
func SpawnChild(
	ctx context.Context,
	platform *Platform,
	agentDef *core.Agent,
	in any,
) (*AgentProcess, error) {
	return spawnChildOptions(ctx, platform, agentDef, in, core.ProcessOptions{})
}

// SpawnChildProtectedOnly creates and runs a child sub-agent process under
// the parent attached to ctx via [core.WithProcess]. The child gets a FRESH
// blackboard that retains ONLY the parent's protected entries — those bound
// via [core.BlackboardWriter.BindProtected]: session id, working directory,
// and other ambient context. The parent's ordinary working state (named
// bindings, conditions, accumulated objects) is NOT inherited.
//
// This is the right default for delegating a self-contained subtask: the
// sub-agent starts clean — so its produce-Out goal isn't accidentally
// pre-satisfied by an object the parent already staged (the no-op failure
// mode [SpawnChildFresh]'s doc warns about) — yet still sees the ambient
// context the session needs (e.g. which project directory the tools run
// in). It backs the agent-as-tool constructors ([AsChatTool] /
// [AsChatToolFromAgent] / [SubagentTools] / [AllAchievableTools]).
//
// Steps:
//
//  1. Resolve the parent [core.Process] from ctx; error if not present.
//  2. Derive the child blackboard via [protectedOnlyBlackboard]: Spawn the
//     parent's, then Clear it — Clear keeps protected entries and drops
//     everything else.
//  3. CreateChildProcess with that blackboard (joins the parent's budget
//     tree) and Bind in (typed dual-binding so the child's first action's
//     planner resolves its In by type).
//  4. ContinueProcess to drive the child's OODA loop.
//
// Returns the child *AgentProcess in a terminal state (Completed / Failed /
// Waiting / Stuck / Terminated / Killed). Callers inspect Status() and
// either extract output via [core.ResultOfType] or classify the failure via
// [ChildError].
//
// nil platform / nil agentDef / missing parent in ctx return errors rather
// than panic — callers fan in user data and a runtime error is the right
// response.
func SpawnChildProtectedOnly(
	ctx context.Context,
	platform *Platform,
	agentDef *core.Agent,
	in any,
) (*AgentProcess, error) {
	if platform == nil {
		return nil, errors.New("spawn child fresh protected: platform is nil")
	}
	parent := core.ProcessFrom(ctx)
	if parent == nil {
		return nil, errors.New("spawn child fresh protected: no parent process in ctx (use core.WithProcess to inject one)")
	}
	return spawnChildOptions(ctx, platform, agentDef, in, core.ProcessOptions{
		Blackboard: protectedOnlyBlackboard(parent.Blackboard()),
	})
}

// protectedOnlyBlackboard returns a child blackboard that keeps only the
// parent's protected entries: [core.Blackboard.Spawn] copies all state, then
// [core.Blackboard.Clear] drops everything except entries bound via
// BindProtected. The result is a clean working surface that still carries
// the parent's ambient / session context.
func protectedOnlyBlackboard(parent core.Blackboard) core.Blackboard {
	bb := parent.Spawn()
	bb.Clear()
	return bb
}

// SpawnChildFresh is the fully-isolated spawn: the child receives a
// **fresh** blackboard (constructed via [Platform.NewBlackboard]) seeded
// only with the typed input. Parent state — including any Out values the
// calling action may have returned in prior ticks, AND the parent's
// protected entries — is NOT inherited. (For delegation that should keep
// ambient/session context, use [SpawnChildProtectedOnly].)
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
// Same error contract and budget aggregation as [SpawnChildProtectedOnly];
// the only difference is the blackboard seed (nothing vs. protected-only).
func SpawnChildFresh(
	ctx context.Context,
	platform *Platform,
	agentDef *core.Agent,
	in any,
) (*AgentProcess, error) {
	if platform == nil {
		return nil, errors.New("spawn child fresh: platform is nil")
	}
	options := core.ProcessOptions{Blackboard: platform.NewBlackboard()}
	return spawnChildOptions(ctx, platform, agentDef, in, options)
}

// SpawnChildAsync is the non-blocking spawn: it creates the child, binds
// the typed input, and drives its OODA loop in the background via
// [Platform.ContinueProcessAsync] — returning the child's process id (use
// it as a task handle) and a done channel that fires the run's terminal
// error (nil on clean exit) the moment the background loop exits.
//
// The caller's tick is NOT blocked: the spawning action returns while the
// child keeps running, and the result is collected later via
// [Platform.ProcessByID] + [core.ResultOfType], or the child is cancelled
// via [Platform.KillProcess] (= SDK stopTask). The child joins the parent's
// budget tree (subtree usage still counts against the parent's
// BudgetPolicy) and inherits the FULL parent blackboard via Spawn.
//
// The background run uses [context.WithoutCancel] so the child survives
// the spawning action's ctx ending — a background task whose parent
// action already returned must not die just because that call frame is
// gone. It therefore has NO deadline and is NOT auto-cancelled when the
// parent ends; lifecycle is the orchestrator's job via the returned id
// (KillProcess one, or [Platform.KillChildren] to sweep all of a
// parent's outstanding children on turn exit).
//
// nil platform / nil agent / missing parent in ctx return errors.
func SpawnChildAsync(
	ctx context.Context,
	platform *Platform,
	agentDef *core.Agent,
	in any,
) (string, <-chan error, error) {
	child, err := prepareChild(ctx, platform, agentDef, in, core.ProcessOptions{})
	if err != nil {
		return "", nil, err
	}
	done := platform.ContinueProcessAsync(context.WithoutCancel(ctx), child.ID())
	return child.ID(), done, nil
}

// RunFresh is the top-level spawn: starts a fresh process via
// [Platform.RunAgent] (no parent process required in ctx)
// and binds in under [core.DefaultBindingName]. Used by MCP-publish
// style flows where each call is its own root process rather than a
// child of the calling LLM's parent.
//
// nil in produces a nil bindings map so the agent's first action
// resolves its input from the planner's defaults instead of from a
// `nil` slot.
func RunFresh(
	ctx context.Context,
	platform *Platform,
	agentDef *core.Agent,
	in any,
) (*AgentProcess, error) {
	if platform == nil {
		return nil, errors.New("run fresh: platform is nil")
	}
	if agentDef == nil {
		return nil, errors.New("run fresh: agent is nil")
	}
	var bindings map[string]any
	if in != nil {
		bindings = map[string]any{core.DefaultBindingName: in}
	}
	proc, err := platform.RunAgent(ctx, agentDef, bindings, core.ProcessOptions{})
	if err != nil {
		return nil, fmt.Errorf("run agent %q: %w", agentDef.Name, err)
	}
	return proc, nil
}

// ChildError formats a non-Completed terminal status as an error.
// Returns nil when child completed cleanly. Used by workflow builders
// and agent-as-tool wrappers to bubble up a uniform "ended in X /
// ended in X: failure" message; call sites add their own prefix
// context (step number / agent name / iteration index).
//
// Waiting is treated as a non-terminal failure here. Agent-as-tool
// wrappers that want to surface a structured "waiting" tool-result
// (instead of bubbling the error) should branch on
// [core.AgentProcessStatus] before calling ChildError.
func ChildError(child *AgentProcess) error {
	if child == nil {
		return errors.New("child process is nil")
	}
	status := child.Status()
	if status == core.StatusCompleted {
		return nil
	}
	if failure := child.Failure(); failure != nil {
		return fmt.Errorf("ended in %s: %w", status, failure)
	}
	return fmt.Errorf("ended in %s", status)
}

// prepareChild is the shared prefix of every child-spawn entry point:
// it validates the inputs, resolves the parent process from ctx,
// creates the child (joining the parent's budget tree, with the given
// blackboard options), and binds the typed input — returning a child
// ready to be driven. The caller picks how: synchronously via
// [Platform.ContinueProcess] ([SpawnChild] / [SpawnChildProtectedOnly] /
// [SpawnChildFresh]) or in the background via
// [Platform.ContinueProcessAsync] ([SpawnChildAsync]).
// Centralizing the prefix keeps validation and error messages identical
// across the three.
func prepareChild(
	ctx context.Context,
	platform *Platform,
	agentDef *core.Agent,
	in any,
	options core.ProcessOptions,
) (*AgentProcess, error) {
	if platform == nil {
		return nil, errors.New("spawn child: platform is nil")
	}
	if agentDef == nil {
		return nil, errors.New("spawn child: agent is nil")
	}
	parent := core.ProcessFrom(ctx)
	if parent == nil {
		return nil, errors.New("spawn child: no parent process in ctx (use core.WithProcess to inject one)")
	}
	parentProc, ok := platform.ProcessByID(parent.ID())
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
	return child, nil
}

// spawnChildOptions is the synchronous shared core of [SpawnChild],
// [SpawnChildProtectedOnly] and [SpawnChildFresh] — prepare the child,
// then drive it to a terminal state in-line. They differ only in the
// ProcessOptions.Blackboard slot they pass through.
func spawnChildOptions(
	ctx context.Context,
	platform *Platform,
	agentDef *core.Agent,
	in any,
	options core.ProcessOptions,
) (*AgentProcess, error) {
	child, err := prepareChild(ctx, platform, agentDef, in, options)
	if err != nil {
		return nil, err
	}
	if err := platform.ContinueProcess(ctx, child.ID()); err != nil {
		return nil, fmt.Errorf("spawn child %q (process %q): run: %w", agentDef.Name, child.ID(), err)
	}
	return child, nil
}
