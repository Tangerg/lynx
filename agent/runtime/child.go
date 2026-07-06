package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
)

// maxSpawnDepth is the hard backstop on sub-agent delegation depth (a top-level
// process is depth 0, its child 1, and so on). A spawn that would exceed it
// fails fast with [ErrMaxSpawnDepth] BEFORE the child process is created —
// structural insurance against pathological recursive delegation (an agent that
// keeps delegating to itself) that holds even when no token / step budget is
// set. Generous on purpose: real recursive task decomposition nests only a few
// levels, so this never trips legitimate use — it only stops runaways.
const maxSpawnDepth = 8

// ErrMaxSpawnDepth reports that a sub-agent spawn was refused because it would
// exceed [maxSpawnDepth]. All spawn entry points fail with it before creating a
// child; the agent-as-tool wrapper returns it as a tool error, so the
// delegating model re-plans instead of recursing without bound.
var ErrMaxSpawnDepth = errors.New("spawn child: max delegation depth exceeded")

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
	return childSpawn{
		ctx:         ctx,
		platform:    platform,
		agentDef:    agentDef,
		input:       in,
		inheritance: childInheritsAll,
	}.run()
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
// [TerminalError].
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
	return childSpawn{
		ctx:         ctx,
		platform:    platform,
		agentDef:    agentDef,
		input:       in,
		inheritance: childInheritsProtectedOnly,
	}.run()
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
	return childSpawn{
		ctx:         ctx,
		platform:    platform,
		agentDef:    agentDef,
		input:       in,
		inheritance: childInheritsNothing,
	}.run()
}

// SpawnChildAsync is the non-blocking spawn: it creates the child, binds
// the typed input, and drives its OODA loop in the background via
// [Platform.ContinueProcessAsync] — returning the child's process id (use
// it as a task handle) and a done channel that fires the run's terminal
// error (nil on clean exit) the moment the background loop exits.
//
// The caller's tick is NOT blocked: the spawning action returns while the
// child keeps running, and the result is collected later via
// [Platform.ProcessByID] + [core.ResultOfType], or the child is canceled
// via [Platform.KillProcess] (= SDK stopTask). The child joins the parent's
// budget tree (subtree usage still counts against the parent's
// BudgetPolicy) and inherits the FULL parent blackboard via Spawn.
//
// The background run uses [context.WithoutCancel] so the child survives
// the spawning action's ctx ending — a background task whose parent
// action already returned must not die just because that call frame is
// gone. It therefore has NO deadline and is NOT auto-canceled when the
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

// TerminalError formats a non-Completed terminal status as an error.
// Returns nil when the process completed cleanly. Used by workflow
// builders and agent-as-tool wrappers to bubble up a uniform "ended in
// X / ended in X: failure" message; call sites add their own prefix
// context (step number / agent name / iteration index).
//
// Waiting is treated as a non-terminal failure here. Agent-as-tool
// wrappers that want to surface a structured "waiting" tool-result
// (instead of bubbling the error) should branch on
// [core.AgentProcessStatus] before calling TerminalError.
func (p *AgentProcess) TerminalError() error {
	if p == nil {
		return errors.New("process is nil")
	}
	status := p.Status()
	if status == core.StatusCompleted {
		return nil
	}
	if failure := p.Failure(); failure != nil {
		return fmt.Errorf("ended in %s: %w", status, failure)
	}
	return fmt.Errorf("ended in %s", status)
}

type childBlackboardInheritance int

const (
	childInheritsAll childBlackboardInheritance = iota
	childInheritsProtectedOnly
	childInheritsNothing
)

type childSpawn struct {
	ctx         context.Context
	platform    *Platform
	agentDef    *core.Agent
	input       any
	inheritance childBlackboardInheritance
}

func (s childSpawn) run() (*AgentProcess, error) {
	child, err := s.prepare()
	if err != nil {
		return nil, err
	}
	if err := s.platform.ContinueProcess(s.ctx, child.ID()); err != nil {
		return nil, fmt.Errorf("spawn child %q (process %q): run: %w", s.agentDef.Name, child.ID(), err)
	}
	return child, nil
}

func (s childSpawn) prepare() (*AgentProcess, error) {
	parentProc, err := s.parent()
	if err != nil {
		return nil, err
	}
	// Structural backstop: refuse before creating the child, so a runaway
	// delegation chain fails fast instead of burning budget spawning deeper.
	if parentProc.depth+1 > maxSpawnDepth {
		return nil, fmt.Errorf("spawn child %q: %w (depth %d > max %d)", s.agentDef.Name, ErrMaxSpawnDepth, parentProc.depth+1, maxSpawnDepth)
	}

	child, err := s.platform.CreateChildProcess(s.agentDef, parentProc, s.options(parentProc))
	if err != nil {
		return nil, fmt.Errorf("spawn child %q: create: %w", s.agentDef.Name, err)
	}
	if err := s.linkSession(child, parentProc); err != nil {
		// CreateChildProcess registered the child AND joined it to the parent's
		// budget tree; linking its session failed, so undo BOTH — unregister it
		// from the platform and drop it from the parent's budget rollup. Either
		// left behind leaks: a never-started child sits at StatusNotStarted
		// (which PruneTerminalProcesses skips), and a stale budget child ref
		// lingers for the parent's whole life.
		_ = s.platform.RemoveProcess(child.ID())
		parentProc.budget.removeChild(child)
		return nil, fmt.Errorf("spawn child %q: link session: %w", s.agentDef.Name, err)
	}
	if s.input != nil {
		child.Blackboard().Bind(s.input)
	}
	return child, nil
}

func (s childSpawn) parent() (*AgentProcess, error) {
	if s.platform == nil {
		return nil, errors.New("spawn child: platform is nil")
	}
	if s.agentDef == nil {
		return nil, errors.New("spawn child: agent is nil")
	}
	parent := core.ProcessFrom(s.ctx)
	if parent == nil {
		return nil, errors.New("spawn child: no parent process in ctx (use core.WithProcess to inject one)")
	}
	parentProc, ok := s.platform.ProcessByID(parent.ID())
	if !ok {
		return nil, fmt.Errorf("spawn child: parent process %q not registered on platform", parent.ID())
	}
	return parentProc, nil
}

func (s childSpawn) options(parent *AgentProcess) core.ProcessOptions {
	switch s.inheritance {
	case childInheritsProtectedOnly:
		return core.ProcessOptions{Blackboard: protectedOnlyBlackboard(parent.Blackboard())}
	case childInheritsNothing:
		return core.ProcessOptions{Blackboard: s.platform.NewBlackboard()}
	default:
		return core.ProcessOptions{}
	}
}

// linkSession gives the child its own conversation while preserving delegation
// lineage through ParentID. Explicitly pinned sessions are left untouched.
func (s childSpawn) linkSession(child, parent *AgentProcess) error {
	if child.options == nil || child.options.Session != nil {
		return nil
	}
	parentConvID := parent.conversationID()
	if parentConvID == "" {
		return nil
	}
	session := core.NewSession(child.ID(), parent.userID(), s.agentDef.Name)
	session.ParentID = parentConvID
	child.options.Session = &session

	if s.platform.sessionStore != nil {
		if err := s.platform.sessionStore.Save(s.ctx, session); err != nil {
			return err
		}
	}
	return nil
}
