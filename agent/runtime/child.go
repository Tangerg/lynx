package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
)

// SpawnChild creates and runs a child sub-agent process under the parent
// process attached to ctx via [core.WithProcess]. It is the public
// helper that ties together [Platform.CreateChildProcess] +
// [Platform.ContinueProcess] + initial Bind for the typed input — the
// same plumbing [AsChatTool] uses, exposed for deterministic
// agent-level orchestration.
//
// The child blackboard inherits the parent's via
// [core.Blackboard.Spawn] (default behavior of CreateChildProcess) so
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
// Status() and either extract output via [core.ResultOfType] or
// classify the failure via [ChildError].
//
// nil platform / nil agentDef / missing parent in ctx return errors
// rather than panic — callers fan in user data and a runtime error is
// the right response.
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
		return nil, errors.New("spawn child fresh: platform is nil")
	}
	options := core.ProcessOptions{Blackboard: platform.NewBlackboard()}
	return spawnChildOptions(ctx, platform, agentDef, in, options)
}

// SpawnChildAsync is the non-blocking sibling of [SpawnChild]: it
// creates the child, binds the typed input, and drives its OODA loop in
// the background via [Platform.ContinueProcessAsync] — returning the
// child's process id (use it as a task handle) and a done channel that
// fires the run's terminal error (nil on clean exit) the moment the
// background loop exits.
//
// Unlike SpawnChild the caller's tick is NOT blocked: the spawning
// action returns while the child keeps running, and the result is
// collected later via [Platform.ProcessByID] + [core.ResultOfType], or
// the child is cancelled via [Platform.KillProcess] (= SDK stopTask).
// The child joins the parent's budget tree (subtree usage still counts
// against the parent's BudgetPolicy) and inherits the parent blackboard
// via Spawn, exactly like SpawnChild.
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

// RunFresh is the top-level companion to [SpawnChild]: starts a fresh
// process via [Platform.RunAgent] (no parent process required in ctx)
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
// [Platform.ContinueProcess] ([SpawnChild] / [SpawnChildFresh]) or in
// the background via [Platform.ContinueProcessAsync] ([SpawnChildAsync]).
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

// spawnChildOptions is the synchronous shared core of [SpawnChild] and
// [SpawnChildFresh] — prepare the child, then drive it to a terminal
// state in-line. The two differ only in the ProcessOptions.Blackboard
// slot they pass through.
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
